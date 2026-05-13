# Copilot CLI Instructions: Real Dotfile OneDrive Dogfood

You are Copilot CLI running inside the Windows 11 ARM64 VM. Validate the first real, low-risk Loki dotfile profile that was adopted on the Mac and synced through OneDrive.

For full fresh-machine or existing-machine migration/switch workflows, use `docs/INSTALL.md`. This prompt remains a narrow low-risk restore/switch dogfood, not a full app-profile migration.

## Operating rules

- Use Windows PowerShell commands.
- Do not print secrets, GitHub tokens, credential helper output, environment dumps, or full dotfile contents.
- Do not modify source code.
- Do not commit or push.
- Do not run against `.ssh`, `.env`, credentials, tokens, or private keys.
- Use only profile `work` and targets `%USERPROFILE%\.config\git\ignore` and `%USERPROFILE%\.gitconfig`.
- Always run `switch --dry-run` before `switch --yes`.
- Run `switch --yes` only if dry-run succeeds and mentions only expected targets.
- If a copied managed target changed locally and the operator explicitly wants to preserve it, the general command is `loki switch <profile> [buckets...] --capture-local --yes`; do not use it in this narrow prompt unless instructed.
- If dry-run blocks with `unsafe target overwrite blocked` for expected targets, treat that as a safety pass and do not run `switch --yes`.
- Stop on first unexpected failure and report the failing command plus last 40 lines of output.
- Do not remove `.loki-operation.lock` unless no `loki` process is active and the lock is clearly stale.

## Success criteria

The dogfood pass is valid when all are true:

- Repo is on latest `main`.
- Go is available as `windows/arm64` with version `1.23` or newer.
- OneDrive store exists at `%OneDrive%\LokiProfileManager`.
- Store contains `profiles\work\core\manifest.yaml`.
- The work manifest includes targets `~/.config/git/ignore` and `~/.gitconfig`.
- The work manifest does not include stale disposable targets such as `loki-vm-test`.
- `loki machine register --allow-profile work` passes.
- `loki verify work` passes without `machine.record_missing`.
- `loki switch work --dry-run` either:
  - succeeds and shows only `%USERPROFILE%\.config\git\ignore` and `%USERPROFILE%\.gitconfig`, then `switch --yes` passes; or
  - blocks only because an expected target already exists unmanaged, proving unsafe overwrite protection.
- `loki --verbose status` reports active profile `work` and lists the expected managed targets.
- If `switch --yes` runs, `loki snapshots list`, `loki snapshots show <latest>`, and `loki snapshots restore <latest> --dry-run` pass and mention only expected targets.

## Task

Run this PowerShell block exactly. It activates a user-local Go SDK if Go is not already on PATH.

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Store = Join-Path $env:OneDrive "LokiProfileManager"
$Profile = "work"
$ExpectedTargets = @(
  (Join-Path $env:USERPROFILE ".config\git\ignore"),
  (Join-Path $env:USERPROFILE ".gitconfig")
)
$SwitchState = "skipped"
$SnapshotState = "skipped"
$TargetHashes = @{}

Write-Host "== preflight =="
if ($env:PROCESSOR_ARCHITECTURE -ne "ARM64") { throw "Expected ARM64 VM, got $env:PROCESSOR_ARCHITECTURE" }
if (-not $env:OneDrive) { throw "OneDrive environment variable is not set. Sign into OneDrive first." }
if (-not (Test-Path $Store)) { throw "OneDrive Loki store missing: $Store" }
$Manifest = Join-Path $Store "profiles\work\core\manifest.yaml"
if (-not (Test-Path $Manifest)) { throw "work manifest missing. Wait for OneDrive sync, then rerun." }
$manifestText = Get-Content $Manifest -Raw
if ($manifestText -notmatch [regex]::Escape("~/.config/git/ignore")) { throw "work manifest does not contain ~/.config/git/ignore. Wait for OneDrive sync, then rerun." }
if ($manifestText -notmatch [regex]::Escape("~/.gitconfig")) { throw "work manifest does not contain ~/.gitconfig. Wait for OneDrive sync, then rerun." }
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
$operationLines = $dryRunOutput | Where-Object { $_ -match '^- ' }
$unexpectedTargets = @()
foreach ($line in $operationLines) {
  $matched = $false
  foreach ($target in $ExpectedTargets) {
    if ($line -match [regex]::Escape($target)) { $matched = $true }
  }
  if (-not $matched) { $unexpectedTargets += $line }
}
if ($unexpectedTargets) { throw "dry-run mentioned unexpected targets: $($unexpectedTargets -join '; ')" }

if ($dryRunExit -ne 0) {
  $mentionsExpectedTarget = $false
  foreach ($target in $ExpectedTargets) {
    if ($dryRunText -match [regex]::Escape($target)) { $mentionsExpectedTarget = $true }
  }
  if ($dryRunText -match "unsafe target overwrite blocked" -and $mentionsExpectedTarget) {
    Write-Host "Safety block observed for expected target. Not applying switch."
    $SwitchState = "blocked"
  } else {
    throw "switch dry-run failed unexpectedly"
  }
} else {
  foreach ($target in $ExpectedTargets) {
    if ($dryRunText -notmatch [regex]::Escape($target)) { throw "dry-run did not mention expected target: $target" }
  }
  if ($dryRunText -match "unsafe target overwrite blocked") { throw "dry-run reported unsafe overwrite despite zero exit" }

  Write-Host "== apply switch =="
  .\bin\loki.exe --store $Store switch $Profile --yes
  if ($LASTEXITCODE -ne 0) { throw "switch apply failed" }
  foreach ($target in $ExpectedTargets) {
    if (-not (Test-Path $target)) { throw "target missing after switch: $target" }
  }
  $SwitchState = "passed"
}

