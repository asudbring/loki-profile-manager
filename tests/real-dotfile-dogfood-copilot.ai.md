# Copilot CLI Instructions: Real Dotfile OneDrive Dogfood

You are Copilot CLI running inside the Windows 11 ARM64 VM. Validate the first real, low-risk Loki dotfile profile that was adopted on the Mac and synced through OneDrive.

## Operating rules

- Use Windows PowerShell commands.
- Do not print secrets, GitHub tokens, credential helper output, environment dumps, or full dotfile contents.
- Do not modify source code.
- Do not commit or push.
- Do not run against `.gitconfig`, `.ssh`, `.env`, credentials, tokens, or private keys.
- Use only profile `work` and target `%USERPROFILE%\.config\git\ignore`.
- Always run `switch --dry-run` before `switch --yes`.
- Run `switch --yes` only if dry-run succeeds and mentions exactly the expected target.
- If dry-run blocks with `unsafe target overwrite blocked` for the expected target, treat that as a safety pass and do not run `switch --yes`.
- Stop on first unexpected failure and report the failing command plus last 40 lines of output.
- Do not remove `.loki-operation.lock` unless no `loki` process is active and the lock is clearly stale.

## Success criteria

The dogfood pass is valid when all are true:

- Repo is on latest `main`.
- Go is available as `windows/arm64` with version `1.23` or newer.
- OneDrive store exists at `%OneDrive%\LokiProfileManager`.
- Store contains `profiles\work\core\manifest.yaml`.
- The work manifest includes target `~/.config/git/ignore`.
- The work manifest does not include stale disposable targets such as `loki-vm-test`.
- `loki machine register --allow-profile work` passes.
- `loki verify work` passes without `machine.record_missing`.
- `loki switch work --dry-run` either:
  - succeeds and shows only `%USERPROFILE%\.config\git\ignore`, then `switch --yes` passes; or
  - blocks only because that expected target already exists unmanaged, proving unsafe overwrite protection.

## Task

Run this PowerShell block exactly. It activates a user-local Go SDK if Go is not already on PATH.

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Store = Join-Path $env:OneDrive "LokiProfileManager"
$Profile = "work"
$ExpectedTarget = Join-Path $env:USERPROFILE ".config\git\ignore"
$SwitchState = "skipped"
$TargetHash = "missing"

Write-Host "== preflight =="
if ($env:PROCESSOR_ARCHITECTURE -ne "ARM64") { throw "Expected ARM64 VM, got $env:PROCESSOR_ARCHITECTURE" }
if (-not $env:OneDrive) { throw "OneDrive environment variable is not set. Sign into OneDrive first." }
if (-not (Test-Path $Store)) { throw "OneDrive Loki store missing: $Store" }
$Manifest = Join-Path $Store "profiles\work\core\manifest.yaml"
if (-not (Test-Path $Manifest)) { throw "work manifest missing. Wait for OneDrive sync, then rerun." }
$manifestText = Get-Content $Manifest -Raw
if ($manifestText -notmatch [regex]::Escape("~/.config/git/ignore")) { throw "work manifest does not contain ~/.config/git/ignore. Wait for OneDrive sync, then rerun." }
if ($manifestText -match "loki-vm-test") { throw "work manifest still contains stale loki-vm-test targets. Wait for cleanup sync, then rerun." }

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
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw "Go is not on PATH and no user-local Go SDK was found." }
$goInfo = go env GOOS GOARCH GOVERSION
$goInfo
$goLines = $goInfo -split "`n"
if ($goLines[0].Trim() -ne "windows") { throw "Expected GOOS=windows, got $($goLines[0])" }
if ($goLines[1].Trim() -ne "arm64") { throw "Expected GOARCH=arm64, got $($goLines[1])" }
if (-not ($goLines[2].Trim() -match '^go1\.(\d+)')) { throw "Could not parse Go version: $($goLines[2])" }
if ([int]$Matches[1] -lt 23) { throw "Go 1.23+ required." }

