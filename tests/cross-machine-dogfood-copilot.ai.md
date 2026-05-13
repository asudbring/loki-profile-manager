# Copilot CLI Instructions: Cross-Machine OneDrive Dogfood

You are Copilot CLI running inside the Windows 11 ARM64 VM. Validate a harmless Loki profile that was adopted on the source machine and synced through OneDrive.

For full fresh-machine or existing-machine migration/switch workflows, use `docs/INSTALL.md`. This prompt stays limited to a disposable cross-machine target.

## Operating rules

- Use Windows PowerShell commands.
- Do not print secrets, GitHub tokens, credential helper output, or environment dumps.
- Do not modify source code.
- Do not commit or push.
- Do not run against real dotfiles.
- Use only the `dogfood-crossos` profile and target `%USERPROFILE%\loki-dogfood\probe.txt`.
- Always run `switch --dry-run` before `switch --yes`.
- Run `switch --yes` only if the dry-run plan targets `%USERPROFILE%\loki-dogfood\probe.txt` and reports no unsafe overwrite or unexpected target.
- Always run `snapshots restore <switch-snapshot> --dry-run --target %USERPROFILE%\loki-dogfood\probe.txt` before matching `--yes --target %USERPROFILE%\loki-dogfood\probe.txt`.
- Never run restore `--yes` against real dotfiles.
- Stop on first unexpected failure and report the failing command plus last 40 lines of output.
- Do not remove `.loki-operation.lock` unless no `loki` process is active and the lock is clearly stale.

## Success criteria

The dogfood pass is valid only when all are true:

- Repo is on latest `main`.
- Go is available as `windows/arm64` with version `1.23` or newer.
- OneDrive store exists at `%OneDrive%\LokiProfileManager`.
- Store contains `profiles\dogfood-crossos\core\manifest.yaml`.
- `loki machine register --allow-profile dogfood-crossos --allow-profile work` passes.
- `loki verify dogfood-crossos` passes without `machine.record_missing`.
- `loki switch dogfood-crossos --dry-run` shows only the harmless dogfood target.
- `loki switch dogfood-crossos --yes` passes.
- `%USERPROFILE%\loki-dogfood\probe.txt` exists and contains text written by the source machine after switch.
- `loki snapshots restore <switch-snapshot> --dry-run --target %USERPROFILE%\loki-dogfood\probe.txt` records a targeted guard, prints `Target filter:`, and mentions only the harmless dogfood target.
- `loki snapshots restore <switch-snapshot> --yes --target %USERPROFILE%\loki-dogfood\probe.txt` passes and reports a pre-restore snapshot.

## Task

Run this PowerShell block exactly. It activates a user-local Go SDK if Go is not already on PATH.

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Store = Join-Path $env:OneDrive "LokiProfileManager"
$Profile = "dogfood-crossos"
$Target = Join-Path $env:USERPROFILE "loki-dogfood\probe.txt"
$RestoreState = "skipped"
$BeforeHash = "missing"
$AfterRestoreHash = "missing"

Write-Host "== preflight =="
if ($env:PROCESSOR_ARCHITECTURE -ne "ARM64") { throw "Expected ARM64 VM, got $env:PROCESSOR_ARCHITECTURE" }
if (-not $env:OneDrive) { throw "OneDrive environment variable is not set. Sign into OneDrive first." }
if (-not (Test-Path $Store)) { throw "OneDrive Loki store missing: $Store" }
if (-not (Test-Path (Join-Path $Store "profiles\dogfood-crossos\core\manifest.yaml"))) {
  throw "dogfood-crossos manifest missing. Wait for OneDrive sync, then rerun."
}

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
.\bin\loki.exe --store $Store machine register --allow-profile $Profile --allow-profile work
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
if ($dryRunExit -ne 0) { throw "switch dry-run failed" }
$dryRunText = ($dryRunOutput | Out-String)
$targetPattern = [regex]::Escape($Target)
if ($dryRunText -match "unsafe target overwrite blocked") { throw "dry-run reported unsafe overwrite" }
if ($dryRunText -notmatch $targetPattern) { throw "dry-run did not mention expected target: $Target" }
$unexpectedTargets = $dryRunOutput | Where-Object { $_ -match '^- ' -and $_ -notmatch $targetPattern }
if ($unexpectedTargets) { throw "dry-run mentioned unexpected targets: $($unexpectedTargets -join '; ')" }

