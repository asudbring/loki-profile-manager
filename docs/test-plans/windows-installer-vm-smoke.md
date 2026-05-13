# Windows Installer VM Smoke Test Plan

Goal: prove Windows installer and uninstaller install Loki correctly, preserve user data on uninstall, and surface Developer Mode / symlink readiness accurately.

Target: Parallels Windows VM, Windows ARM64 or AMD64.

## Preconditions

- Windows VM running.
- Normal non-admin PowerShell available.
- Optional: Administrator PowerShell available for Developer Mode branch.
- Release assets available locally or copied to VM:
  - `loki_<version>_windows_<arch>.zip`
  - `checksums.txt`
  - `install.ps1`
  - `uninstall.ps1`
- Developer Mode state known before test.
- No secrets printed in logs.

## Variables

```powershell
$Version = "v0.1.6"  # replace with the release tag under test
$Arch = if ([Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$Assets = "$env:USERPROFILE\Downloads\loki-release-test"
$Archive = Join-Path $Assets "loki_${Version}_windows_${Arch}.zip"
$Checksums = Join-Path $Assets "checksums.txt"
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\Loki"
$DisposableStore = Join-Path $env:TEMP "loki-installer-store"
$DisposableHome = Join-Path $env:TEMP "loki-installer-home"
```

## Baseline capture

```powershell
$ErrorActionPreference = "Stop"
$beforePath = [Environment]::GetEnvironmentVariable("Path", "User")
$devModeBefore = (Get-ItemProperty `
  -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' `
  -Name AllowDevelopmentWithoutDevLicense `
  -ErrorAction SilentlyContinue).AllowDevelopmentWithoutDevLicense
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
[pscustomobject]@{
  ComputerName = $env:COMPUTERNAME
  Arch = $Arch
  IsAdmin = $isAdmin
  DeveloperMode = $devModeBefore
  InstallDir = $InstallDir
}
```

## Install: per-user normal shell

```powershell
Set-Location $Assets
.\install.ps1 `
  -Version $Version `
  -ArchivePath $Archive `
  -ChecksumsPath $Checksums `
  -InstallDir $InstallDir `
  -AddToPath
```

Expected:

- Exit code 0.
- `$InstallDir\loki.exe` exists.
- User PATH contains `$InstallDir`.
- Output prints version, install path, PATH status, Developer Mode status, symlink probe result.
- If symlink unavailable and `-RequireSymlink` not set, install succeeds with warning.

## New terminal verification

Open a new normal PowerShell after PATH update.

```powershell
Get-Command loki
loki --version
loki doctor --json | ConvertFrom-Json | Select-Object Healthy, Version, StorePath
loki tui --help
```

Expected:

- `Get-Command loki` resolves to `$InstallDir\loki.exe`.
- `loki --version` equals `$Version`.
- `doctor --json` parses.
- `tui --help` exits 0.

## Persistent store smoke

```powershell
Remove-Item -Recurse -Force $DisposableStore -ErrorAction SilentlyContinue
loki store init $DisposableStore
loki store status
loki machine register --allow-profile work --active-profile work
loki verify work
loki switch work --dry-run
```

Expected:

- Store initialized.
- Machine registered.
- Verify exits 0.
- Switch dry-run exits 0 with 0 operations for empty profile.

## Symlink switch smoke

Create a disposable manifest with one symlink operation under the user home.

```powershell
$Target = Join-Path $env:USERPROFILE "loki-installer-symlink-probe.txt"
$StoreFile = Join-Path $DisposableStore "profiles\work\core\files\symlink-probe.txt"
$Manifest = Join-Path $DisposableStore "profiles\work\core\manifest.yaml"
Remove-Item -Force $Target -ErrorAction SilentlyContinue
"hello from Loki" | Set-Content -LiteralPath $StoreFile -Encoding UTF8
@"
version: 1
name: work-core
files:
  - id: symlink-probe
    source: files/symlink-probe.txt
    target: ~/loki-installer-symlink-probe.txt
    mode: symlink
skills: []
ignore: []
merge_rules: {}
targets: {}
"@ | Set-Content -LiteralPath $Manifest -Encoding UTF8

