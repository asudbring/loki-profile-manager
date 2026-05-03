# Scribe Plan: loki-profile-manager

**Tier**: 2
**Project type**: Go CLI
**Date**: 2026-05-03

## Pass 1 — as-documented picture

Existing documentation artifacts:

- `spec-loki-profile-manager.md` — Product specification for a local cross-platform CLI/TUI profile manager backed by a OneDrive/Dropbox-synced Loki store. Describes intended V1 behavior including `status`, `switch`, `sync`, `import-skill`, `verify`, `doctor`, `tui`, skill mirroring, Infisical render, snapshots, and rollback.
- `plan-loki-profile-manager.md` — Architecture and implementation plan. Describes Go package layout, store layout, manifest schema, profile layers, activation flow, sync/conflict behavior, skill targets, Infisical integration, testing, and release documentation.
- `tasks-loki-profile-manager.md` — Phase task list. Covers implemented Phases 1-4, newly added Phase 4.5 migration/adoption bootstrap, and future Phases 5-8.
- `docs/handoffs/multi-os-phase-4.5-handoff.md` — Repo-local handoff for continuing on a macOS/Windows dev machine. States Phase 4 is complete, Phase 4.5 is needed before meaningful multi-OS dogfood tests, and lists validation commands.

Missing from repo:

- Root `README.md`.
- `docs/INSTALL.md`.
- `docs/USAGE.md`.
- `docs/ARCHITECTURE.md`.
- AI-operator install companion.
- `docs/DEVELOPMENT.md` or `CONTRIBUTING.md`.
- `LICENSE`.
- `CHANGELOG.md`.

## Pass 2 — as-coded picture

Code is a Go 1.23 CLI module:

- Module: `github.com/allensu/loki-profile-manager` in `go.mod`.
- Entry point: `cmd/loki/main.go` calls `cli.Execute()`.
- CLI framework: Cobra.
- Current user-facing commands from `internal/cli/root.go`:
  - `loki status [--json]`
  - `loki verify [profile] [buckets...] [--json]`
  - `loki switch <profile> [buckets...] [--dry-run] [--yes]`
- Persistent flags:
  - `--store <path>`
  - `--verbose`
- App service layer in `internal/app`:
  - local path resolution
  - SQLite bootstrap
  - structured logging
  - store operations
  - machine registry and policy calls
  - verification
  - profile switch orchestration
- Store layer in `internal/store`:
  - provider discovery for local OneDrive/Dropbox/manual folders
  - `loki/` layout creation and validation
  - base profiles: `common`, `work`, `dev`, `writer`
- Machine layer in `internal/machine`:
  - local machine UUID
  - shared registry `registry/machines.json`
  - per-machine heartbeat files
  - allowed parent profile/bucket policy validation
- Manifest layer in `internal/manifest`:
  - YAML schema v1
  - modes: `symlink`, `copy`, `merge`, `render`
  - formats: `json`, `yaml`, `toml`, `text`
  - path expansion for `~`, env vars, `%VAR%`, `${VAR}`, and manifest target variables
  - source validation and mergeability dry-run
- Profile layer in `internal/profile`:
  - resolves common + parent core + ordered buckets
- Activation engine in `internal/activation`:
  - builds activation plans
  - classifies target safety using `managed_targets`
  - blocks unmanaged files/directories and broken symlinks
  - creates symlinks
  - copies files/directories
  - deep-merges JSON/YAML/TOML with later layer wins
  - renders templates through a secret provider
  - snapshots affected targets
  - rolls back on activation failure
  - updates local managed target state after successful writes
- Infisical integration in `internal/infisical`:
  - injectable CLI runner
  - `infisical run -- printenv NAME` retrieval strategy
  - placeholder renderer for `{{ NAME }}` and `${NAME}`
- SQLite local state in `internal/db/migrations.go`:
  - `kv_state`
  - `managed_targets`
  - `snapshots`
  - `pending_captures`
  - `command_history`
- Verification layer in `internal/verify`:
  - store layout
  - machine policy when available
  - manifest validation
  - skill folder validation
  - mergeability
- Tests cover packages under `internal/*` and currently pass via Docker.

## Findings

### Wrong (docs contradict code)

- `internal/cli/root.go:41` — Cobra long help still says "Phase 1 provides CLI foundations and status bootstrap only." Code now includes `verify` and `switch`, plus the Phase 4 activation engine.
- `spec-loki-profile-manager.md:219` — Spec says no subcommand launches the Bubble Tea TUI. Code currently prints help on no args in `internal/cli/root.go:42-44`.
- `spec-loki-profile-manager.md:341-347` and `plan-loki-profile-manager.md:370-400` — Spec/plan list `tui`, `sync`, `import-skill`, and `doctor` as V1 CLI commands. Code currently registers only `status`, `verify`, and `switch` in `internal/cli/root.go:58-60`.
- `spec-loki-profile-manager.md:189-193` — Spec says activation or sync mirrors skills to Pi/agents/Claude/Copilot/VS Code targets. Current `internal/activation/execute.go` treats `OperationMirror` as a no-op and no CLI surface triggers skill mirroring yet.

### Stale

- `tasks-loki-profile-manager.md:3` — Status says "Ready for implementation" even though Phases 1-4 are implemented and Phase 4.5 is now the next blocking implementation phase.
- `docs/handoffs/multi-os-phase-4.5-handoff.md` — Correct as a continuation note, but not suitable as user documentation. It should remain a handoff, not become README content.

### Missing detail