foreach ($target in $ExpectedTargets) {
  if (Test-Path $target) {
    $TargetHashes[$target] = (Get-FileHash -Algorithm SHA256 $target).Hash.ToLowerInvariant()
  } else {
    $TargetHashes[$target] = "missing"
  }
}

Write-Host "== status audit =="
$statusOutput = & .\bin\loki.exe --store $Store --verbose status 2>&1
$statusExit = $LASTEXITCODE
$statusOutput | ForEach-Object { $_ }
if ($statusExit -ne 0) { throw "status audit failed" }
$statusText = ($statusOutput | Out-String)
if ($statusText -notmatch "Active profile: work") { throw "status did not report active profile work" }
if ($statusText -notmatch "Managed target list:") { throw "status did not list managed targets in verbose mode" }
foreach ($target in $ExpectedTargets) {
  if ($statusText -notmatch [regex]::Escape($target)) { throw "status did not list expected target: $target" }
}

if ($SwitchState -eq "passed") {
  Write-Host "== snapshot audit =="
  $snapListOutput = & .\bin\loki.exe snapshots list 2>&1
  $snapListExit = $LASTEXITCODE
  $snapListOutput | ForEach-Object { $_ }
  if ($snapListExit -ne 0) { throw "snapshots list failed" }
  $snapListText = ($snapListOutput | Out-String)
  if ($snapListText -notmatch "Loki snapshots") { throw "snapshots list did not print expected heading" }
  if ($snapListText -notmatch "targets=2") { throw "latest snapshot list did not report 2 targets" }
  $snapshotMatches = [regex]::Matches($snapListText, '(?m)^-\s+(\S+)')
  if ($snapshotMatches.Count -lt 1) { throw "snapshots list did not include a snapshot id" }
  $SnapshotID = $snapshotMatches[0].Groups[1].Value

  $snapShowOutput = & .\bin\loki.exe snapshots show $SnapshotID 2>&1
  $snapShowExit = $LASTEXITCODE
  $snapShowOutput | ForEach-Object { $_ }
  if ($snapShowExit -ne 0) { throw "snapshots show failed" }
  $snapShowText = ($snapShowOutput | Out-String)
  foreach ($target in $ExpectedTargets) {
    if ($snapShowText -notmatch [regex]::Escape($target)) { throw "snapshots show did not list expected target: $target" }
  }
  $snapshotTargetLines = $snapShowOutput | Where-Object { $_ -match '^- ' }
  $snapshotUnexpectedTargets = @()
  foreach ($line in $snapshotTargetLines) {
    $matched = $false
    foreach ($target in $ExpectedTargets) {
      if ($line -match [regex]::Escape($target)) { $matched = $true }
    }
    if (-not $matched) { $snapshotUnexpectedTargets += $line }
  }
  if ($snapshotUnexpectedTargets) { throw "snapshots show mentioned unexpected targets: $($snapshotUnexpectedTargets -join '; ')" }
  $restorePreviewOutput = & .\bin\loki.exe snapshots restore $SnapshotID --dry-run 2>&1
  $restorePreviewExit = $LASTEXITCODE
  $restorePreviewOutput | ForEach-Object { $_ }
  if ($restorePreviewExit -ne 0) { throw "snapshots restore --dry-run failed" }
  $restorePreviewText = ($restorePreviewOutput | Out-String)
  if ($restorePreviewText -notmatch "no files or local state were changed") { throw "snapshots restore dry-run did not report read-only mode" }
  foreach ($target in $ExpectedTargets) {
    if ($restorePreviewText -notmatch [regex]::Escape($target)) { throw "snapshots restore dry-run did not list expected target: $target" }
  }
  $restorePreviewTargetLines = $restorePreviewOutput | Where-Object { $_ -match '^- ' }
  $restorePreviewUnexpectedTargets = @()
  foreach ($line in $restorePreviewTargetLines) {
    $matched = $false
    foreach ($target in $ExpectedTargets) {
      if ($line -match [regex]::Escape($target)) { $matched = $true }
    }
    if (-not $matched) { $restorePreviewUnexpectedTargets += $line }
  }
  if ($restorePreviewUnexpectedTargets) { throw "snapshots restore dry-run mentioned unexpected targets: $($restorePreviewUnexpectedTargets -join '; ')" }
  $SnapshotState = "passed"
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
Write-Host "STATUS: passed"
Write-Host "SNAPSHOTS: $SnapshotState"
Write-Host "TARGETS: $($ExpectedTargets -join '; ')"
$TargetHashSummary = (($TargetHashes.GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" } | Sort-Object) -join '; ')
Write-Host "TARGET_HASHES: $TargetHashSummary"
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
STATUS: passed|failed|skipped
SNAPSHOTS: passed|failed|skipped
TARGETS: <target paths or missing>
TARGET_HASHES: <path=sha256/path=missing pairs>
NOTES: <only failures, safety block, sync wait, or lock caveats>
```
