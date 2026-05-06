# Installer + Release Plan

## Status

Implemented now:

- `scripts/install.ps1` / `scripts/uninstall.ps1`.
- `scripts/install.sh` / `scripts/uninstall.sh`.
- Installer assets bundled inside release archives and copied standalone.
- `release-manifest.json` with version, commit, build date, Go version, asset sizes, and SHA-256 hashes.
- `.github/workflows/ci.yml` package + installer smoke jobs.
- `.github/workflows/release.yml` smoke-before-release publishing gate.

Remaining dogfood validation:

- Run the Windows VM installer automation against packaged assets.
- Add any elevated Windows test runner setup needed for admin-only branches.
- Consider `loki doctor` symlink-probe JSON improvement after installer smoke stabilizes.

## Recommendation

Start with script installers bundled with release archives. Do not start with MSI/pkg/deb/Homebrew/winget yet.

Reason:

- Repo is private dogfood, so public package managers add friction.
- Loki is a single static Go binary; script install/uninstall is enough for first release.
- Windows symlink/Developer Mode checks need product-specific logic that is easier in PowerShell first.
- MSI/MSIX/code signing/notarization can come after installer behavior settles.

## Release asset set

Keep existing binary archives:

```text
loki_<version>_windows_amd64.zip
loki_<version>_windows_arm64.zip
loki_<version>_darwin_amd64.tar.gz
loki_<version>_darwin_arm64.tar.gz
loki_<version>_linux_amd64.tar.gz
loki_<version>_linux_arm64.tar.gz
checksums.txt
```

Add installer assets:

```text
install.ps1
uninstall.ps1
install.sh
uninstall.sh
release-manifest.json
```

Also bundle installer scripts inside archives:

- Windows zip: `loki.exe`, `install.ps1`, `uninstall.ps1`, `README.md`, `CHANGELOG.md`.
- macOS/Linux tarballs: `loki`, `install.sh`, `uninstall.sh`, `README.md`, `CHANGELOG.md`.

## Install locations

| OS | Default user install | Admin/system option | Local state preserved by app |
|---|---|---|---|
| Windows | `%LOCALAPPDATA%\Programs\Loki` | `C:\Program Files\Loki` | `%LOCALAPPDATA%\loki-profile-manager` |
| macOS | `$HOME/.local/bin` | `/usr/local/bin` | `~/Library/Application Support/loki-profile-manager` |
| Linux | `$HOME/.local/bin` | `/usr/local/bin` | `~/.local/state/loki-profile-manager` |

Installer must treat install path and Loki store path as separate concepts.

Default installer should not create or change OneDrive/Dropbox store unless user passes explicit `-StorePath` / `--store-path`.

## Windows installer design

### `install.ps1` parameters

```powershell
.\install.ps1 `
  -Version v0.1.0 `
  -ArchivePath .\loki_v0.1.0_windows_arm64.zip `
  -ChecksumsPath .\checksums.txt `
  -InstallDir "$env:LOCALAPPDATA\Programs\Loki" `
  -AddToPath
