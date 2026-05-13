# Copilot CLI Instructions: Consent-Gated Real Dotfile Targeted Restore

You are Copilot CLI running inside the Windows 11 ARM64 VM. Prepare and, only with explicit human consent, execute a targeted Loki snapshot restore for one low-risk real dotfile synced through OneDrive.

For install, migration, adoption, and first real activation workflows, use `docs/INSTALL.md`. This prompt covers targeted restore only.

This prompt is intentionally two-phase:

1. Phase 1 is dry-run only and always safe to run.
2. Phase 2 restores with `--yes` only after the human copies the exact consent phrase printed by Phase 1.

## Operating rules

- Use Windows PowerShell commands.
- Do not print secrets, GitHub tokens, credential helper output, environment dumps, or full dotfile contents.
- Do not modify source code.
- Do not commit or push.
- Do not run against `.ssh`, `.env`, credentials, tokens, private keys, or any sensitive-looking path.
- Use only profile `work`.
- Use only one target per run:
  - default allowed target: `%USERPROFILE%\.config\git\ignore`
  - alternate allowed target: `%USERPROFILE%\.gitconfig`
- Always use `snapshots restore <snapshot> --target <target>` for real dotfiles.
- Never run full restore against real dotfiles. This command is forbidden: `snapshots restore <snapshot> --yes` without `--target`.
- Always run restore `--dry-run --target <target>` before matching `--yes --target <target>`.
- Run restore `--yes --target <target>` only if all are true:
  - Phase 1 passed.
  - Dry-run output recorded a guard.
  - Dry-run output printed `Target filter:`.
  - Dry-run output mentioned only the selected target.
  - Human gave the exact consent phrase printed by Phase 1.
- Stop on first unexpected failure and report the failing command plus last 40 lines of output.
- Do not remove `.loki-operation.lock` unless no `loki` process is active and the lock is clearly stale.

## Success criteria

Phase 1 pass is valid when all are true:

- Repo is on latest `main`.
- Go is available as `windows/arm64` with version `1.23` or newer.
- OneDrive store exists at `%OneDrive%\LokiProfileManager`.
- Store contains `profiles\work\core\manifest.yaml`.
- Manifest contains the selected target.
- `loki machine register --allow-profile work` passes.
- `loki verify work` passes without `machine.record_missing`.
- A snapshot containing the selected target is found.
- `loki snapshots restore <snapshot> --dry-run --target <target>` records a guard, prints `Target filter:`, and mentions only the selected target.
- No files are changed by Phase 1.

Phase 2 pass is valid only when all Phase 1 criteria pass and:

- Human consent phrase exactly matches Phase 1 output.
- `loki snapshots restore <snapshot> --yes --target <target>` passes.
- Restore output reports `Loki snapshot restore complete` and `Pre-restore snapshot:`.
- Before/after hashes are reported for the selected target.

## Phase 1: dry-run and consent phrase

Run this PowerShell block exactly. It activates a user-local Go SDK if Go is not already on PATH.

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Store = Join-Path $env:OneDrive "LokiProfileManager"
$Profile = "work"
$SelectedTarget = Join-Path $env:USERPROFILE ".config\git\ignore"
$AllowedTargets = @(
  (Join-Path $env:USERPROFILE ".config\git\ignore"),
  (Join-Path $env:USERPROFILE ".gitconfig")
)
$SnapshotID = "missing"
$BeforeHash = "missing"

