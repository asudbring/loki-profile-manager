# Windows VM Infisical + TUI Smoke Test Plan

Goal: verify latest Windows ARM64 build, Infisical machine identity auth, real interactive TUI behavior, and the post-install app/manual switch path on the Parallels Windows VM.

Do not paste or save `INFISICAL_CLIENT_SECRET`, `INFISICAL_TOKEN`, or secret values into this repo, the synced Loki store, screenshots, logs, or chat. For fresh-machine and existing-machine setup details, see [`../INSTALL.md`](../INSTALL.md).

## Paths

- Repo: `%USERPROFILE%\github\loki-profile-manager`
- Binary: `%USERPROFILE%\github\loki-profile-manager\bin\loki.exe`
- Store: `%USERPROFILE%\OneDrive\LokiProfileManager`
- Plan: `%USERPROFILE%\github\loki-profile-manager\docs\test-plans\windows-vm-infisical-tui-smoke.md`

## 1. Build check

Open Windows Terminal in the VM:

```powershell
cd %USERPROFILE%\github\loki-profile-manager

$go = Get-ChildItem $env:USERPROFILE\go-sdk -Directory -Filter go* |
  Sort-Object Name -Descending |
  ForEach-Object { Join-Path $_.FullName 'go\bin\go.exe' } |
  Where-Object { Test-Path $_ } |
  Select-Object -First 1
$env:GOROOT = Split-Path (Split-Path $go -Parent) -Parent
$env:PATH = "$(Join-Path $env:GOROOT 'bin');$env:PATH"

git status --short
git log -1 --oneline
go version
go build -o .\bin\loki.exe .\cmd\loki
.\bin\loki.exe --version
.\bin\loki.exe --store $env:OneDrive\LokiProfileManager status
```

Expected:

- `git status --short` empty.
- Latest commit is expected test build commit.
- Build succeeds.
- `status` shows configured OneDrive store or clear setup message. No secret values printed.

## 2. Infisical environment check

Run in same Windows Terminal. This checks presence only; it does not print values.

```powershell
'INFISICAL_API_URL',
'INFISICAL_HOST',
'INFISICAL_AUTH_METHOD',
'INFISICAL_CLIENT_ID',
'INFISICAL_CLIENT_SECRET',
'INFISICAL_PROJECT_ID' |
  ForEach-Object {
    $present = [bool][Environment]::GetEnvironmentVariable($_, 'User') -or [bool](Get-Item "Env:\$_" -ErrorAction SilentlyContinue)
    "{0}: {1}" -f $_, $(if ($present) { 'set' } else { 'missing' })
  }
```

Expected:

- All six report `set` on a configured test machine.
- If any are missing, configure from secure channel, then open a new Windows Terminal.

## 3. Infisical readiness through Loki

Run:

```powershell
.\bin\loki.exe secrets status
.\bin\loki.exe secrets status --json
.\bin\loki.exe secrets check __LOKI_INFISICAL_READINESS_PROBE__
```

Expected:

- `secrets status` exits success and says Infisical is authenticated/ready.
- JSON output contains readiness metadata only. No token, client secret, or secret values.
- `secrets check __LOKI_INFISICAL_READINESS_PROBE__` either reports the probe name as available or missing. If missing, nonzero exit is acceptable; missing probe still proves auth/project routing when `secrets status` is ready.
- Any error must not include `INFISICAL_CLIENT_SECRET`, `INFISICAL_TOKEN`, or returned secret values.

## 4. Optional direct machine-token mint check

Only if debugging. This writes token to current process env and does not print it.

```powershell
$env:INFISICAL_TOKEN = infisical login `
  --method=universal-auth `
  --client-id $env:INFISICAL_CLIENT_ID `
  --client-secret $env:INFISICAL_CLIENT_SECRET `
  --domain $env:INFISICAL_API_URL `
  --plain `
  --silent

if ($env:INFISICAL_TOKEN) { 'INFISICAL_TOKEN: set' } else { 'INFISICAL_TOKEN: missing' }
.\bin\loki.exe secrets status
```

Expected:

- Token presence prints only `set`/`missing`, never value.
- Loki status still ready.

## 5. TUI launch and dashboard

Run:

```powershell
.\bin\loki.exe --store $env:OneDrive\LokiProfileManager tui
```

