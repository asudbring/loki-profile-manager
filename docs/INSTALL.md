# Install

Loki Profile Manager is a local CLI. Install puts the `loki` command on `PATH`. First-run setup connects this machine to a synced Loki store and activates profile-managed files.

Loki does not sync cloud files itself. Put the store in a folder already synced by OneDrive, Dropbox, iCloud Drive, Syncthing, or another filesystem sync tool.

## Supported runtime targets

| OS | Architectures | Notes |
|---|---|---|
| Windows | amd64, arm64 | PowerShell, Command Prompt, Git Bash, and Windows Terminal supported. Symlink targets require Developer Mode or elevation. |
| macOS | amd64, arm64 | zsh/bash supported. Default install path is `$HOME/.local/bin`. |
| Linux | amd64, arm64 | bash/zsh supported. Default install path is `$HOME/.local/bin`. |

## Requirements

Required for normal install:

- Node.js 18 or later with npm, or a downloaded GitHub Release archive.
- A synced folder for the Loki store.

Required for source builds only:

- Git.
- Go 1.23 or later.
- Optional Docker for validation when Go is unavailable on the host.

Not implemented yet:

- Homebrew, winget, Scoop, MSI/MSIX, macOS notarization, deb, and rpm packages.

## Install Loki

### Preferred: npm registry

The npm package bundles native Loki binaries for all supported OS/architecture pairs and installs a `loki` wrapper on `PATH`.

Windows PowerShell:

```powershell
npm install -g @asudbring/loki-profile-manager
loki --version
loki doctor
```

macOS/Linux shell:

```bash
npm install -g @asudbring/loki-profile-manager
loki --version
loki doctor
```

Uninstall removes the npm wrapper and bundled binary only. It does not remove the synced store, local Loki state, snapshots, or managed target files.

```bash
npm uninstall -g @asudbring/loki-profile-manager
```

### GitHub Release npm tarball

Download `asudbring-loki-profile-manager-<npm-version>.tgz` from the GitHub Release, then install it locally.

```bash
npm install -g ./asudbring-loki-profile-manager-<npm-version>.tgz
loki --version
```

`<npm-version>` is the release version without the leading `v`.

### GitHub Release binary archive

Download the archive for your OS/architecture plus `checksums.txt` from the same GitHub Release.

Asset pattern:

```text
loki_<version>_<os>_<arch>.tar.gz
loki_<version>_windows_<arch>.zip
install.sh
uninstall.sh
install.ps1
uninstall.ps1
checksums.txt
release-manifest.json
```

Linux/macOS script install:

```bash
chmod +x install.sh uninstall.sh
./install.sh --version <version> \
  --archive ./loki_<version>_<os>_<arch>.tar.gz \
  --checksums ./checksums.txt \
  --install-dir "$HOME/.local/bin"

loki --version
```

Windows PowerShell script install:

```powershell
.\install.ps1 `
  -Version <version> `
  -ArchivePath .\loki_<version>_windows_arm64.zip `
  -ChecksumsPath .\checksums.txt `
  -InstallDir "$env:LOCALAPPDATA\Programs\Loki" `
  -AddToPath

loki --version
```

Default install and local-state paths:

| OS | Default binary path | Local state path preserved by uninstall |
|---|---|---|
| Windows | `%LOCALAPPDATA%\Programs\Loki` | `%LOCALAPPDATA%\loki-profile-manager` |
| macOS | `$HOME/.local/bin` | `~/Library/Application Support/loki-profile-manager` |
| Linux | `$HOME/.local/bin` | `~/.local/state/loki-profile-manager` |

Uninstall scripts preserve the synced store and managed targets:

```bash
./uninstall.sh --install-dir "$HOME/.local/bin"
```

```powershell
.\uninstall.ps1 -InstallDir "$env:LOCALAPPDATA\Programs\Loki"
```

Manual checksum verification, Linux/macOS:

```bash
grep " loki_<version>_<os>_<arch>.tar.gz$" checksums.txt | shasum -a 256 -c -
```

Manual checksum verification, Windows PowerShell:

```powershell
$Archive = "loki_<version>_windows_arm64.zip"
$Expected = (Get-Content .\checksums.txt | Where-Object { $_ -like "* $Archive" }).Split()[0]
$Actual = (Get-FileHash .\$Archive -Algorithm SHA256).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) { throw "checksum mismatch" }
```

## First-run path: fresh machine

Use this path when the machine has no important local profile files, or when you are joining an already-migrated Loki store and want Loki to become the source of truth for this machine.

If the store has no profiles yet, create the first profile core before running `verify <profile>` or `switch <profile>`. See [`PROFILES.md`](PROFILES.md) for profile naming, bucket design, settings organization, skill import, and machine registration from scratch.

### Fresh machine safety rules

- Sign in to the sync provider first and wait until the Loki store is available locally.
- Use `loki store use` for an existing valid store.
- Use `loki store init` only for a new empty store or an empty folder you want Loki to initialize.
- If `store init` creates a new empty store, create/import/migrate profile manifests before `verify <profile>` or `switch <profile>`; an empty store has no activatable profiles. Use [`PROFILES.md`](PROFILES.md) when starting from nothing.
- Register the machine before `verify` or `switch`.
- Run `loki switch ... --dry-run` before `--yes`.
- `--yes` does not bypass unmanaged overwrite protection by itself.
- If dry-run reports unmanaged blockers and the Loki store should win, run `loki switch ... --backup-unmanaged --yes` to move those blockers to a local backup directory before activation.
- If an unmanaged local file should become source of truth, use `loki adopt` or `loki migrate local` instead of `--backup-unmanaged`.
- Do not put secret values in the store. Render templates should reference secret names only.

### Windows fresh machine

Open PowerShell.

```powershell
# Install if not already installed.
npm install -g @asudbring/loki-profile-manager
loki --version