Write-Host "== preflight =="
if ($env:PROCESSOR_ARCHITECTURE -ne "ARM64") { throw "Expected ARM64 VM, got $env:PROCESSOR_ARCHITECTURE" }
if (-not $env:OneDrive) { throw "OneDrive environment variable is not set. Sign into OneDrive first." }
if (-not (Test-Path $Store)) { throw "OneDrive Loki store missing: $Store" }
if ($AllowedTargets -notcontains $SelectedTarget) { throw "selected target is not allowed: $SelectedTarget" }
if ($SelectedTarget -match '\\.ssh|\\.env|credential|token|private|id_rsa|id_ed25519|\.pem$|\.key$') { throw "selected target looks sensitive: $SelectedTarget" }
$Manifest = Join-Path $Store "profiles\work\core\manifest.yaml"
if (-not (Test-Path $Manifest)) { throw "work manifest missing. Wait for OneDrive sync, then rerun." }
$manifestText = Get-Content $Manifest -Raw
$targetSpec = $SelectedTarget.Replace($env:USERPROFILE, "~").Replace("\", "/")
if ($manifestText -notmatch [regex]::Escape($targetSpec)) { throw "work manifest does not contain selected target $targetSpec. Wait for OneDrive sync, then rerun." }

Write-Host "== tools =="
$gitExe = "C:\Program Files\Git\cmd\git.exe"
if (-not (Test-Path $gitExe)) { $gitExe = (Get-Command git -ErrorAction Stop).Source }
& $gitExe --version
$goExe = ""
if (Get-Command go -ErrorAction SilentlyContinue) {
  $goExe = (Get-Command go).Source
} else {
  $goSdkRoot = Join-Path $env:USERPROFILE "go-sdk"
  $goExe = Get-ChildItem $goSdkRoot -Directory -Filter "go*" -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -notmatch '\s' } |
    Sort-Object Name -Descending |
    ForEach-Object { Join-Path $_.FullName "go\bin\go.exe" } |
    Where-Object { Test-Path $_ } |
    Select-Object -First 1
  if ($goExe) {
    $env:GOROOT = Split-Path (Split-Path $goExe -Parent) -Parent
    $env:PATH = "$(Join-Path $env:GOROOT 'bin');$env:PATH"
    Write-Host "Activated existing user-local Go SDK: $env:GOROOT"
  }
}
if (-not $goExe -or -not (Test-Path $goExe)) { throw "Go is not on PATH and no user-local Go SDK was found." }
$goInfo = & $goExe env GOOS GOARCH GOVERSION
$goInfo
$goLines = $goInfo -split "`n"
if ($goLines[0].Trim() -ne "windows") { throw "Expected GOOS=windows, got $($goLines[0])" }
if ($goLines[1].Trim() -ne "arm64") { throw "Expected GOARCH=arm64, got $($goLines[1])" }
if (-not ($goLines[2].Trim() -match '^go1\.(\d+)')) { throw "Could not parse Go version: $($goLines[2])" }
if ([int]$Matches[1] -lt 23) { throw "Go 1.23+ required." }

Write-Host "== update repo =="
Set-Location $RepoDir
& $gitExe fetch origin main
& $gitExe checkout main
& $gitExe pull --ff-only origin main
& $gitExe status -sb
$commit = & $gitExe log -1 --oneline
$commit

Write-Host "== build =="
New-Item -ItemType Directory -Force .\bin | Out-Null
& $goExe build -o .\bin\loki.exe .\cmd\loki

Write-Host "== register machine =="
.\bin\loki.exe --store $Store machine register --allow-profile $Profile
if ($LASTEXITCODE -ne 0) { throw "machine register failed" }

Write-Host "== verify profile =="
$verifyOutput = & .\bin\loki.exe --store $Store verify $Profile 2>&1
$verifyExit = $LASTEXITCODE
$verifyOutput | ForEach-Object { $_ }
if ($verifyExit -ne 0) { throw "verify failed" }
if (($verifyOutput | Out-String) -match "machine\.record_missing") { throw "verify reported machine.record_missing after registration" }

Write-Host "== find snapshot containing target =="
$snapListOutput = & .\bin\loki.exe --store $Store snapshots list 2>&1
$snapListExit = $LASTEXITCODE
$snapListOutput | ForEach-Object { $_ }
if ($snapListExit -ne 0) { throw "snapshots list failed" }
$snapshotMatches = [regex]::Matches(($snapListOutput | Out-String), '(?m)^-\s+(\S+)')
if ($snapshotMatches.Count -lt 1) { throw "snapshots list did not include a snapshot id" }
foreach ($match in $snapshotMatches) {
  $candidate = $match.Groups[1].Value
  $showOutput = & .\bin\loki.exe --store $Store snapshots show $candidate 2>&1
  if ($LASTEXITCODE -ne 0) { continue }
  $showText = ($showOutput | Out-String)
  if ($showText -match [regex]::Escape($SelectedTarget)) {
    $SnapshotID = $candidate
    break
  }
}
if ($SnapshotID -eq "missing") { throw "no snapshot found containing selected target: $SelectedTarget" }
if (Test-Path $SelectedTarget) { $BeforeHash = (Get-FileHash -Algorithm SHA256 $SelectedTarget).Hash.ToLowerInvariant() }