- No user-facing doc explains the implemented commands and flags from `internal/cli/status.go`, `internal/cli/verify.go`, and `internal/cli/switch.go`.
- No doc explains the current limitation that setup, migration/adoption, skill import/mirroring, sync, doctor, and TUI are not implemented yet.
- No doc explains where local state lives on Windows, macOS, and Linux, despite `internal/config/paths.go` implementing OS-specific paths.
- No doc explains the mandatory unsafe overwrite protection classes implemented in `internal/activation/overwrite.go`.
- No doc explains the store layout produced by `internal/store/layout.go`.
- No doc explains the manifest schema implemented in `internal/manifest/schema.go` and validated by `internal/manifest/validate.go`.
- No doc explains the rollback/snapshot behavior implemented in `internal/activation/snapshot.go` and `internal/activation/rollback.go`.
- No doc explains Infisical render placeholder syntax supported by `internal/infisical/render.go`.

### Missing docs

- `README.md` — Required for any repo.
- `docs/USAGE.md` — Needed for a multi-command CLI.
- `docs/INSTALL.md` — Needed because native Go install/test/smoke differs across Windows/macOS/Linux.
- `docs/ai-ops/install.ai.md` — Needed as AI-operator companion for install/test bootstrap.
- `docs/ARCHITECTURE.md` — Not mandatory for Tier 2, but warranted because the CLI has store, local SQLite, manifests, machine registry, activation engine, and external Infisical CLI integration.
- `docs/DEVELOPMENT.md` — Needed for test commands, Docker-on-Windows commands, package map, and no-real-home testing rule.
- `CHANGELOG.md` — Not urgent before releases; optional.
- `LICENSE` — Missing. Do not create until owner chooses license or confirms private/internal-only status.

### Orphans

- None. Existing docs are planning/handoff artifacts, not orphaned product docs.

### Fine (no changes needed)

- `docs/handoffs/multi-os-phase-4.5-handoff.md` — Accurate continuation note for the current cross-machine workflow.
- `tasks-loki-profile-manager.md` — Newly includes Phase 4.5 migration/adoption bootstrap; useful as internal project plan.
- `spec-loki-profile-manager.md` and `plan-loki-profile-manager.md` — Useful as aspirational/internal planning docs if clearly labeled as not-current user documentation.

## Proposed actions

### Update in place

- `internal/cli/root.go` — Update Cobra long description so `loki --help` matches current code. Remove stale "Phase 1" wording.
- `tasks-loki-profile-manager.md` — Optional: update top status from "Ready for implementation" to "Phase 4 complete; Phase 4.5 next".
- `docs/handoffs/multi-os-phase-4.5-handoff.md` — Leave content intact unless user wants a shorter handoff.

### Create new

- `README.md` — Root user entry point.
  - What Loki is today.
  - Current implementation status.
  - Install-from-source quick path.
  - Quick commands: `status`, `verify`, `switch --dry-run`.
  - Link to usage/install/architecture/handoff.
  - Explicit note that migration/adoption is next and not implemented yet.
- `docs/USAGE.md` — Current CLI reference sourced from Cobra command code.
  - Global flags.
  - `status`.
  - `verify`.
  - `switch`.
  - Exit behavior and safety notes.
  - Not-yet-implemented commands list: `migrate`, `adopt`, `sync`, `import-skill`, `doctor`, `tui`.
- `docs/INSTALL.md` — Cross-platform install and validation.
  - Windows PowerShell.
  - macOS shell.
  - Linux shell.
  - Docker validation command for hosts without Go.
  - Build from source and run tests.
  - No package/binary release instructions until releases exist.
- `docs/ai-ops/install.ai.md` — AI-operator install/test companion.
  - Clone private repo.
  - Verify Go version.
  - Run tests/vet.
  - Build binary.
  - Run smoke commands in temp dirs.
  - Avoid real home paths.
- `docs/ARCHITECTURE.md` — Current architecture only.
  - Component diagram.
  - Switch flow sequence diagram.
  - Store/local-state data model.
  - Safety and rollback design.
  - Current limitations.
- `docs/DEVELOPMENT.md` — Developer workflow.
  - Package map.
  - Test commands.
  - Docker-on-Windows validation.
  - Coding/testing constraints.
  - Secret handling rules.
- `docs/STATUS.md` — Optional, if user wants roadmap/status separated from README.
  - Implemented: Phases 1-4.
  - Next: Phase 4.5.
  - Not implemented: Phases 5-8.

### Diagrams to produce

- System architecture (`flowchart`) in `docs/ARCHITECTURE.md`:
  - Cobra CLI -> app service -> store/machine/profile/manifest/activation/verify/Infisical/db/logging.
  - Synced Loki store vs local machine state boundary.
- Switch flow (`sequenceDiagram`) in `docs/ARCHITECTURE.md`:
  - CLI -> app service -> policy -> planner -> safety -> snapshot -> execute -> managed targets -> heartbeat.
- Data ownership (`flowchart`) in `docs/ARCHITECTURE.md`:
  - YAML store as source of truth; SQLite as local runtime/cache/state; snapshots as local recovery.

### Out of scope

- Do not document `migrate`, `adopt`, `sync`, `import-skill`, `doctor`, or `tui` as working commands until implemented.
- Do not create `LICENSE` without owner license choice.
- Do not create release docs or CHANGELOG history before tags/releases exist.
- Do not document real user secrets, real local config contents, or real synced store paths.
- Do not replace spec/plan/tasks; they remain planning documents.
