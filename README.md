# Loki Profile Manager

Loki Profile Manager is a local Go CLI for managing profile-specific dotfiles, app settings, AI-tool skills, and secret-rendered templates from a synced Loki store.

## Status

- Repository visibility: private.
- Current implementation: Phases 1-4 complete; Phase 4.5 migration/adoption bootstrap implemented.
- Current commands: `status`, `verify`, `switch`, `machine register`, `machine status`, `migrate repo`, `migrate local`, and `adopt`.
- Not implemented yet: `sync`, `import-skill`, `doctor`, and `tui`.
- License: not selected yet.

## What it does today

Loki creates and validates a local store layout, tracks machine identity and machine policy, parses YAML profile manifests, verifies skill folders and mergeability, and activates a selected profile with symlink, copy, structured merge, and template-render operations.

Activation is guarded by unsafe overwrite protection. Loki refuses to replace unmanaged local files or directories. A target must be missing, already managed by Loki, or adopted by migration/adoption commands before activation can overwrite it.

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

## Documentation

- [`docs/USAGE.md`](docs/USAGE.md) — current CLI commands, flags, and behavior.
- [`docs/INSTALL.md`](docs/INSTALL.md) — Windows, macOS, Linux, and Docker validation.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — current components, data ownership, and switch flow.
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) — package map and development workflow.
- [`docs/ai-ops/install.ai.md`](docs/ai-ops/install.ai.md) — AI-operator install/test procedure.
- [`docs/ai-ops/windows-arm64-vm-test.ai.md`](docs/ai-ops/windows-arm64-vm-test.ai.md) — AI-operator Windows ARM64 VM + OneDrive validation procedure.
- [`tests/windows-arm64-vm-copilot.ai.md`](tests/windows-arm64-vm-copilot.ai.md) — Copilot CLI prompt for running Windows ARM64 VM validation.
- [`tests/cross-machine-dogfood-copilot.ai.md`](tests/cross-machine-dogfood-copilot.ai.md) — Copilot CLI prompt for cross-machine OneDrive dogfood verification.
- [`tests/real-dotfile-dogfood-copilot.ai.md`](tests/real-dotfile-dogfood-copilot.ai.md) — Copilot CLI prompt for the first real low-risk dotfile dogfood.
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