Write-Host "== targeted restore dry-run =="
$restoreDryOutput = & .\bin\loki.exe --store $Store snapshots restore $SnapshotID --dry-run --target $SelectedTarget 2>&1
$restoreDryExit = $LASTEXITCODE
$restoreDryOutput | ForEach-Object { $_ }
if ($restoreDryExit -ne 0) { throw "targeted snapshots restore dry-run failed" }
$restoreDryText = ($restoreDryOutput | Out-String)
if ($restoreDryText -notmatch "Guard: recorded") { throw "targeted restore dry-run did not record guard" }
if ($restoreDryText -notmatch "Target filter:") { throw "targeted restore dry-run did not print target filter" }
if ($restoreDryText -notmatch [regex]::Escape($SelectedTarget)) { throw "targeted restore dry-run did not mention selected target: $SelectedTarget" }
$restoreUnexpectedTargets = $restoreDryOutput | Where-Object { $_ -match '^- ' -and $_ -notmatch [regex]::Escape($SelectedTarget) }
if ($restoreUnexpectedTargets) { throw "targeted restore dry-run mentioned unexpected targets: $($restoreUnexpectedTargets -join '; ')" }

$ConsentPhrase = "RESTORE REAL DOTFILE $SnapshotID $SelectedTarget"
Write-Host "Phase 1 passed. No files changed."
Write-Host "RESULT: CONSENT_REQUIRED"
Write-Host "COMMIT: $commit"
Write-Host "GO: $(($goInfo -join ' ').Trim())"
Write-Host "PROFILE: $Profile"
Write-Host "SNAPSHOT: $SnapshotID"
Write-Host "TARGET: $SelectedTarget"
Write-Host "BEFORE_HASH: $BeforeHash"
Write-Host "CONSENT_PHRASE: $ConsentPhrase"
Write-Host "NEXT: Ask the human to paste the exact consent phrase before running Phase 2."
```

## Phase 2: execute targeted restore only after consent

Do not run this block until the human pasted the exact `CONSENT_PHRASE` printed by Phase 1.

Before running, set these three variables from Phase 1:

- `$SnapshotID`
- `$SelectedTarget`
- `$ConsentPhrase`

```powershell
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RepoDir = Join-Path $env:USERPROFILE "github\loki-profile-manager"
$Store = Join-Path $env:OneDrive "LokiProfileManager"
$Profile = "work"
$SnapshotID = "PASTE_SNAPSHOT_ID_FROM_PHASE_1"
$SelectedTarget = "PASTE_TARGET_FROM_PHASE_1"
$ConsentPhrase = "PASTE_EXACT_CONSENT_PHRASE_FROM_HUMAN"
$ExpectedConsentPhrase = "RESTORE REAL DOTFILE $SnapshotID $SelectedTarget"
$AllowedTargets = @(
  (Join-Path $env:USERPROFILE ".config\git\ignore"),
  (Join-Path $env:USERPROFILE ".gitconfig")
)

if ($SnapshotID -eq "PASTE_SNAPSHOT_ID_FROM_PHASE_1") { throw "snapshot id placeholder not replaced" }
if ($SelectedTarget -eq "PASTE_TARGET_FROM_PHASE_1") { throw "target placeholder not replaced" }
if ($ConsentPhrase -eq "PASTE_EXACT_CONSENT_PHRASE_FROM_HUMAN") { throw "consent phrase placeholder not replaced" }
if ($ConsentPhrase -cne $ExpectedConsentPhrase) { throw "consent phrase mismatch; refusing real dotfile restore" }
if ($AllowedTargets -notcontains $SelectedTarget) { throw "selected target is not allowed: $SelectedTarget" }
if ($SelectedTarget -match '\\.ssh|\\.env|credential|token|private|id_rsa|id_ed25519|\.pem$|\.key$') { throw "selected target looks sensitive: $SelectedTarget" }

