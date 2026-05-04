param(
  [string]$RepoRoot = (Get-Location).Path,
  [string]$OneDrivePath = "",
  [string]$StorePath = "",
  [string]$TestRoot = "",
  [string]$SmokeId = "",
  [switch]$SkipValidation,
  [switch]$KeepData
)

$ErrorActionPreference = "Stop"

function Write-Step([string]$Message) {
  Write-Host "`n== $Message =="
}

function Resolve-OneDrivePath {
  if ($OneDrivePath) {
    return $OneDrivePath
  }

  $accounts = Get-ChildItem "HKCU:\Software\Microsoft\OneDrive\Accounts" -ErrorAction SilentlyContinue |
    ForEach-Object { Get-ItemProperty $_.PSPath -ErrorAction SilentlyContinue } |
    Where-Object { $_.UserFolder } |
    Select-Object DisplayName, UserEmail, UserFolder

  if ($accounts) {
    return ($accounts | Select-Object -First 1).UserFolder
  }

  $folder = Get-ChildItem $env:USERPROFILE -Directory -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -like "OneDrive*" } |
    Select-Object -First 1

  if ($folder) {
    return $folder.FullName
  }

  throw "OneDrive folder not found. Pass -OneDrivePath 'C:\Users\...\OneDrive...'"
}

function Ensure-LokiStore([string]$Store) {
  $tmpDir = Join-Path $RepoRoot "_tmp_ensure_store"
  New-Item -ItemType Directory -Force $tmpDir | Out-Null
  $tmpMain = Join-Path $tmpDir "main.go"
  @'
package main

import (
	"log"
	"os"

	"github.com/allensu/loki-profile-manager/internal/store"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("store path required")
	}
	if _, err := store.EnsureLayout(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}
'@ | Set-Content -Path $tmpMain -Encoding UTF8

  try {
    go run $tmpMain $Store
  } finally {
    Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
  }
}

function Assert-CommandSuccess([scriptblock]$Command, [string]$Label) {
  Write-Step $Label
  & $Command
  if ($LASTEXITCODE -ne 0) {
    throw "$Label failed with exit code $LASTEXITCODE"
  }
}

Set-Location $RepoRoot

Write-Step "repo"
git log -1 --oneline

$oneDrive = Resolve-OneDrivePath
if (-not (Test-Path $oneDrive)) {
  throw "OneDrive path does not exist: $oneDrive"
}

if (-not $StorePath) {
  $StorePath = Join-Path $oneDrive "LokiProfileManager"
}
if (-not $SmokeId) {
  $SmokeId = Get-Date -Format "yyyyMMddHHmmss"
}
$SmokeId = $SmokeId.Trim()
if ($SmokeId -notmatch '^[A-Za-z0-9._-]+$') {
  throw "SmokeId must contain only letters, numbers, dot, underscore, or hyphen: $SmokeId"
}
$SmokeProfile = "vm-smoke-$SmokeId"
$SmokeBucket = "vm-bucket-$SmokeId"
$SmokeProfilePath = Join-Path $StorePath "profiles\$SmokeProfile"

$defaultTestRoot = -not $TestRoot
if (-not $TestRoot) {
  $TestRoot = Join-Path $env:USERPROFILE "loki-vm-test-$SmokeId"
}

Write-Step "paths"
Write-Host "OneDrive: $oneDrive"
Write-Host "Store:    $StorePath"
Write-Host "TestRoot: $TestRoot"
Write-Host "Profile:  $SmokeProfile"
Write-Host "Bucket:   $SmokeBucket"

New-Item -ItemType Directory -Force $StorePath | Out-Null
if ($defaultTestRoot -and (Test-Path $TestRoot)) {
  Write-Host "Resetting disposable test root: $TestRoot"
  Remove-Item $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
}
New-Item -ItemType Directory -Force $TestRoot | Out-Null

Write-Step "pin OneDrive store local"
attrib +P -U "$StorePath" /S /D 2>$null

if (-not $SkipValidation) {
  Assert-CommandSuccess { go test ./... } "go test"
  Assert-CommandSuccess { go vet ./... } "go vet"
  Assert-CommandSuccess { go mod verify } "go mod verify"
}

Write-Step "build loki"
New-Item -ItemType Directory -Force (Join-Path $RepoRoot "bin") | Out-Null
$bin = Join-Path $RepoRoot "bin\loki.exe"
go build -o $bin .\cmd\loki
if (-not (Test-Path $bin)) {
  throw "build did not create $bin"
}
Write-Host "Binary: $bin"

Write-Step "ensure OneDrive Loki store layout"
Ensure-LokiStore $StorePath
& $bin --store $StorePath verify
if ($LASTEXITCODE -ne 0) { throw "verify failed after store layout creation" }

Write-Step "OneDrive local sync probe"
$probe = Join-Path $StorePath "sync-probe-vm.txt"
"vm wrote $(Get-Date -Format o)" | Set-Content -Path $probe -Encoding UTF8
if (-not (Test-Path $probe)) { throw "sync probe write failed" }
Write-Host "Wrote $probe"
Write-Host "Confirm this file appears on the other machine before dogfooding."