if (Test-Path $Target) {
  $BeforeHash = (Get-FileHash -Algorithm SHA256 $Target).Hash.ToLowerInvariant()
}

Write-Host "== apply switch =="
$switchOutput = & .\bin\loki.exe --store $Store switch $Profile --yes 2>&1
$switchExit = $LASTEXITCODE
$switchOutput | ForEach-Object { $_ }
if ($switchExit -ne 0) { throw "switch apply failed" }
$snapshotLine = $switchOutput | Where-Object { $_ -match '^Snapshot: ' } | Select-Object -First 1
if (-not $snapshotLine) { throw "switch did not print snapshot id" }
$SnapshotID = ($snapshotLine -replace '^Snapshot:\s*', '').Trim()
if (-not (Test-Path $Target)) { throw "target missing after switch: $Target" }
$targetText = Get-Content $Target -Raw
$targetText.Trim()
if ($targetText -notmatch "mac wrote") { throw "target content did not contain source-machine marker" }

Write-Host "== targeted restore snapshot dry-run =="
$restoreDryOutput = & .\bin\loki.exe --store $Store snapshots restore $SnapshotID --dry-run --target $Target 2>&1
$restoreDryExit = $LASTEXITCODE
$restoreDryOutput | ForEach-Object { $_ }
if ($restoreDryExit -ne 0) { throw "targeted snapshots restore dry-run failed" }
$restoreDryText = ($restoreDryOutput | Out-String)
if ($restoreDryText -notmatch "Guard: recorded") { throw "targeted restore dry-run did not record guard" }
if ($restoreDryText -notmatch [regex]::Escape($Target)) { throw "targeted restore dry-run did not mention expected target: $Target" }
if ($restoreDryText -notmatch "Target filter:") { throw "targeted restore dry-run did not print target filter" }
$restoreUnexpectedTargets = $restoreDryOutput | Where-Object { $_ -match '^- ' -and $_ -notmatch [regex]::Escape($Target) }
if ($restoreUnexpectedTargets) { throw "targeted restore dry-run mentioned unexpected targets: $($restoreUnexpectedTargets -join '; ')" }

Write-Host "== targeted restore snapshot yes =="
$restoreYesOutput = & .\bin\loki.exe --store $Store snapshots restore $SnapshotID --yes --target $Target 2>&1
$restoreYesExit = $LASTEXITCODE
$restoreYesOutput | ForEach-Object { $_ }
if ($restoreYesExit -ne 0) { throw "targeted snapshots restore yes failed" }
$restoreYesText = ($restoreYesOutput | Out-String)
if ($restoreYesText -notmatch "Loki snapshot restore complete") { throw "targeted restore yes did not report completion" }
if ($restoreYesText -notmatch "Pre-restore snapshot:") { throw "targeted restore yes did not report pre-restore snapshot" }
if ($restoreYesText -notmatch [regex]::Escape($Target)) { throw "targeted restore yes did not mention expected target" }
$RestoreState = "passed"
if (Test-Path $Target) {
  $AfterRestoreHash = (Get-FileHash -Algorithm SHA256 $Target).Hash.ToLowerInvariant()
}

Write-Host "Cross-machine dogfood passed."
Write-Host "SNAPSHOT: $SnapshotID"
Write-Host "RESTORE: $RestoreState"
Write-Host "BEFORE_HASH: $BeforeHash"
Write-Host "AFTER_RESTORE_HASH: $AfterRestoreHash"
```

## Final report format

Return only this summary:

```text
RESULT: PASS|FAIL
COMMIT: <git log -1 --oneline>
GO: <go env GOOS GOARCH GOVERSION, single line>
PROFILE: dogfood-crossos
MACHINE_REGISTER: passed|failed
VERIFY: passed|failed
DRY_RUN: passed|failed
SWITCH: passed|failed|skipped
SNAPSHOT: <switch snapshot id or missing>
RESTORE: passed|failed|skipped
TARGET: <target path or missing>
TARGET_CONTENT: <last line after switch or missing>
BEFORE_HASH: <sha256 or missing>
AFTER_RESTORE_HASH: <sha256 or missing>
NOTES: <only failures, skipped items, sync wait, or lock caveats>
```