```

Recommended parameters:

| Parameter | Purpose |
|---|---|
| `-Version <version>` | Expected release version. Optional when `-ArchivePath` is explicit and filename contains version. |
| `-ArchivePath <zip>` | Install from local archive. Primary dogfood path. |
| `-ChecksumsPath <checksums.txt>` | Required checksum file. |
| `-InstallDir <path>` | Override install folder. |
| `-AddToPath` | Add install folder to user PATH. Default true for interactive. |
| `-NoPath` | Do not edit PATH. Useful in CI. |
| `-Force` | Overwrite existing install. |
| `-StorePath <path>` | Optional: run `loki store init/use` after install. |
| `-RequireSymlink` | Fail install if Loki cannot create symlinks. |
| `-EnableDeveloperMode` | If symlink probe fails, try to enable Developer Mode. Requires admin. |
| `-ElevateForDeveloperMode` | Prompt UAC and run only Developer Mode setup elevated. |
| `-Repo <owner/repo>` | Optional download mode for private GitHub release. |
| `-Token <token>` / `GH_TOKEN` | Private GitHub release auth. Never print token. |

### Install flow

1. Detect Windows and architecture.
2. Resolve archive:
   - Prefer `-ArchivePath`.
   - Else download release asset using `GH_TOKEN`/`-Token`.
3. Verify checksum against `checksums.txt`.
4. Extract archive to temp directory.
5. Run extracted `loki.exe --version`; verify expected version if provided.
6. Create install directory.
7. Copy `loki.exe`, docs, `uninstall.ps1`, and installer metadata.
8. Add install directory to user PATH unless `-NoPath`.
9. Run post-install smoke:
   - `loki.exe --version`
   - `loki.exe doctor --json`
   - `loki.exe tui --help`
10. Run symlink capability probe.
11. If `-StorePath` provided:
   - If valid store exists: `loki store use <path>`.
   - If missing/empty: `loki store init <path>`.
   - If invalid non-empty: fail with remediation.
12. Print final status and next commands.

## Windows Developer Mode / symlink plan

Do not rely only on registry or admin state. Final truth must be a real symlink probe.

### Checks

1. Elevated process:

```powershell
$current = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($current)
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
```

2. Developer Mode registry:

```powershell
$devMode = (Get-ItemProperty `
  -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' `
  -Name AllowDevelopmentWithoutDevLicense `
  -ErrorAction SilentlyContinue).AllowDevelopmentWithoutDevLicense -eq 1
```

3. Real symlink probe:

- Create temp folder.
- Create target file.
- Try symlink creation.
- Remove temp files.

Important: PowerShell `New-Item -ItemType SymbolicLink` may still fail when Go `os.Symlink` succeeds under Developer Mode. Best final probe should use installed Loki or a small Go-backed helper. Preferred: add a future `loki doctor --symlink-probe` or include symlink probe in `loki doctor` JSON.

### Developer Mode enablement

If symlink probe fails:

- If `-RequireSymlink` not set: install succeeds with warning.
- If `-RequireSymlink` set: install fails unless Developer Mode/elevation remediation succeeds.
- If `-EnableDeveloperMode` and process is elevated:
  - Set `HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock\AllowDevelopmentWithoutDevLicense = 1`.
  - Optionally set `AllowAllTrustedApps = 1` only if required; avoid if not needed.
  - Re-run symlink probe.
- If `-ElevateForDeveloperMode` and not elevated:
  - Relaunch a minimal dev-mode setup command with `Start-Process -Verb RunAs`.
  - Re-run symlink probe after elevated command exits.

Installer should not silently flip Developer Mode without a flag or explicit prompt.

## Windows uninstall design

### `uninstall.ps1` default behavior

Remove installer-owned files only:

1. Resolve install directory.
2. Remove install directory from user PATH.
3. Remove installed `loki.exe`, copied docs, installer metadata, and uninstall script.
4. Remove install directory if empty.
5. Print warning to open new terminal.

Preserve by default:

- `%LOCALAPPDATA%\loki-profile-manager`
- OneDrive/Dropbox Loki store
- managed dotfile targets
- Developer Mode setting

Developer Mode is shared OS state. Do not disable by default, even if installer enabled it.

### Optional destructive uninstall flags

| Flag | Behavior |
|---|---|
| `-RemoveState` | Delete `%LOCALAPPDATA%\loki-profile-manager` after confirmation. |
| `-RemoveStore <path>` | Delete specified store after exact `DELETE LOKI STORE` confirmation. Disposable-store tests only. |
| `-DisableDeveloperModeIfSetByInstaller` | Optional future; risky because other dev tools may depend on it. |

Do not unlink or restore managed dotfiles during installer uninstall. That belongs in a future `loki deactivate` command, not installer lifecycle.

## macOS/Linux installer design

### `install.sh`

Default:

```bash
./install.sh --archive ./loki_<version>_<os>_<arch>.tar.gz --checksums ./checksums.txt
```

Behavior:

1. Detect OS and arch.
2. Verify checksum.
3. Extract to temp directory.
4. Install binary to `$HOME/.local/bin` by default.
5. `chmod 0755`.
6. Smoke:
   - `loki --version`
   - `loki doctor --json`
   - `loki tui --help`
7. Warn if install dir is not on PATH.
8. Run symlink probe; warn if unavailable.
9. Optional `--store-path <path>` for `loki store init/use`.

### `uninstall.sh`

Default:

- Remove installed binary from chosen install dir.
- Preserve local state/store.
- Optional `--remove-state` and `--remove-store PATH` with confirmation.

## Packaging changes

Update `scripts/package-release.sh`:

1. Include installer scripts in archives.
2. Copy standalone installer scripts to `dist/packages`.
3. Generate `release-manifest.json` with:
   - version
   - commit SHA
   - build date
   - Go version
   - asset list
   - sha256 per asset
4. Generate `checksums.txt` after all assets exist.
5. Keep binary inside archive named `loki`/`loki.exe`.

## CI changes

Add installer smoke job to `.github/workflows/ci.yml`:

| OS | Test |
|---|---|
| Windows | Build package, run `install.ps1 -ArchivePath ... -ChecksumsPath ... -InstallDir $env:RUNNER_TEMP\Loki -NoPath`, smoke binary, run `uninstall.ps1`, assert files removed. |
| Ubuntu | Build package, run `install.sh --archive ... --checksums ... --install-dir $RUNNER_TEMP/loki-bin`, smoke, uninstall. |
| macOS | Same as Ubuntu. |

Static checks:

- PowerShell parser check for `install.ps1` and `uninstall.ps1`.
- `bash -n scripts/install.sh scripts/uninstall.sh`.
- Optional later: PSScriptAnalyzer and shellcheck.

## Release workflow changes

Update `.github/workflows/release.yml`:

1. Run full validation.
2. Package release assets.
3. Run installer smoke against packaged assets.
4. Upload artifacts.
5. On tag push, create GitHub Release.
6. Release notes include:
   - install commands
   - checksum instructions
   - Windows Developer Mode / symlink note
   - uninstall commands
   - private GitHub token note

Stable releases should require a manual Windows VM installer smoke result. Pre-release tags can remain automatic prereleases.

## Windows VM install/uninstall test plan

Detailed procedure: `docs/test-plans/windows-installer-vm-smoke.md`.

High-level:

1. Build/package release.
2. Copy Windows zip, checksums, `install.ps1`, `uninstall.ps1` to VM.
3. Run per-user install in normal PowerShell.
4. Verify PATH, version, doctor, TUI help.
5. Verify symlink behavior with Developer Mode on and off/elevated branch where possible.
6. Run disposable store switch with a symlink target.
7. Uninstall.
8. Verify install files and PATH removed; local state and store preserved.
9. Reinstall/uninstall to prove idempotence.
10. Run destructive uninstall only against disposable state/store.

## Implementation sequence

1. [x] Add `scripts/install.ps1` and `scripts/uninstall.ps1`.
2. [x] Use `scripts/windows-installer-smoke.ps1` for Windows local-archive install/uninstall smoke in CI and the VM.
3. [x] Update package script to include standalone and bundled installers.
4. [x] Add `scripts/install.sh` and `scripts/uninstall.sh`.
5. [x] Add Linux/macOS installer smoke in CI.
6. [x] Update release workflow and release notes.
7. [ ] Add `loki doctor` symlink probe improvement.
8. [ ] Dogfood on Windows ARM64 VM.
9. [ ] Cut prerelease tag.
10. [ ] Consider MSI/MSIX/Homebrew/winget/Scoop after dogfood stabilizes.

## Open decisions

- Should install default add PATH automatically or require `-AddToPath`? Recommendation: interactive default yes, CI `-NoPath`.
- Should installer prompt for Developer Mode interactively or require `-EnableDeveloperMode`? Recommendation: prompt only in interactive mode; require explicit flag in non-interactive mode.
- Should installer run `loki store init` if OneDrive detected? Recommendation: no, only with explicit `-StorePath`.
- Should stable release block on VM smoke? Recommendation: yes; prerelease can be automatic.
