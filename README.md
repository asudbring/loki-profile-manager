# Loki Profile Manager

Loki Profile Manager is a local Go CLI for managing profile-specific dotfiles, app settings, AI-tool skills, and secret-rendered templates from a synced Loki store.

## Status

- Repository visibility: private.
- Current implementation: Phases 1-4 complete; Phase 4.5 migration/adoption bootstrap implemented; guarded snapshot inspection and restore implemented.
- Current commands: `status`, `verify`, `switch`, `sync`, `doctor`, `snapshots list`, `snapshots show`, `snapshots restore`, `machine register`, `machine status`, `migrate repo`, `migrate local`, and `adopt`.
- Not implemented yet: `import-skill` and `tui`.
- License: not selected yet.

## What it does today

Loki creates and validates a local store layout, tracks machine identity and machine policy, parses YAML profile manifests, verifies skill folders and mergeability, and activates a selected profile with symlink, copy, structured merge, and template-render operations.

Activation is guarded by unsafe overwrite protection. Loki refuses to replace unmanaged local files or directories. A target must be missing, already managed by Loki, or adopted by migration/adoption commands before activation can overwrite it.

## Install from release binary

Release assets are published from semver tags such as `v0.1.0-doctor.1`. This repository is private, so downloads require GitHub access.

1. Open the GitHub release.
2. Download the archive for your OS/architecture.
3. Download `checksums.txt` and verify the archive.
4. Extract `loki` or `loki.exe` onto your `PATH`.
5. Run `loki --version` and `loki doctor`.

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

Verify a store:

```bash
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

## Documentation

- [`docs/USAGE.md`](docs/USAGE.md) — current CLI commands, flags, and behavior.
- [`docs/INSTALL.md`](docs/INSTALL.md) — Windows, macOS, Linux, and Docker validation.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — current components, data ownership, and switch flow.
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) — package map and development workflow.
- [`CHANGELOG.md`](CHANGELOG.md) — pre-release dogfood and safety milestones.
- [`docs/ai-ops/install.ai.md`](docs/ai-ops/install.ai.md) — AI-operator install/test procedure.
- [`docs/ai-ops/windows-arm64-vm-test.ai.md`](docs/ai-ops/windows-arm64-vm-test.ai.md) — AI-operator Windows ARM64 VM + OneDrive validation procedure.
- [`tests/windows-arm64-vm-copilot.ai.md`](tests/windows-arm64-vm-copilot.ai.md) — Copilot CLI prompt for running Windows ARM64 VM validation.
- [`tests/cross-machine-dogfood-copilot.ai.md`](tests/cross-machine-dogfood-copilot.ai.md) — Copilot CLI prompt for cross-machine OneDrive dogfood verification.
- [`tests/real-dotfile-dogfood-copilot.ai.md`](tests/real-dotfile-dogfood-copilot.ai.md) — Copilot CLI prompt for the first real low-risk dotfile dogfood.
- [`tests/real-dotfile-targeted-restore-consent-copilot.ai.md`](tests/real-dotfile-targeted-restore-consent-copilot.ai.md) — consent-gated prompt for real-dotfile targeted snapshot restore.
- [`docs/handoffs/multi-os-phase-4.5-handoff.md`](docs/handoffs/multi-os-phase-4.5-handoff.md) — continuation handoff for macOS and Windows VM work.
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

No license has been selected yet. Treat this private repository as all rights reserved until a license file is added.
