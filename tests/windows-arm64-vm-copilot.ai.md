# Copilot CLI Instructions: Windows ARM64 VM Validation

You are Copilot CLI running inside a Windows 11 ARM64 VM. Validate `loki-profile-manager` on Windows/ARM64 and prove its store works inside OneDrive.

## Operating rules

- Use Windows PowerShell commands.
- Do not print secrets, GitHub tokens, credential helper output, or environment dumps.
- Do not modify source code.
- Do not commit or push.
- Stop on first unexpected failure and report the last failing command plus last 40 lines of output.
- Treat `unsafe target overwrite blocked` as expected only when followed by `Safety block observed as expected.`
- Do not remove `.loki-operation.lock` unless no `loki` process is active and the lock is clearly stale.

## Success criteria

The validation passes only if all are true:

- Repo is on clean `main` after pulling latest `origin/main`.
- `go env GOOS GOARCH GOVERSION` reports `windows`, `arm64`, and Go `1.23` or newer.
- `scripts\validate-local.ps1` ends with `validation complete`.
- `scripts\windows-onedrive-smoke.ps1` ends with `Windows OneDrive smoke passed.`
- `%OneDrive%\LokiProfileManager\sync-probe-vm.txt` exists.
- Final output contains `Windows ARM64 VM validation passed.`

## Task

Run the following PowerShell block. If Go is missing or older than 1.23, run the Go bootstrap block first, then rerun the validation block.

### Validation block

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoUrl = "https://github.com/asudbring/loki-profile-manager.git"
$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Parent = Split-Path $RepoDir -Parent

Write-Host "== preflight =="
Write-Host "arch=$env:PROCESSOR_ARCHITECTURE"
if ($env:PROCESSOR_ARCHITECTURE -ne "ARM64") { throw "Expected ARM64 VM, got $env:PROCESSOR_ARCHITECTURE" }
if (-not $env:OneDrive) { throw "OneDrive environment variable is not set. Sign into OneDrive first." }
if (-not (Test-Path $env:OneDrive)) { throw "OneDrive path missing: $env:OneDrive" }

Write-Host "== tools =="
git --version
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "Go is not on PATH. Run the Go bootstrap block, then rerun this validation block."
}
go version
$goInfo = go env GOOS GOARCH GOVERSION
$goInfo
$goLines = $goInfo -split "`n"
if ($goLines[0].Trim() -ne "windows") { throw "Expected GOOS=windows, got $($goLines[0])" }
if ($goLines[1].Trim() -ne "arm64") { throw "Expected GOARCH=arm64, got $($goLines[1])" }
if (-not ($goLines[2].Trim() -match '^go1\.(\d+)')) { throw "Could not parse Go version: $($goLines[2])" }
if ([int]$Matches[1] -lt 23) { throw "Go 1.23+ required. Run the Go bootstrap block, then rerun this validation block." }

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
$status = git status -sb
$status
if (-not ($status -match '^## main\.\.\.origin/main')) { throw "Repo not on clean tracked main: $status" }

$probe = Join-Path $env:OneDrive "LokiProfileManager\sync-probe-vm.txt"
if (-not (Test-Path $probe)) { throw "sync probe missing: $probe" }
Write-Host "Probe: $probe"
Get-Content $probe -Tail 1
Write-Host "Windows ARM64 VM validation passed."
```

### Go bootstrap block

Run only if the validation block says Go is missing or older than 1.23. It installs a user-local Windows/ARM64 Go SDK under `%USERPROFILE%\go-sdk` and updates PATH only for this PowerShell session.

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
go env GOOS GOARCH GOVERSION
```

After bootstrap succeeds, rerun the validation block in the same PowerShell session.

## Expected success tail

```text
validation complete
Windows OneDrive smoke passed.
Probe: C:\Users\<user>\OneDrive\LokiProfileManager\sync-probe-vm.txt
vm wrote <timestamp>
Windows ARM64 VM validation passed.
```

## Final report format

Return only this summary:

```text
RESULT: PASS|FAIL
COMMIT: <git log -1 --oneline>
GO: <go env GOOS GOARCH GOVERSION, single line>
VALIDATE_LOCAL: passed|failed
ONEDRIVE_SMOKE: passed|failed
PROBE: <probe path or missing>
NOTES: <only failures, skipped items, or sync caveats>
```
