# Loki Profile Manager

Loki Profile Manager is a local Go CLI for managing profile-specific dotfiles, app settings, AI-tool skills, shell prompt state, and secret-rendered templates from a synced Loki store.

## Status

- Repository visibility: public.
- Current npm release: `v0.1.20`; latest full Windows app/manual dogfood validation: `v0.1.7`.
- Current implementation: profile store setup, machine registration, migration/adoption bootstrap, guarded profile switching, pre-switch local-change capture, obsolete managed-target cleanup, safe stale managed-state repair, local active-profile marker, Windows redirected Documents support, snapshot inspection/restore, sync conflict cleanup, skill folder/zip/markdown import, plugin bundle import MVP, Infisical readiness/setup UX, and Bubble Tea TUI MVP.
- Current commands: `status`, `store status`, `store discover`, `store migrate`, `store use`, `store init`, `store unset`, `verify`, `switch`, `sync`, `tui`, `update`, `import-skill`, `import-plugin`, `secrets`, `doctor`, `snapshots list`, `snapshots show`, `snapshots restore`, `machine register`, `machine status`, `migrate repo`, `migrate local`, and `adopt`.
- Future work: Azure Key Vault/other secret providers.
- License: MIT.

## What it does

Loki treats a synced filesystem folder as profile source of truth. The store contains YAML manifests, profile files, templates, skills, and a machine registry. Each machine keeps local SQLite state for the configured store path, active profile/buckets, managed target hashes, snapshots, and recovery guards.

Activation is guarded by unsafe overwrite protection. Loki refuses to replace unmanaged local files or directories unless you explicitly move those blockers to a machine-local backup with `switch --backup-unmanaged --yes`. A target must be missing, already managed by Loki, adopted/migrated, or backed up before activation can overwrite it.

Before switching, Loki checks copied managed targets from the currently active profile for local edits. If a tool changed a copied settings file, `loki switch` reports the local change and blocks real activation until you resolve it or rerun with `--capture-local` to write safe copy-mode changes back to the store. Symlink targets already write directly to the store. Render outputs are never captured because they can contain secrets.

After a successful switch, Loki prunes obsolete managed targets that are not part of the newly active profile. Cleanup is hash-guarded: unchanged Loki-managed targets are removed and their state records are deleted; changed obsolete targets block and require manual capture, removal, or adoption.

## Install

Recommended install path:

```bash
npm install -g @asudbring/loki-profile-manager
loki --version
loki doctor
```

Windows PowerShell uses the same npm command:

```powershell
npm install -g @asudbring/loki-profile-manager
loki --version
loki doctor
```

Update an npm install in place:

```bash
loki update
```

`loki update` installs `@asudbring/loki-profile-manager@latest` with npm. Human CLI commands check npm at most once every 24 hours and print a stderr notice when a newer Loki version is available. Set `LOKI_NO_UPDATE_CHECK=1` to disable the startup check.

If Windows cannot find `loki` after npm install, open a new shell and ensure the npm global bin directory is on `PATH` (usually `%APPDATA%\npm`). Node/npm also require `C:\Program Files\nodejs` on `PATH`.

Release binary archives, script installers, source builds, and Docker validation are documented in [`docs/INSTALL.md`](docs/INSTALL.md).

## First-run paths

Use one of these workflows after installing the binary.

