---
operation: test
target: Windows 11 ARM64 VM validation and OneDrive smoke test
prerequisites:
  - tool: powershell
    version: Windows PowerShell 5.1 or later
  - tool: git
    version: any
  - tool: go
    version: ">=1.23"
  - account: null
    permissions: []
  - service: OneDrive
    requirement: signed in and sync folder available
variables:
  - name: REPO_URL
    description: Git URL for the repository.
    required: false
    default: https://github.com/asudbring/loki-profile-manager.git
    sensitive: false
  - name: REPO_DIR
    description: Windows checkout path.
    required: false
    default: $env:USERPROFILE\github\loki-profile-manager
    sensitive: false
  - name: STORE_PATH
    description: Loki store path inside OneDrive.
    required: false
    default: auto-detected OneDrive folder + \LokiProfileManager
    sensitive: false
idempotent: true
estimated_duration: 10-20 minutes
side_effects:
  - creates or updates $env:USERPROFILE\github\loki-profile-manager
  - downloads Go modules into the Go module cache
  - writes build artifact .\bin\loki.exe
  - creates or updates OneDrive\LokiProfileManager
  - creates a unique vm-smoke-<timestamp> profile and vm-bucket-<timestamp> bucket inside the Loki store
  - creates and removes $env:USERPROFILE\loki-vm-test-<timestamp> unless -KeepData is passed
  - writes OneDrive\LokiProfileManager\sync-probe-vm.txt
requires_network: true
requires_sudo: false
---

# Windows ARM64 VM Test Procedure

Purpose: validate the latest `main` checkout on a Windows 11 ARM64 VM, prove Loki can use a OneDrive-backed store safely, and provide a safe bridge to the real-profile app/manual switch validation described in [`docs/INSTALL.md`](../INSTALL.md).

Audience: AI coding agent or human operator running inside the Windows VM.

## Safety rules

- Do not print secrets or GitHub tokens.
- Do not run commands against real dotfiles except the disposable test paths created by the smoke script or a separately approved real-profile dogfood pass.
- Do not manually remove `.loki-operation.lock` unless you verified no Loki process is active on any synced machine.
- Treat `unsafe target overwrite blocked` during the smoke test as expected. The script intentionally triggers that safety block.
- For existing machines, run migration/adoption dry-runs and `switch --dry-run` before any `switch --yes`. See [`docs/INSTALL.md#first-run-path-existing-machine-with-profiles-not-migrated`](../INSTALL.md#first-run-path-existing-machine-with-profiles-not-migrated).

## What success means

The run succeeds only when all are true:

- `git status -sb` shows a clean `main` checkout.
- `go env GOOS GOARCH GOVERSION` reports `windows`, `arm64`, and Go `1.23` or later.
- `.\scripts\validate-local.ps1` ends with `validation complete`.
- `.\scripts\windows-onedrive-smoke.ps1` ends with `Windows OneDrive smoke passed.`.
- `$env:OneDrive\LokiProfileManager\sync-probe-vm.txt` exists.
- Smoke output includes a unique `Profile:  vm-smoke-...` line and `Bucket:   vm-bucket-...` line.
- The source machine later sees the same `sync-probe-vm.txt` through OneDrive.
- Optional real-profile validation, when explicitly approved, ends with `loki status --verbose` showing the requested profile/buckets and a repeat `loki switch <profile> [buckets...] --dry-run` with no unexpected blockers.

## Fast path: run this in Windows PowerShell

Open Windows PowerShell on the VM and run:

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoUrl = "https://github.com/asudbring/loki-profile-manager.git"
$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Parent = Split-Path $RepoDir -Parent

Write-Host "== tools =="
git --version
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  $goSdkRoot = Join-Path $env:USERPROFILE "go-sdk"
  $goCandidate = Get-ChildItem $goSdkRoot -Directory -Filter "go*" -ErrorAction SilentlyContinue |
    Sort-Object Name -Descending |
    ForEach-Object { Join-Path $_.FullName "go\bin\go.exe" } |
    Where-Object { Test-Path $_ } |
    Select-Object -First 1
  if ($goCandidate) {
    $env:GOROOT = Split-Path (Split-Path $goCandidate -Parent) -Parent
    $env:PATH = "$(Join-Path $env:GOROOT 'bin');$env:PATH"
    Write-Host "Activated existing user-local Go SDK: $env:GOROOT"
  }
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "Go is not on PATH. Run the Go bootstrap section below, then rerun this fast path."
}
go version

Write-Host "== clone or update repo =="
New-Item -ItemType Directory -Force $Parent | Out-Null
if (Test-Path (Join-Path $RepoDir ".git")) {
  Set-Location $RepoDir
  git fetch origin main
  git checkout main
  git pull --ff-only origin main
} else {
  git clone $RepoUrl $RepoDir
  Set-Location $RepoDir
  git checkout main
}

git status -sb
git log -1 --oneline