Write-Host "== update repo =="
Set-Location $RepoDir
git fetch origin main
git checkout main
git pull --ff-only origin main
git status -sb
$commit = git log -1 --oneline
$commit

Write-Host "== build =="
New-Item -ItemType Directory -Force .\bin | Out-Null
go build -o .\bin\loki.exe .\cmd\loki

Write-Host "== register machine =="
.\bin\loki.exe --store $Store machine register --allow-profile $Profile
if ($LASTEXITCODE -ne 0) { throw "machine register failed" }

Write-Host "== verify profile =="
$verifyOutput = & .\bin\loki.exe --store $Store verify $Profile 2>&1
$verifyExit = $LASTEXITCODE
$verifyOutput | ForEach-Object { $_ }
if ($verifyExit -ne 0) { throw "verify failed" }
if (($verifyOutput | Out-String) -match "machine\.record_missing") { throw "verify reported machine.record_missing after registration" }

Write-Host "== dry-run switch =="
$dryRunOutput = & .\bin\loki.exe --store $Store switch $Profile --dry-run 2>&1
$dryRunExit = $LASTEXITCODE
$dryRunOutput | ForEach-Object { $_ }
$dryRunText = ($dryRunOutput | Out-String)
$targetPattern = [regex]::Escape($ExpectedTarget)
$operationLines = $dryRunOutput | Where-Object { $_ -match '^- ' }
$unexpectedTargets = $operationLines | Where-Object { $_ -notmatch $targetPattern }
if ($unexpectedTargets) { throw "dry-run mentioned unexpected targets: $($unexpectedTargets -join '; ')" }

if ($dryRunExit -ne 0) {
  if ($dryRunText -match "unsafe target overwrite blocked" -and $dryRunText -match $targetPattern) {
    Write-Host "Safety block observed for expected target. Not applying switch."
    $SwitchState = "blocked"
  } else {
    throw "switch dry-run failed unexpectedly"
  }
} else {
  if ($dryRunText -notmatch $targetPattern) { throw "dry-run did not mention expected target: $ExpectedTarget" }
  if ($dryRunText -match "unsafe target overwrite blocked") { throw "dry-run reported unsafe overwrite despite zero exit" }

  Write-Host "== apply switch =="
  .\bin\loki.exe --store $Store switch $Profile --yes
  if ($LASTEXITCODE -ne 0) { throw "switch apply failed" }
  if (-not (Test-Path $ExpectedTarget)) { throw "target missing after switch: $ExpectedTarget" }
  $SwitchState = "passed"
}

if (Test-Path $ExpectedTarget) {
  $TargetHash = (Get-FileHash -Algorithm SHA256 $ExpectedTarget).Hash.ToLowerInvariant()
}

Write-Host "Real dotfile dogfood completed."
Write-Host "RESULT: PASS"
Write-Host "COMMIT: $commit"
Write-Host "GO: $(($goInfo -join ' ').Trim())"
Write-Host "PROFILE: $Profile"
Write-Host "MACHINE_REGISTER: passed"
Write-Host "VERIFY: passed"
Write-Host "DRY_RUN: passed"
Write-Host "SWITCH: $SwitchState"
Write-Host "TARGET: $ExpectedTarget"
Write-Host "TARGET_HASH: $TargetHash"
Write-Host "NOTES:"
```

## Final report format

Return only this summary:

```text
RESULT: PASS|FAIL
COMMIT: <git log -1 --oneline>
GO: <go env GOOS GOARCH GOVERSION, single line>
PROFILE: work
MACHINE_REGISTER: passed|failed
VERIFY: passed|failed
DRY_RUN: passed|failed
SWITCH: passed|blocked|failed|skipped
TARGET: <target path or missing>
TARGET_HASH: <sha256 or missing>
NOTES: <only failures, safety block, sync wait, or lock caveats>
```