Set-Location $RepoDir
if (Test-Path $SelectedTarget) {
  $BeforeHash = (Get-FileHash -Algorithm SHA256 $SelectedTarget).Hash.ToLowerInvariant()
} else {
  $BeforeHash = "missing"
}

Write-Host "== re-run targeted restore dry-run =="
$restoreDryOutput = & .\bin\loki.exe --store $Store snapshots restore $SnapshotID --dry-run --target $SelectedTarget 2>&1
$restoreDryExit = $LASTEXITCODE
$restoreDryOutput | ForEach-Object { $_ }
if ($restoreDryExit -ne 0) { throw "targeted snapshots restore dry-run failed" }
$restoreDryText = ($restoreDryOutput | Out-String)
if ($restoreDryText -notmatch "Guard: recorded") { throw "targeted restore dry-run did not record guard" }
if ($restoreDryText -notmatch "Target filter:") { throw "targeted restore dry-run did not print target filter" }
if ($restoreDryText -notmatch [regex]::Escape($SelectedTarget)) { throw "targeted restore dry-run did not mention selected target: $SelectedTarget" }
$restoreUnexpectedTargets = $restoreDryOutput | Where-Object { $_ -match '^- ' -and $_ -notmatch [regex]::Escape($SelectedTarget) }
if ($restoreUnexpectedTargets) { throw "targeted restore dry-run mentioned unexpected targets: $($restoreUnexpectedTargets -join '; ')" }

Write-Host "== targeted restore yes =="
$restoreYesOutput = & .\bin\loki.exe --store $Store snapshots restore $SnapshotID --yes --target $SelectedTarget 2>&1
$restoreYesExit = $LASTEXITCODE
$restoreYesOutput | ForEach-Object { $_ }
if ($restoreYesExit -ne 0) { throw "targeted snapshots restore yes failed" }
$restoreYesText = ($restoreYesOutput | Out-String)
if ($restoreYesText -notmatch "Loki snapshot restore complete") { throw "targeted restore yes did not report completion" }
if ($restoreYesText -notmatch "Pre-restore snapshot:") { throw "targeted restore yes did not report pre-restore snapshot" }
if ($restoreYesText -notmatch [regex]::Escape($SelectedTarget)) { throw "targeted restore yes did not mention selected target" }
if (Test-Path $SelectedTarget) {
  $AfterRestoreHash = (Get-FileHash -Algorithm SHA256 $SelectedTarget).Hash.ToLowerInvariant()
} else {
  $AfterRestoreHash = "missing"
}

Write-Host "Consent-gated real dotfile targeted restore passed."
Write-Host "RESULT: PASS"
Write-Host "PROFILE: $Profile"
Write-Host "SNAPSHOT: $SnapshotID"
Write-Host "RESTORE: passed"
Write-Host "TARGET: $SelectedTarget"
Write-Host "BEFORE_HASH: $BeforeHash"
Write-Host "AFTER_RESTORE_HASH: $AfterRestoreHash"
Write-Host "NOTES: exact human consent phrase matched"
```

## Final report format

Return only this summary:

```text
RESULT: CONSENT_REQUIRED|PASS|FAIL
COMMIT: <git log -1 --oneline or missing>
GO: <go env GOOS GOARCH GOVERSION, single line or missing>
PROFILE: work
SNAPSHOT: <snapshot id or missing>
RESTORE: dry-run-only|passed|failed|skipped
TARGET: <selected target path or missing>
BEFORE_HASH: <sha256 or missing>
AFTER_RESTORE_HASH: <sha256 or missing>
CONSENT: required|matched|missing|mismatch
NOTES: <only failures, consent wait, sync wait, or lock caveats>
```