Write-Step "adopt dry-run writes nothing"
$adoptTarget = Join-Path $TestRoot ".gitconfig"
"[user]`n`tname = Loki VM" | Set-Content -Path $adoptTarget -Encoding UTF8
& $bin --store $StorePath adopt $adoptTarget --profile $SmokeProfile --dry-run
if ($LASTEXITCODE -ne 0) { throw "adopt dry-run failed" }

Write-Step "adopt apply"
& $bin --store $StorePath adopt $adoptTarget --profile $SmokeProfile --yes
if ($LASTEXITCODE -ne 0) { throw "adopt apply failed" }
& $bin --store $StorePath verify $SmokeProfile
if ($LASTEXITCODE -ne 0) { throw "verify after adopt failed" }
& $bin --store $StorePath switch $SmokeProfile --dry-run
if ($LASTEXITCODE -ne 0) { throw "switch dry-run after adopt failed" }

Write-Step "changed adopted target blocks switch"
Add-Content -Path $adoptTarget -Value "`n[alias]`n`tco = checkout"
& $bin --store $StorePath switch $SmokeProfile --dry-run
if ($LASTEXITCODE -eq 0) {
  throw "switch dry-run succeeded after adopted target changed; expected safety block"
}
Write-Host "Safety block observed as expected."

Write-Step "re-adopt changed target"
& $bin --store $StorePath adopt $adoptTarget --profile $SmokeProfile --yes
if ($LASTEXITCODE -ne 0) { throw "re-adopt failed" }
& $bin --store $StorePath switch $SmokeProfile --dry-run
if ($LASTEXITCODE -ne 0) { throw "switch dry-run after re-adopt failed" }

Write-Step "migrate repo"
$legacy = Join-Path $TestRoot "legacy"
New-Item -ItemType Directory -Force (Join-Path $legacy "loki-vm-test") | Out-Null
'{"repo": true}' | Set-Content -Path (Join-Path $legacy "loki-vm-test\repo-settings.json") -Encoding UTF8
'{"repo": true}' | Set-Content -Path (Join-Path $TestRoot "repo-settings.json") -Encoding UTF8
& $bin --store $StorePath migrate repo $legacy --profile $SmokeProfile --dry-run
if ($LASTEXITCODE -ne 0) { throw "migrate repo dry-run failed" }
& $bin --store $StorePath migrate repo $legacy --profile $SmokeProfile --yes
if ($LASTEXITCODE -ne 0) { throw "migrate repo apply failed" }
& $bin --store $StorePath verify $SmokeProfile
if ($LASTEXITCODE -ne 0) { throw "verify after migrate repo failed" }

Write-Step "sensitive file skip"
New-Item -ItemType Directory -Force (Join-Path $legacy ".ssh") | Out-Null
"FAKE PRIVATE KEY - DO NOT IMPORT" | Set-Content -Path (Join-Path $legacy ".ssh\id_ed25519") -Encoding UTF8
& $bin --store $StorePath migrate repo $legacy --profile $SmokeProfile --dry-run
if ($LASTEXITCODE -ne 0) { throw "migrate repo dry-run with sensitive file failed" }
$leaked = Get-ChildItem $SmokeProfilePath -Recurse -File | Where-Object { $_.Name -eq "id_ed25519" }
if ($leaked) {
  throw "sensitive SSH key was copied into store: $($leaked.FullName)"
}
Write-Host "Sensitive key not copied."

Write-Step "bucket migrate and real switch"
$bucketLegacy = Join-Path $TestRoot "legacy-bucket"
New-Item -ItemType Directory -Force (Join-Path $bucketLegacy "loki-vm-test") | Out-Null
"bucket works from OneDrive store" | Set-Content -Path (Join-Path $bucketLegacy "loki-vm-test\bucket.txt") -Encoding UTF8
Remove-Item (Join-Path $TestRoot "bucket.txt") -Force -ErrorAction SilentlyContinue
& $bin --store $StorePath migrate repo $bucketLegacy --profile $SmokeProfile --bucket $SmokeBucket --yes
if ($LASTEXITCODE -ne 0) { throw "bucket migrate failed" }
& $bin --store $StorePath verify $SmokeProfile $SmokeBucket
if ($LASTEXITCODE -ne 0) { throw "verify bucket failed" }
& $bin --store $StorePath switch $SmokeProfile $SmokeBucket --yes
if ($LASTEXITCODE -ne 0) { throw "real switch failed" }
$bucketTarget = Join-Path $TestRoot "bucket.txt"
if ((Get-Content $bucketTarget -Raw).Trim() -ne "bucket works from OneDrive store") {
  throw "bucket target content mismatch"
}

Write-Step "result"
Write-Host "Windows OneDrive smoke passed."
Write-Host "Store changes should sync from: $StorePath"

if (-not $KeepData) {
  Write-Host "Keeping OneDrive store. Removing disposable test root: $TestRoot"
  Remove-Item $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
} else {
  Write-Host "Kept disposable test root: $TestRoot"
}