| Scenario | Start here |
|---|---|
| No profiles yet; starting from a blank store | [`docs/PROFILES.md`](docs/PROFILES.md) |
| Fresh machine or already-migrated store | [`docs/INSTALL.md#first-run-path-fresh-machine`](docs/INSTALL.md#first-run-path-fresh-machine) |
| Existing machine with local profiles/settings not migrated | [`docs/INSTALL.md#first-run-path-existing-machine-with-profiles-not-migrated`](docs/INSTALL.md#first-run-path-existing-machine-with-profiles-not-migrated) |
| AI/operator install and validation | [`docs/ai-ops/install.ai.md`](docs/ai-ops/install.ai.md) |

Short fresh-machine example:

```bash
STORE="$HOME/OneDrive/LokiProfileManager"
loki store discover --manual "$STORE"
loki store use "$STORE"        # existing valid store with profiles
# or: loki store init "$STORE" # new empty store; add/migrate/import profiles before verify/switch
loki machine register --allow-profile work --allow-bucket content-dev
loki verify work content-dev
loki switch work content-dev --dry-run
loki switch work content-dev --yes
loki status --verbose
```

Short existing-machine example:

```bash
STORE="$HOME/OneDrive/LokiProfileManager"
loki store use "$STORE"
loki machine register --allow-profile work --allow-bucket content-dev
loki migrate local --profile work --dry-run
loki migrate local --profile work --yes
loki verify work content-dev
loki switch work content-dev --dry-run
loki switch work content-dev --yes
```

When switching away from a profile after a managed copy target changed locally and you want to preserve that change in the store:

```bash
loki switch work content-dev --capture-local --yes
```

`--capture-local` writes safe managed copy-mode local changes back to the store before switching. If a fresh install blocks on unmanaged files and the Loki store should win, use `loki switch work content-dev --backup-unmanaged --yes` to move those blockers to a local backup directory and switch.

## Quick command examples

Update Loki from the latest npm build:

```bash
loki update
```

Show local status, active profile, and managed target count:

```bash
loki status
loki --verbose status
```

Discover, configure, or move a store:

```bash
loki store discover
loki store discover --manual /path/to/LokiProfileManager
loki store init /path/to/LokiProfileManager
loki store use /path/to/LokiProfileManager
loki store status
loki store migrate --to "$HOME/Dropbox/LokiProfileManager" --provider dropbox --dry-run
loki store migrate --to "$HOME/Dropbox/LokiProfileManager" --provider dropbox --yes
# If dry-run reports cloud-only files, materialize them explicitly:
loki store migrate --to "$HOME/Dropbox/LokiProfileManager" --provider dropbox --yes --hydrate
```

`store migrate` copies the current valid store to a hidden staging sibling, validates it, promotes it to the missing/empty destination, and rewires local SQLite state unless `--copy-only` is set. It reports progress by default during `--yes`, fails fast on cloud-only source files unless `--hydrate` is provided, retargets active Loki-managed symlinks after rewire, and never deletes the old store. Use `--cleanup` to remove interrupted staging directories and `--capture-local` if safe copy-mode local changes must be written back before copying.

Register this machine:

```bash
loki machine register --allow-profile work --allow-bucket content-dev --allow-bucket azure
loki machine status
```

Verify a profile and buckets:

```bash
loki verify work
loki verify work content-dev azure
loki verify work content-dev azure --json
```

Dry-run and activate a profile:

```bash
loki switch work content-dev --dry-run
loki switch work content-dev --yes
loki switch work content-dev --capture-local --yes
loki switch work content-dev --backup-unmanaged --yes
```

Migrate or adopt existing local files before the first real switch:

```bash
loki migrate local --profile work --dry-run
loki migrate local --profile work --yes
loki migrate repo ~/github/legacy-profile-repo --profile work --dry-run
loki adopt ~/.gitconfig --profile work --dry-run
loki adopt ~/.gitconfig --profile work --yes
```

Prepare Infisical-backed render secrets without printing values:

```bash
loki secrets configure infisical
loki secrets --infisical
loki secrets login
loki secrets status
loki secrets check OPENAI_API_KEY
```

`loki secrets configure infisical` prompts for Infisical Universal Auth details, validates them before writing, and writes only local `~/.config/infisical/.env` config. If validation fails, the existing env file stays unchanged. Run `loki secrets status` afterward to verify readiness. `loki secrets --infisical` remains noninteractive for automation from existing safe local inputs. Output lists key names/readiness only, never values.

Run local diagnostics and safely repair stale managed-target SQLite state:

```bash
loki doctor
loki doctor --json
loki doctor --repair-managed-state
loki doctor --repair-managed-state --write-safe-files
```

`loki doctor --repair-managed-state` refreshes stale local managed-target records only when the local target and current manifest source are equivalent. `--write-safe-files` additionally canonicalizes safe local files before updating state. Semantic repair is implemented for JSON; other files must be byte-identical unless Loki can regenerate the canonical merge output safely.

Inspect and restore local activation snapshots:

```bash
loki snapshots list
loki snapshots show <snapshot-id>
loki snapshots restore <snapshot-id> --dry-run
loki snapshots restore <snapshot-id> --target ~/.config/git/ignore --dry-run
loki snapshots restore <snapshot-id> --target ~/.config/git/ignore --yes
```

Full snapshot restore without `--target` requires a matching prior dry-run guard and an interactive confirmation phrase: `RESTORE <snapshot-id>`.

Launch the terminal UI:

```bash
loki
loki tui
loki --store /path/to/LokiProfileManager tui
```

The TUI MVP covers dashboard diagnostics, persistent store setup, machine registration, Infisical setup/status, profile views, guarded profile switching, guarded sync conflict cleanup, and snapshot list/show/restore dry-run handoff. Restore writes still happen only through the guarded CLI restore flow.

## Documentation

- [`docs/INSTALL.md`](docs/INSTALL.md) — install, fresh-machine setup, existing-machine migration, app smoke checks, and troubleshooting.
- [`docs/PROFILES.md`](docs/PROFILES.md) — create first profiles/buckets from scratch, organize settings/skills, and register machines.
- [`docs/USAGE.md`](docs/USAGE.md) — CLI commands, flags, behavior, and safety rules.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — components, data ownership, switch flow, TUI flow, and safety model.
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) — package map, build/test commands, release packaging, and validation rules.
- [`docs/RELEASE.md`](docs/RELEASE.md) — public Actions release path and local fallback release process.
- [`docs/ai-ops/install.ai.md`](docs/ai-ops/install.ai.md) — AI-operator install/bootstrap procedure.
- [`docs/ai-ops/release.ai.md`](docs/ai-ops/release.ai.md) — AI-operator release procedure.
- [`docs/ai-ops/windows-arm64-vm-test.ai.md`](docs/ai-ops/windows-arm64-vm-test.ai.md) — AI-operator Windows ARM64 VM + OneDrive validation procedure.
- [`docs/test-plans/windows-vm-infisical-tui-smoke.md`](docs/test-plans/windows-vm-infisical-tui-smoke.md) — manual Windows VM Infisical and TUI smoke plan.
- [`CHANGELOG.md`](CHANGELOG.md) — release history and safety milestones.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — development setup, PR expectations, and safety rules.
- [`SECURITY.md`](SECURITY.md) — vulnerability reporting and security boundaries.
- [`tests/windows-arm64-vm-copilot.ai.md`](tests/windows-arm64-vm-copilot.ai.md) — Copilot CLI prompt for Windows ARM64 VM validation.
- [`tests/cross-machine-dogfood-copilot.ai.md`](tests/cross-machine-dogfood-copilot.ai.md) — Copilot CLI prompt for cross-machine OneDrive dogfood verification.
- [`tests/real-dotfile-dogfood-copilot.ai.md`](tests/real-dotfile-dogfood-copilot.ai.md) — Copilot CLI prompt for first real low-risk dotfile dogfood.
- [`tests/real-dotfile-targeted-restore-consent-copilot.ai.md`](tests/real-dotfile-targeted-restore-consent-copilot.ai.md) — consent-gated prompt for real-dotfile targeted snapshot restore.
- [`spec-loki-profile-manager.md`](spec-loki-profile-manager.md), [`plan-loki-profile-manager.md`](plan-loki-profile-manager.md), and [`tasks-loki-profile-manager.md`](tasks-loki-profile-manager.md) — planning documents, not a guarantee of implemented behavior.

## Development

Requires Go 1.23 or later.

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go test ./...
go vet ./...
go build -o loki ./cmd/loki
./loki --help
```

If the host lacks Go but has Docker:

```bash
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go test ./...
```

On Git Bash for Windows, set `MSYS_NO_PATHCONV=1` for Docker volume paths that start with a Windows drive letter.

## License

MIT. See [`LICENSE`](LICENSE).