# Pick the synced store path. Adjust if OneDrive uses a business tenant name.
$Store = Join-Path $env:OneDrive "LokiProfileManager"

# Discover candidates and persist the store.
loki store discover --manual $Store
if (Test-Path (Join-Path $Store "registry")) {
  loki store use $Store
} else {
  loki store init $Store
}

# Register this machine for the profiles/buckets it is allowed to activate.
loki machine register --name $env:COMPUTERNAME --allow-profile work --allow-bucket content-dev

# If this was a brand-new empty store, add/migrate/import profile manifests before this point.
# Check store, machine policy, manifests, skills, mergeability, and secret readiness.
loki doctor
loki verify work content-dev

# Preview activation. Review every target and blocker before writing.
loki switch work content-dev --dry-run

# If the only blockers are unmanaged local files and the synced Loki store should win,
# move those blockers to a local backup and activate in one guarded operation.
loki switch work content-dev --backup-unmanaged --yes

# Otherwise activate only when the dry-run shows no blockers.
loki switch work content-dev --yes

# Confirm state.
loki status --verbose
```

If the machine already has safe copy-mode changes from the currently active Loki profile and you want to preserve them before switching again, use:

```powershell
loki switch work content-dev --capture-local --yes
```

`--capture-local` writes safe managed-target local changes back to the store before switching. It does not capture unmanaged files, render outputs, or unresolved merge conflicts.

### macOS fresh machine

Use zsh or bash.

```bash
npm install -g @asudbring/loki-profile-manager
loki --version

# Common OneDrive path on recent macOS versions. Adjust for your provider.
STORE="$HOME/Library/CloudStorage/OneDrive-Personal/LokiProfileManager"

loki store discover --manual "$STORE"
if [ -d "$STORE/registry" ]; then
  loki store use "$STORE"
else
  loki store init "$STORE"
fi

loki machine register --name "$(hostname)" --allow-profile work --allow-bucket content-dev
# If this was a brand-new empty store, add/migrate/import profile manifests before this point.
loki doctor
loki verify work content-dev
loki switch work content-dev --dry-run
loki switch work content-dev --yes
loki status --verbose
```

### Linux fresh machine

Use bash or zsh.

```bash
npm install -g @asudbring/loki-profile-manager
loki --version

# Adjust to your sync provider path.
STORE="$HOME/OneDrive/LokiProfileManager"

loki store discover --manual "$STORE"
if [ -d "$STORE/registry" ]; then
  loki store use "$STORE"
else
  loki store init "$STORE"
fi

