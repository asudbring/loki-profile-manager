# Loki Profile Manager

Loki Profile Manager is a local Go CLI for managing profile-specific dotfiles, app settings, AI-tool skills, and secret-rendered templates from a synced Loki store.

## Status

- Repository visibility: public.
- Current implementation: Phases 1-7 complete through the TUI MVP; Phase 4.5 migration/adoption bootstrap, persistent store setup, guarded snapshot inspection/restore, sync conflict cleanup, skill folder/zip import MVP, Infisical secrets readiness UX, and Bubble Tea terminal UI are implemented.
- Current commands: `status`, `store status`, `store discover`, `store use`, `store init`, `store unset`, `verify`, `switch`, `sync`, `tui`, `import-skill`, `secrets`, `doctor`, `snapshots list`, `snapshots show`, `snapshots restore`, `machine register`, `machine status`, `migrate repo`, `migrate local`, and `adopt`.
- Not implemented yet: `import-skill` markdown conversion and Azure Key Vault/other secret providers.
- License: MIT.

## What it does today

Loki creates and validates a local store layout, tracks machine identity and machine policy, parses YAML profile manifests, verifies skill folders and mergeability, checks Infisical-backed secret readiness, and activates a selected profile with symlink, copy, structured merge, and template-render operations.

Activation is guarded by unsafe overwrite protection. Loki refuses to replace unmanaged local files or directories. A target must be missing, already managed by Loki, or adopted by migration/adoption commands before activation can overwrite it.

Before switching, Loki also checks copied managed targets from the currently active profile for local edits. If a tool changed a copied settings file, `loki switch` reports the local change and blocks real activation until you either resolve it or rerun with `--capture-local` to write safe copy-mode changes back to the store. Symlinked targets already write directly to the store; rendered outputs are never captured because they can contain secrets, and Loki regenerates them from templates during activation.

After a successful switch, Loki prunes obsolete managed targets that are not part of the newly active profile, such as old profile skill directories. Pruning is hash-guarded: unchanged Loki-managed targets are removed and their state records are deleted; changed obsolete targets block and require manual capture, removal, or adoption.

## Install from release binary

Release assets are published from semver tags such as `v0.1.0`. Download assets from the GitHub Releases page.

Recommended cross-platform install is the npm registry package. It bundles Loki binaries for Windows, macOS, and Linux on amd64/arm64, and npm uninstall only removes the wrapper/bundled binary.

```bash
npm install -g @asudbring/loki-profile-manager
loki --version
```

GitHub Release npm tarballs and script installers remain available for direct archive installs.

On Windows, open a new shell after npm global install and make sure the npm global bin directory is on `PATH` (typically `%APPDATA%\npm`, for example `%USERPROFILE%\AppData\Roaming\npm`). Node/npm also require `C:\Program Files\nodejs` on `PATH`.

1. Open the GitHub release.
2. Download the archive for your OS/architecture plus `checksums.txt`.
3. Use the script installer, or manually verify/extract the archive.

Windows PowerShell:

```powershell
.\install.ps1 -Version <version> `
  -ArchivePath .\loki_<version>_windows_arm64.zip `
  -ChecksumsPath .\checksums.txt `
  -AddToPath
```

macOS/Linux:

```bash
chmod +x install.sh uninstall.sh
./install.sh --version <version> \
  --archive ./loki_<version>_<os>_<arch>.tar.gz \
  --checksums ./checksums.txt
```

Uninstall scripts preserve local Loki state, synced stores, and managed targets by default. See [`docs/INSTALL.md`](docs/INSTALL.md) for install paths, store setup flags, and manual checksum commands.

## Install from source

Requires Go 1.23 or later.

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go test ./...
go build -o loki ./cmd/loki
./loki --help
```

On Windows PowerShell:

```powershell
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go test ./...
go build -o loki.exe ./cmd/loki
.\loki.exe --help
```

For full cross-platform setup and validation, see [`docs/INSTALL.md`](docs/INSTALL.md).

## Quick examples

Show local status, active profile, and managed target count:

```bash
go run ./cmd/loki status
go run ./cmd/loki --verbose status
```

Configure a persistent store:

```bash
go run ./cmd/loki store discover
go run ./cmd/loki store init /path/to/loki
# or use an existing valid store without creating files
go run ./cmd/loki store use /path/to/loki
```

Verify a store:

```bash
go run ./cmd/loki verify
# or override for one command
go run ./cmd/loki --store /path/to/loki verify
```

Run environment/store diagnostics:

```bash
go run ./cmd/loki doctor
go run ./cmd/loki --store /path/to/loki doctor --json
```

Scan and resolve provider conflict-copy files:

```bash
go run ./cmd/loki --store /path/to/loki sync --dry-run
go run ./cmd/loki --store /path/to/loki sync --yes
```

Import a skill folder or `.zip` archive into the store:

```bash
go run ./cmd/loki --store /path/to/loki import-skill ~/skills/my-skill --profile work --dry-run
go run ./cmd/loki --store /path/to/loki import-skill ~/Downloads/my-skill.zip --profile work --yes
```

Prepare Infisical-backed render secrets without printing values:

```bash
go run ./cmd/loki secrets --infisical
go run ./cmd/loki secrets login
go run ./cmd/loki secrets status
go run ./cmd/loki secrets check OPENAI_API_KEY
```

`loki secrets --infisical` creates or updates `~/.config/infisical/.env` from existing `INFISICAL_*` environment variables and local `.infisical.json` project config, then runs readiness checks. Output lists key names and readiness only, never values.

For machine identity auth, set `INFISICAL_TOKEN` or `INFISICAL_AUTH_METHOD=universal-auth` with `INFISICAL_CLIENT_ID`, `INFISICAL_CLIENT_SECRET`, and `INFISICAL_PROJECT_ID` in the local environment or in `~/.config/infisical/.env`. Set `INFISICAL_ENV` to choose a non-`dev` environment. Set `INFISICAL_API_URL`, `INFISICAL_HOST`, or legacy `INFISICAL_HOST_URL` when using a non-default Infisical host. Loki mints short-lived tokens only in child processes and never stores secret values in the synced store.

Register this machine for a profile and bucket policy:

```bash
go run ./cmd/loki --store /path/to/loki machine register --allow-profile work --allow-bucket azure
```

Dry-run profile activation:

```bash
go run ./cmd/loki --store /path/to/loki switch work --dry-run
```

Real activation:

```bash
go run ./cmd/loki --store /path/to/loki switch work --yes
```

`--yes` does not bypass unsafe overwrite protection.

Inspect and restore snapshots:

```bash
go run ./cmd/loki snapshots list
go run ./cmd/loki snapshots show <snapshot-id>
go run ./cmd/loki snapshots restore <snapshot-id> --dry-run
go run ./cmd/loki snapshots restore <snapshot-id> --target ~/.config/git/ignore --dry-run
go run ./cmd/loki snapshots restore <snapshot-id> --target ~/.config/git/ignore --yes
```

Full snapshot restore without `--target` requires a matching prior dry-run guard and an interactive confirmation phrase: `RESTORE <snapshot-id>`. Targeted restore still requires the dry-run guard but does not prompt for full active-state restore consent.

Launch the terminal UI:

```bash
go run ./cmd/loki
# explicit subcommand still works
go run ./cmd/loki tui
go run ./cmd/loki --store /path/to/loki tui
```

TUI MVP screens cover dashboard diagnostics, persistent store setup, machine registration, secrets/profile views, guarded profile switching, guarded sync conflict cleanup, and snapshot list/show/restore dry-run handoff. Restore writes still happen only through the existing `loki snapshots restore ... --yes` CLI flow shown by the TUI after dry-run.

## Documentation

- [`docs/USAGE.md`](docs/USAGE.md) — current CLI commands, flags, and behavior.
- [`docs/INSTALL.md`](docs/INSTALL.md) — Windows, macOS, Linux, and Docker validation.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — current components, data ownership, and switch flow.
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) — package map and development workflow.
- [`docs/RELEASE.md`](docs/RELEASE.md) — public Actions release path and local fallback release process.
- [`docs/ai-ops/release.ai.md`](docs/ai-ops/release.ai.md) — AI-operator release procedure.
- [`CHANGELOG.md`](CHANGELOG.md) — release history and safety milestones.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — development setup, PR expectations, and safety rules.
- [`SECURITY.md`](SECURITY.md) — vulnerability reporting and security boundaries.
- [`docs/ai-ops/install.ai.md`](docs/ai-ops/install.ai.md) — AI-operator install/test procedure.
- [`docs/ai-ops/windows-arm64-vm-test.ai.md`](docs/ai-ops/windows-arm64-vm-test.ai.md) — AI-operator Windows ARM64 VM + OneDrive validation procedure.
- [`tests/windows-arm64-vm-copilot.ai.md`](tests/windows-arm64-vm-copilot.ai.md) — Copilot CLI prompt for running Windows ARM64 VM validation.
- [`tests/cross-machine-dogfood-copilot.ai.md`](tests/cross-machine-dogfood-copilot.ai.md) — Copilot CLI prompt for cross-machine OneDrive dogfood verification.
- [`tests/real-dotfile-dogfood-copilot.ai.md`](tests/real-dotfile-dogfood-copilot.ai.md) — Copilot CLI prompt for the first real low-risk dotfile dogfood.
- [`tests/real-dotfile-targeted-restore-consent-copilot.ai.md`](tests/real-dotfile-targeted-restore-consent-copilot.ai.md) — consent-gated prompt for real-dotfile targeted snapshot restore.
- [`spec-loki-profile-manager.md`](spec-loki-profile-manager.md), [`plan-loki-profile-manager.md`](plan-loki-profile-manager.md), and [`tasks-loki-profile-manager.md`](tasks-loki-profile-manager.md) — planning documents, not a guarantee of implemented behavior.

## Development

Run tests:

```bash
go test ./...
go vet ./...
```

If the host lacks Go but has Docker:

```bash
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go test ./...
```

On Git Bash for Windows, set `MSYS_NO_PATHCONV=1` for Docker volume paths that start with a Windows drive letter.

## License

MIT. See [`LICENSE`](LICENSE).