Write-Host "== validate local =="
.\scripts\validate-local.ps1

Write-Host "== OneDrive smoke =="
.\scripts\windows-onedrive-smoke.ps1

Write-Host "== final checks =="
$probe = Join-Path $env:OneDrive "LokiProfileManager\sync-probe-vm.txt"
if (-not (Test-Path $probe)) { throw "sync probe missing: $probe" }
Write-Host "Probe: $probe"
Get-Content $probe -Tail 1
Write-Host "Windows ARM64 VM validation passed."
```

Expected tail output:

```text
validation complete
Windows OneDrive smoke passed.
Windows ARM64 VM validation passed.
```

## Go bootstrap for Windows ARM64

Run this only if `go version` fails on the VM. It installs a user-local ARM64 Go archive under `$env:USERPROFILE\go-sdk` and updates PATH for the current PowerShell session.

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$release = Invoke-RestMethod "https://go.dev/dl/?mode=json" | Where-Object { $_.stable } | Select-Object -First 1
$file = $release.files | Where-Object { $_.os -eq "windows" -and $_.arch -eq "arm64" -and $_.kind -eq "archive" } | Select-Object -First 1
if (-not $file) { throw "No stable windows/arm64 Go archive found" }

$version = $release.version
$zip = Join-Path $env:TEMP $file.filename
$toolRoot = Join-Path $env:USERPROFILE "go-sdk\$version"
$goRoot = Join-Path $toolRoot "go"

Write-Host "Downloading $($file.filename)"
Invoke-WebRequest -Uri "https://go.dev/dl/$($file.filename)" -OutFile $zip
$actualHash = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLowerInvariant()
if ($actualHash -ne $file.sha256) { throw "Go archive hash mismatch: $actualHash" }

Remove-Item $toolRoot -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $toolRoot | Out-Null
Expand-Archive -Path $zip -DestinationPath $toolRoot -Force

$env:GOROOT = $goRoot
$env:PATH = "$goRoot\bin;$env:PATH"
go version
```

After bootstrap, rerun the fast path in the same PowerShell session. For future sessions, either add `$goRoot\bin` to the user PATH or set `$env:GOROOT` and `$env:PATH` before running validation.

## Manual step-by-step path

Use this when debugging failures.

### 1. Confirm architecture

```powershell
Write-Host "arch=$env:PROCESSOR_ARCHITECTURE"
[System.Runtime.InteropServices.RuntimeInformation]::OSDescription
```

Expected:

```text
arch=ARM64
Microsoft Windows ...
```

### 2. Update checkout

```powershell
$RepoUrl = "https://github.com/asudbring/loki-profile-manager.git"
$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
New-Item -ItemType Directory -Force (Split-Path $RepoDir -Parent) | Out-Null

if (Test-Path (Join-Path $RepoDir ".git")) {
  Set-Location $RepoDir
  git fetch origin main
  git checkout main
  git pull --ff-only origin main
} else {
  git clone $RepoUrl $RepoDir
  Set-Location $RepoDir
  git checkout main
}

git status -sb
git log -1 --oneline
```

Expected:

```text
## main...origin/main
```

`git log -1 --oneline` should print one commit hash and subject from the latest `main` commit.

### 3. Run local validation

```powershell
.\scripts\validate-local.ps1
```

Expected sections:

```text
== go env ==
windows
arm64
...
== go test ==
== go vet ==
== go mod verify ==
== go build ==
== go test -race ==
skipped: Go race detector is not supported on windows/arm64
validation complete
```

### 4. Run OneDrive smoke

```powershell
.\scripts\windows-onedrive-smoke.ps1
```

Optional parameters:

```powershell
.\scripts\windows-onedrive-smoke.ps1 -OneDrivePath "$env:OneDrive"
.\scripts\windows-onedrive-smoke.ps1 -StorePath "$env:OneDrive\LokiProfileManager"
.\scripts\windows-onedrive-smoke.ps1 -SmokeId "manual-rerun-1"
.\scripts\windows-onedrive-smoke.ps1 -SkipValidation
.\scripts\windows-onedrive-smoke.ps1 -KeepData
```

Expected successful tail:

```text
Profile:  vm-smoke-20260503235959
Bucket:   vm-bucket-20260503235959
...
Sensitive key not copied.
== register machine for smoke policy ==
...
== bucket migrate and real switch ==
...
== result ==
Windows OneDrive smoke passed.
Store changes should sync from: %USERPROFILE%\OneDrive\LokiProfileManager
```

Expected intentional error text during the run:

```text
unsafe target overwrite blocked: ... existing file hash differs from Loki state ...
```

This is not a failure when followed by:

```text
Safety block observed as expected.
```

### 5. Confirm OneDrive sync probe exists on VM

```powershell
$probe = Join-Path $env:OneDrive "LokiProfileManager\sync-probe-vm.txt"
Test-Path $probe
Get-Content $probe -Tail 1
```

Expected:

```text
True
vm wrote 2026-05-03T17:50:06.2767188-05:00
```

### 6. Ask the source machine to confirm sync

The source machine should see the same file under its OneDrive cloud storage path. On macOS this has been:

```text
$HOME/Library/CloudStorage/OneDrive-Personal/LokiProfileManager/sync-probe-vm.txt
```

If the source machine does not see the probe, wait for OneDrive sync and check OneDrive status before changing Loki code.

## Optional dogfood verification

Run only after the source machine has adopted a harmless dogfood bucket into the same OneDrive store. For real profile/app validation, prefer the fresh-machine or existing-machine procedures in [`docs/INSTALL.md`](../INSTALL.md) and keep the same dry-run-before-write rule.

```powershell
Set-Location $env:USERPROFILE\github\loki-profile-manager
$store = Join-Path $env:OneDrive "LokiProfileManager"

.\bin\loki.exe --store $store machine register --allow-profile work --allow-bucket dogfood
.\bin\loki.exe --store $store verify work dogfood
.\bin\loki.exe --store $store switch work dogfood --dry-run
```

If dry-run shows only safe operations against disposable paths, run:

```powershell
.\bin\loki.exe --store $store switch work dogfood --yes
Get-Content "$env:USERPROFILE\loki-dogfood\probe.txt"
```

Do not use this optional section for real dotfiles until the dry-run plan has been reviewed.

## Optional real-profile app/manual switch validation

Use this only after the Loki store already contains migrated/adopted real profiles and the operator has approved a real switch.

```powershell
$store = Join-Path $env:OneDrive "LokiProfileManager"
$profile = "work"
$buckets = @("content-dev")

.\bin\loki.exe --store $store machine register --allow-profile $profile --allow-bucket $buckets
.\bin\loki.exe --store $store verify $profile @buckets
.\bin\loki.exe --store $store switch $profile @buckets --dry-run
```

If the dry-run shows only expected managed targets and no blockers, activate:

```powershell
.\bin\loki.exe --store $store switch $profile @buckets --yes
.\bin\loki.exe --store $store status --verbose
.\bin\loki.exe --store $store switch $profile @buckets --dry-run
```

If the only dry-run blockers are unmanaged file/directory targets and the synced Loki store should win, preserve those local blockers outside the store and activate:

```powershell
.\bin\loki.exe --store $store switch $profile @buckets --backup-unmanaged --yes
.\bin\loki.exe --store $store status --verbose
```

Expected output includes `Backed up unmanaged targets` and `Backup root:`. Do not use this branch when local files should become store source of truth, or when blockers include broken symlinks, managed hash mismatches, obsolete changed targets, or render drift.

If activation blocks with safe copied-target local drift that should be preserved, use the capture form:

```powershell
.\bin\loki.exe --store $store switch $profile @buckets --capture-local --yes
```

Manual app checks after activation:

- Open a fresh Windows Terminal PowerShell tab and confirm the Loki profile line/prompt state reflects the active profile/buckets.
- Run `echo $env:STARSHIP_CONFIG`, `echo $env:LOKI_PROFILE`, and `starship prompt`; there must be no legacy profile-repo errors.
- Open Git Bash and run `echo $LOKI_PROFILE` plus `starship prompt`.
- Check VS Code, Codex, Pi, Claude/Copilot, Git, and Warp config paths only for legacy path references; do not print secret values or full config contents.

## Troubleshooting

### `Go is not on PATH`

Run the Go bootstrap section, then rerun the fast path in the same PowerShell session.

### `OneDrive folder not found`

Confirm OneDrive is signed in and `$env:OneDrive` exists:

```powershell
$env:OneDrive
Test-Path $env:OneDrive
```

If OneDrive uses a nonstandard path, pass it explicitly:

```powershell
$customOneDrive = Join-Path $env:USERPROFILE "OneDrive"
.\scripts\windows-onedrive-smoke.ps1 -OneDrivePath $customOneDrive
```

### Git authentication failure

Authenticate without printing tokens:

```powershell
gh auth login
```

Or configure Git Credential Manager. Do not paste tokens into chat or logs.

### Operation lock timeout

A failed or interrupted run can leave:

```text
$env:OneDrive\LokiProfileManager\.loki-operation.lock
```

First check no Loki process is active:

```powershell
Get-Process loki -ErrorAction SilentlyContinue
```

Only if no Loki process is active on any synced machine, remove the stale lock:

```powershell
Remove-Item "$env:OneDrive\LokiProfileManager\.loki-operation.lock" -Force
```

Then rerun the failed command.

### Smoke test left disposable data

Default behavior removes `$env:USERPROFILE\loki-vm-test-<timestamp>` and keeps the OneDrive store. If `-KeepData` was used, clean manually:

```powershell
Remove-Item "$env:USERPROFILE\loki-vm-test-*" -Recurse -Force -ErrorAction SilentlyContinue
```

Do not delete the OneDrive store unless the source machine owner confirms it is disposable.