loki verify work
loki switch work --dry-run
loki switch work --yes
$item = Get-Item -LiteralPath $Target -Force
[pscustomobject]@{
  Exists = Test-Path $Target
  IsReparse = [bool]($item.Attributes -band [IO.FileAttributes]::ReparsePoint)
  Target = $item.Target
  Content = Get-Content -LiteralPath $Target -Raw
}
```

Expected when Developer Mode is enabled or shell has symlink privilege:

- `switch --yes` exits 0.
- Target exists.
- Target is reparse point.
- Content matches source.

Expected when Developer Mode is disabled and shell is non-admin:

- `switch --yes` fails with remediation mentioning Developer Mode or elevated shell.
- No copy fallback occurs.
- Target is absent or rollback restored previous state.

## Developer Mode enablement branch

Run only when testing installer Developer Mode behavior.

### Non-admin with `-RequireSymlink`

```powershell
.\install.ps1 `
  -Version $Version `
  -ArchivePath $Archive `
  -ChecksumsPath $Checksums `
  -InstallDir $InstallDir `
  -Force `
  -RequireSymlink
```

Expected if symlink unavailable:

- Fails with clear remediation.
- Does not silently change Developer Mode.

### Admin with `-EnableDeveloperMode`

Open elevated PowerShell.

```powershell
.\install.ps1 `
  -Version $Version `
  -ArchivePath $Archive `
  -ChecksumsPath $Checksums `
  -InstallDir $InstallDir `
  -Force `
  -EnableDeveloperMode `
  -RequireSymlink
```

Expected:

- Developer Mode registry value set if needed.
- Symlink probe rerun.
- Install succeeds if symlink probe passes.
- Output records Developer Mode status.

## Default uninstall

```powershell
Set-Location $Assets
.\uninstall.ps1 -InstallDir $InstallDir
```

Expected:

- `$InstallDir\loki.exe` removed.
- User PATH no longer contains `$InstallDir`.
- Install dir removed if empty.
- `%LOCALAPPDATA%\loki-profile-manager` preserved.
- `$DisposableStore` preserved.
- Managed target `$Target` preserved; installer does not deactivate profiles.
- Developer Mode setting unchanged.

Open new PowerShell:

```powershell
$cmd = Get-Command loki -ErrorAction SilentlyContinue
if ($cmd -and $cmd.Source -like "$InstallDir*") { throw "loki still resolves to uninstalled path" }
```

## Reinstall idempotence

```powershell
Set-Location $Assets
.\install.ps1 -Version $Version -ArchivePath $Archive -ChecksumsPath $Checksums -InstallDir $InstallDir -AddToPath
loki --version
.\uninstall.ps1 -InstallDir $InstallDir
```

Expected:

- Reinstall succeeds over clean state.
- Uninstall succeeds again.

## Destructive uninstall branch: disposable data only

Only run against disposable paths from this test.

```powershell
.\uninstall.ps1 -InstallDir $InstallDir -RemoveState
.\uninstall.ps1 -InstallDir $InstallDir -RemoveStore $DisposableStore
# Type exact confirmation if prompted: DELETE LOKI STORE
```

Expected:

- Local Loki state removed only when `-RemoveState` confirmed.
- Disposable store removed only when `-RemoveStore` confirmed.
- Real OneDrive store is never touched.

## Automation harness

After `install.ps1` and `uninstall.ps1` exist, most of this plan can run headless with:

```powershell
.\scripts\windows-installer-smoke.ps1 `
  -Version $Version `
  -AssetsDir $Assets `
  -ArchivePath $Archive `
  -ChecksumsPath $Checksums `
  -InstallScript (Join-Path $Assets 'install.ps1') `
  -UninstallScript (Join-Path $Assets 'uninstall.ps1') `
  -RunSymlinkSwitch `
  -RequireSymlinkSwitch
```

From macOS/Parallels, invoke with `prlctl exec "Windows 11" --current-user powershell.exe -NoProfile -ExecutionPolicy Bypass -File <script> ...` after copying assets into the VM.

Automated coverage:

- Local archive install.
- Optional user PATH update check.
- Version, doctor JSON, TUI help.
- Disposable store init, machine register, verify, switch dry-run.
- Optional symlink switch smoke.
- Default uninstall.
- Preservation checks for local state and disposable store.

Manual/UAC coverage still needed for:

- Developer Mode enablement from non-admin via UAC.
- Any prompt-driven admin elevation UX.
- Full interactive TUI navigation beyond `tui --help`.

## Pass criteria

- Install succeeds from local checked archive.
- PATH update works in new shell.
- Installed binary version matches release.
- Doctor and TUI help run.
- Persistent store and machine register work through installed binary.
- Symlink behavior matches Developer Mode/elevation state.
- Default uninstall removes installer-owned files and PATH only.
- Default uninstall preserves app state, synced store, managed targets, and Developer Mode.
- Reinstall/uninstall is idempotent.