Expected:

- TUI opens in Windows Terminal. No `interactive terminal` error.
- Dashboard renders cleanly.
- Store path is `%USERPROFILE%\OneDrive\LokiProfileManager`.
- Secrets row says ready/authenticated or shows a clear warning without secret values.

## 6. TUI navigation smoke

From dashboard:

| Key | Expected |
| --- | --- |
| `r` | Refresh completes; no stuck spinner. |
| `d` | Doctor screen renders. `esc` returns. |
| `m` | Machine screen renders. `esc` returns. |
| `s` | Secrets screen renders readiness. No secret values. `esc` returns. |
| `p` | Profiles screen renders profile catalog. `esc` returns. |
| `n` | Snapshots screen renders. `enter` shows selected snapshot if present. `d` runs full restore dry-run preview only. `t` runs targeted dry-run after snapshot loaded/target selected. `esc` returns. |

Pass criteria: all screens readable, footer/help visible, no panic, no layout corruption, no leaked secrets.

## 7. TUI sync dry-run

From dashboard press `y`, then on Sync screen press `d`.

Expected:

- Dry-run scans provider conflict-copy files.
- If no conflicts, says no deletable conflict copies.
- If conflicts exist, required confirmation phrase is shown.
- Do not execute deletion unless intentionally testing cleanup.
- `esc` returns dashboard.

## 8. TUI switch dry-run

From dashboard press `w`.

Optional keys:

- `up`/`down` or `k`/`j`: select profile.
- `left`/`right` or `h`/`l`: select bucket.
- `space`: toggle bucket.
- `d`: run switch dry-run.

Expected:

- Dry-run renders planned operations.
- Safety blockers visible if targets changed.
- No real switch unless you press `x`, type exact `SWITCH <profile> [buckets...]`, and press `enter`.
- For this smoke, stop after dry-run unless intentionally testing real activation.

## 9. Optional real app/manual switch check

Run only when the OneDrive store already has migrated/adopted real profiles and you intend to validate a real switch.

```powershell
$Store = "$env:OneDrive\LokiProfileManager"
$Profile = "work"
$Buckets = @("content-dev")

.\bin\loki.exe --store $Store machine register --allow-profile $Profile --allow-bucket $Buckets
.\bin\loki.exe --store $Store verify $Profile @Buckets
.\bin\loki.exe --store $Store switch $Profile @Buckets --dry-run
```

Expected:

- Dry-run shows the expected managed target count.
- No unexpected unmanaged overwrite blockers.
- Any copied-target local drift is understood before proceeding.

If the dry-run is clean:

```powershell
.\bin\loki.exe --store $Store switch $Profile @Buckets --yes
.\bin\loki.exe --store $Store status --verbose
.\bin\loki.exe --store $Store switch $Profile @Buckets --dry-run
```

If the only blocker is safe copied-target drift that should be written back to the store before switching:

```powershell
.\bin\loki.exe --store $Store switch $Profile @Buckets --capture-local --yes
```

Manual app checks:

- Open a fresh Windows Terminal PowerShell tab. Confirm the Loki profile line/prompt state matches the active profile/buckets.
- Run `echo $env:STARSHIP_CONFIG`, `echo $env:LOKI_PROFILE`, and `starship prompt`. No legacy profile-repo errors should appear.
- Open Git Bash and run `echo $LOKI_PROFILE` plus `starship prompt`.
- Check VS Code, Codex, Pi, Claude/Copilot, Git, and Warp config paths for stale legacy references without printing secret values or full config files.

## 10. Exit and cleanup check

Press `q`, then run:

```powershell
git status --short
.\bin\loki.exe --store $env:OneDrive\LokiProfileManager verify
```

Expected:

- Returned to normal PowerShell prompt.
- Cursor and terminal usable.
- `git status --short` empty.
- `verify` succeeds or reports only known store/profile issues. No secret values printed.

## Fail criteria

- TUI panic or hang >10 seconds after operation.
- Wrong store path.
- Secret/client secret/token printed anywhere.
- `secrets status` not ready with configured env.
- `secrets get`/`run` errors mention raw tokens, client secrets, or secret values.
- Real switch/sync/restore happens without explicit confirmation.
- App/manual switch check shows stale legacy profile-repo references after activation.
- Repo becomes dirty after smoke.