loki machine register --name "$(hostname)" --allow-profile work --allow-bucket content-dev
# If this was a brand-new empty store, add/migrate/import profile manifests before this point.
loki doctor
loki verify work content-dev
loki switch work content-dev --dry-run
loki switch work content-dev --yes
loki status --verbose
```

### Fresh machine app smoke checklist

After activation, open fresh shells/apps, not already-running processes.

PowerShell on Windows:

```powershell
$env:STARSHIP_CONFIG
echo $env:LOKI_PROFILE
starship prompt
loki status --verbose
```

Git Bash / bash / zsh:

```bash
echo "$LOKI_PROFILE"
echo "$STARSHIP_CONFIG"
starship prompt
loki status --verbose
```

App checks:

- Windows Terminal or terminal app loads the Loki-managed shell profile.
- Prompt shows the Loki active profile/buckets from Loki local state.
- `starship prompt` exits successfully and does not reference legacy profile scripts.
- VS Code user settings are present and do not point at old profile repos.
- Codex, Pi, Claude/Copilot, Git, and Warp config files exist only if the selected profile manages them.
- `loki switch <profile> [buckets...] --dry-run` has no unexpected blockers after the real switch.

Recent dogfood validation covered the Windows VM app/manual switch flow with Loki `v0.1.5`, profile marker `work:content-dev`, 231 managed targets, PowerShell/Git Bash/starship startup, VS Code settings, Codex/Pi/Claude/Copilot/Git config files, and a full VM store legacy-reference audit with zero legacy hits. Release `v0.1.6` adds the interactive Infisical setup wizard and `--backup-unmanaged` first-install remediation on top of that validated path.

## First-run path: existing machine with profiles not migrated

Use this path when the machine already has dotfiles, shell profiles, app settings, AI-tool config, or legacy profile repositories that Loki does not manage yet. If you need to decide profile names, bucket names, or where skills/settings belong before migration, read [`PROFILES.md`](PROFILES.md) first.

### Existing-machine safety rules

- Do not run `loki switch ... --yes` first.
- Inventory and back up current local files before migration.
- Start with `migrate ... --dry-run` and `adopt ... --dry-run`.
- Review generated manifests before a real switch.
- Use `verify` and `switch --dry-run` before any write.
- If `switch` blocks on unmanaged files, do not force it. Adopt, migrate, move aside, or back up the file intentionally.
- `--capture-local` is for safe copied managed targets from the currently active Loki profile. It is not a migration shortcut for unmanaged files.

### Step 1 — Install Loki and configure the store

Windows PowerShell:

```powershell
npm install -g @asudbring/loki-profile-manager
$Store = Join-Path $env:OneDrive "LokiProfileManager"
loki store discover --manual $Store
loki store init $Store   # only if this is a new empty store
# or: loki store use $Store
loki machine register --name $env:COMPUTERNAME --allow-profile work --allow-bucket content-dev
```

macOS/Linux shell:

```bash
npm install -g @asudbring/loki-profile-manager
STORE="$HOME/OneDrive/LokiProfileManager"
loki store discover --manual "$STORE"
loki store init "$STORE"   # only if this is a new empty store
# or: loki store use "$STORE"
loki machine register --name "$(hostname)" --allow-profile work --allow-bucket content-dev
```

### Step 2 — Back up current local profile files

Pick a backup directory outside the Loki store. Do not put secrets or private keys in chat/log output.

Windows PowerShell example:

```powershell
$Backup = Join-Path $env:USERPROFILE ("loki-preflight-backup\" + (Get-Date -Format yyyyMMdd-HHmmss))
New-Item -ItemType Directory -Force $Backup | Out-Null
$Targets = @(
  "$env:USERPROFILE\.gitconfig",
  "$env:USERPROFILE\.bashrc",
  "$env:USERPROFILE\.zshrc",
  "$HOME\Documents\PowerShell\Microsoft.PowerShell_profile.ps1",
  "$env:APPDATA\Code\User\settings.json",
  "$env:USERPROFILE\.codex",
  "$env:USERPROFILE\.pi",
  "$env:USERPROFILE\.claude",
  "$env:USERPROFILE\.copilot"
)
foreach ($Target in $Targets) {
  if (Test-Path $Target) {
    Copy-Item $Target -Destination $Backup -Recurse -Force
  }
}
Write-Host "Backup: $Backup"
```

macOS/Linux example:

```bash
BACKUP="$HOME/loki-preflight-backup/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP"
for target in \
  "$HOME/.gitconfig" \
  "$HOME/.bashrc" \
  "$HOME/.zshrc" \
  "$HOME/.config/Code/User/settings.json" \
  "$HOME/.codex" \
  "$HOME/.pi" \
  "$HOME/.claude" \
  "$HOME/.copilot"; do
  [ -e "$target" ] && cp -a "$target" "$BACKUP/"
done
printf 'Backup: %s\n' "$BACKUP"
```

Adjust the target list for your actual local apps. Backups are for rollback; Loki also creates activation snapshots before real switches, but snapshots only cover targets Loki is about to write.

### Step 3 — Migrate known local files

`migrate local` scans known dotfiles, app settings, shell profiles, and local skill folders. It writes store files and managed-target records only with `--yes`; it does not rewrite the local target files.

```bash
loki migrate local --profile work --dry-run
loki migrate local --profile work --yes
```

Use a bucket when the settings belong to a profile bucket:

```bash
loki migrate local --profile work --bucket content-dev --dry-run
loki migrate local --profile work --bucket content-dev --yes
```

### Step 4 — Migrate a legacy profile repository, if one exists

`migrate repo` imports supported files/templates/skills from an existing repository into the selected Loki layer. It skips generated and sensitive paths such as `.git` and private SSH keys. It does not rewrite local targets.

```bash
loki migrate repo ~/github/legacy-profile-repo --profile work --dry-run
loki migrate repo ~/github/legacy-profile-repo --profile work --yes
```

Bucket example:

```bash
loki migrate repo ~/github/legacy-profile-repo --profile work --bucket content-dev --dry-run
loki migrate repo ~/github/legacy-profile-repo --profile work --bucket content-dev --yes
```

### Step 5 — Adopt missed targets one at a time

Use `adopt` for important files not covered by migration. Start with dry-run.

```bash
loki adopt ~/.gitconfig --profile work --dry-run
loki adopt ~/.gitconfig --profile work --yes
loki adopt ~/.config/app/settings.json --profile work --bucket content-dev --mode merge --dry-run
loki adopt ~/.config/app/settings.json --profile work --bucket content-dev --mode merge --yes
```

Windows PowerShell profile example with redirected Documents:

```powershell
$ProfilePath = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'PowerShell\Microsoft.PowerShell_profile.ps1'
loki adopt $ProfilePath --profile work --source-name powershell/profile.ps1 --dry-run
loki adopt $ProfilePath --profile work --source-name powershell/profile.ps1 --yes
```

### Step 6 — Review manifests and verify

Review the generated files under the store before activation:

```bash
loki store status
loki verify work content-dev
loki status --verbose
```

Check manifest targets for the expected profile. Do not paste secret values into manifests. Use template placeholders and Infisical readiness checks instead:

```bash
loki secrets configure infisical  # interactive local-only setup, if Infisical is not configured yet
loki secrets status
loki secrets check SECRET_NAME
```

For automation that already has safe `INFISICAL_*` environment variables or `.infisical.json` project config, use noninteractive `loki secrets --infisical` instead of the wizard.

### Step 7 — Dry-run and resolve blockers

```bash
loki switch work content-dev --dry-run
```

Common blockers and actions:

| Blocker | Action |
|---|---|
| Unmanaged file/directory | If the local file should become store truth, adopt or migrate it. If the synced Loki store should win, rerun switch with `--backup-unmanaged --yes` to move the local blocker to a local backup before activation. Or manually move it aside/remove the conflicting manifest target. |
| Managed hash mismatch | Review local change. If safe copy-mode drift, use `--capture-local`; otherwise manually reconcile. |
| Render target drift | Do not capture generated output. Update the template/source secret, then switch again. |
| Merge conflict | Resolve the store source or local target manually. Merge capture is manual in this MVP. |
| Broken symlink | Repair/remove/adopt intentionally before switch. |
| Target outside home | Fix manifest target path. |

### Step 8 — Activate

After a clean dry-run:

```bash
loki switch work content-dev --yes
```

If the only blocker is safe copy-mode local drift from the currently active Loki-managed profile and you want to write it back first:

```bash
loki switch work content-dev --capture-local --yes
```

After activation:

```bash
loki status --verbose
loki switch work content-dev --dry-run
```

The second dry-run should show no unexpected blockers. It can still show the planned operation count.

### Step 9 — Test apps and shells

Open fresh app windows. Do not rely on already-running processes.

- PowerShell: profile startup prints/sets the Loki profile state if your store manages that profile.
- Bash/zsh: shell wrapper reads Loki active profile state, not legacy profile scripts.
- Starship: `starship prompt` succeeds and reads Loki marker/state.
- VS Code: settings load from the managed target.
- Codex/Pi/Claude/Copilot: config files exist for the selected profile and do not point at a legacy profile repo.
- Git/Git Bash: Git config and shell startup match the active profile.
- Warp: if installed, managed config/theme files load and do not point at old profile paths.

## Windows-specific notes

### Npm PATH

If npm install succeeds but `loki` is not found:

```powershell
npm config get prefix
$env:PATH
```

The per-user npm bin is usually `%APPDATA%\npm`, for example `%USERPROFILE%\AppData\Roaming\npm`. Node/npm also require `C:\Program Files\nodejs` on `PATH`. Open a new shell after changing PATH.

### Symlinks

Windows symlink activation requires Developer Mode or elevated privileges. Loki returns a remediation error when symlink creation is denied and does not silently fall back to copy.

### Redirected Documents and PowerShell profiles

Parallels, OneDrive, and enterprise policies can redirect Documents away from `C:\Users\<user>\Documents`. Use Loki manifest variables such as `${DOCUMENTS}`, `${DOCUMENTS_DIR}`, or `${USER_DOCUMENTS}` for PowerShell profile targets. Loki resolves them through the Windows Known Folder API when available.

Expected PowerShell target example:

```yaml
target: ${DOCUMENTS}/PowerShell/Microsoft.PowerShell_profile.ps1
```

### OneDrive locks

OneDrive can temporarily lock or hydrate files such as `registry\machines.json`. If `loki machine register` fails with `Access is denied`:

1. Wait for OneDrive sync/hydration to settle.
2. Confirm the store folder is available locally.
3. Retry the same command.
4. If it persists, inspect file attributes and ACLs:

   ```powershell
   $Store = "$env:OneDrive\LokiProfileManager"
   attrib "$Store\registry\machines.json"
   Get-Acl "$Store\registry\machines.json"
   ```

## macOS/Linux-specific notes

- Ensure the install directory is on `PATH`. For `$HOME/.local/bin`, add it to `~/.zshrc`, `~/.bashrc`, or your shell profile if needed.
- Symlink activation uses normal Unix symlinks and fails if permissions or filesystem rules block creation.
- Local state is machine-local and should not be moved into the synced store.
- If using iCloud Drive or another provider with placeholder files, make the Loki store available offline before switching.

## Source build

Use source builds for development or validation, not as the normal user install path.

Windows PowerShell:

```powershell
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go mod verify
go build -o loki.exe ./cmd/loki
.\loki.exe --help
```

macOS/Linux shell:

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go mod verify
go build -o loki ./cmd/loki
./loki --help
```

Windows ARM64 note: `go test -race ./...` is not supported by Go on Windows ARM64. Use normal `go test ./...`, cross-compile validation, and a native VM smoke test.

## Docker validation

Use Docker when the host does not have Go installed.

Linux/macOS shell:

```bash
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go test ./...
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go vet ./...
```

Git Bash for Windows:

```bash
MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/<user>/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go test ./...

MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/<user>/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go vet ./...
```

## Disposable smoke test

Use a disposable store when validating a build without touching real profiles.

```bash
STORE=$(mktemp -d)/loki
loki store init "$STORE"
loki --store "$STORE" machine register --allow-profile smoke
loki --store "$STORE" status
loki --store "$STORE" doctor
loki --store "$STORE" tui --help
```

An empty initialized store has no real profile manifests. `verify smoke` and `switch smoke` require a profile manifest first.

## Troubleshooting quick table

| Symptom | Likely cause | Fix |
|---|---|---|
| `loki` not found after npm install | npm global bin missing from PATH | Add npm prefix bin to PATH and open a new shell. |
| `store use` refuses path | Existing path is not a valid Loki store layout | Use `store init` on an empty folder or select the correct existing store. |
| `store init` refuses path | Directory is non-empty but not a valid Loki store | Move contents aside or choose an empty folder. |
| `machine.record_missing` warning | Machine ID exists locally but registry has no record | Run `loki machine register --allow-profile <profile> ...`. |
| `switch` blocks unmanaged file | Loki will not overwrite local files it does not manage | Use `migrate local`, `migrate repo`, `adopt`, move the file aside, or rerun with `--backup-unmanaged --yes` only when the synced Loki store should win. |
| `switch` asks for capture | A copied managed target changed locally | Review change, then rerun with `--capture-local --yes` only if safe. |
| Secret render fails | Infisical not ready or secret missing | Run `loki secrets configure infisical` for interactive setup, then `loki secrets status` and `loki secrets check <NAME>`. For automation from existing safe local inputs, run `loki secrets --infisical`. |
| Operation lock timeout | Another Loki process or stale `.loki-operation.lock` | Confirm no Loki process is active on any synced machine before manual lock removal. |

## Rollback and recovery

- Install rollback: uninstall the package or remove the binary. This does not alter managed targets.
- Store rollback: restore the synced store from your sync provider/version history or backups.
- Activation rollback: inspect local snapshots with `loki snapshots list` and `loki snapshots show <id>`. Preview restore first:

```bash
loki snapshots restore <snapshot-id> --dry-run
loki snapshots restore <snapshot-id> --target <path> --dry-run
```

Full restore requires a matching dry-run and typed `RESTORE <snapshot-id>` confirmation. Targeted restore still requires a prior dry-run guard.
