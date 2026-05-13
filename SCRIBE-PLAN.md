# Scribe Plan: loki-profile-manager

**Tier**: 3
**Project type**: Go CLI with npm/release installers, synced profile-store workflows, migration/adoption flows, TUI, validation scripts, and AI-operator procedures
**Date**: 2026-05-13

## Inventory

### Documentation artifacts reviewed
- `README.md` — top-level project description, install path, first-run routing, quick examples, and documentation index.
- `CHANGELOG.md` — release history and current unreleased/release notes.
- `docs/INSTALL.md` — install methods, fresh-machine setup, existing-machine migration, smoke checks, and troubleshooting.
- `docs/USAGE.md` — CLI command and flag reference.
- `docs/ARCHITECTURE.md` — component model, store/local-state model, switch safety flow, TUI flow, and release packaging.
- `docs/DEVELOPMENT.md` — package map, test/build commands, release packaging, and implementation phase notes.
- `docs/RELEASE.md` and `docs/ai-ops/release.ai.md` — tag-based release and local fallback release procedures.
- `docs/ai-ops/install.ai.md` — AI-operator install/bootstrap procedure.
- `docs/ai-ops/release.ai.md` — AI-operator local packaging and post-release verification procedure.
- `docs/ai-ops/windows-arm64-vm-test.ai.md` — AI-operator Windows ARM64 VM validation procedure.
- `docs/PROFILES.md` — profile/bucket creation and organization guidance.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` — manual Windows VM Infisical/TUI smoke plan.
- `docs/test-plans/windows-installer-vm-smoke.md` — manual Windows installer smoke plan.
- `CONTRIBUTING.md`, `SECURITY.md`, and root planning/test prompt docs were spot-checked for release, secret, and first-run claims.

### As-coded picture
- Entry point: `cmd/loki/main.go`.
- CLI commands include `status`, `store`, `verify`, `switch`, `sync`, `tui`, `import-skill`, `secrets`, `doctor`, `snapshots`, `machine`, `migrate`, and `adopt`.
- New first-run setup code since `v0.1.5`:
  - `internal/cli/secrets_configure.go` adds `loki secrets configure infisical`.
  - `internal/app/secrets_configure.go` writes local Infisical Universal Auth config and validates host values.
  - `internal/tui/secrets.go` exposes masked Infisical setup from the TUI Secrets screen.
  - `internal/app/switch_unmanaged_backup.go` and `internal/cli/switch.go` add `switch --backup-unmanaged --yes`.
  - `internal/infisical/cli.go` mints Universal Auth tokens through the Infisical API and validates configured hosts.
- Release state: local tag `v0.1.6`, npm latest `@asudbring/loki-profile-manager@0.1.6`, release workflow passed.

## Findings

### Wrong (docs contradict code or current release state)
- `README.md:Status` — claimed current dogfood release `v0.1.5` as the current release after `v0.1.6` was published to npm.
- `CHANGELOG.md:Unreleased` — held `v0.1.6` documentation/setup changes under `Unreleased` after the `v0.1.6` release.

### Stale
- `docs/INSTALL.md:Recent dogfood validation` — referenced only `v0.1.5`; still true as the latest full Windows app/manual dogfood, but needed a `v0.1.6` release-context note for new setup flows.
- `docs/DEVELOPMENT.md:Current implementation phase` — omitted first-install unmanaged-backup remediation and interactive Infisical configuration from current hardening focus.

### Missing detail
- `docs/INSTALL.md:Troubleshooting quick table` — unmanaged blocker and secret-render troubleshooting did not prefer the new `--backup-unmanaged --yes` / `secrets configure infisical` paths.
- `docs/ai-ops/install.ai.md` — AI-operator first-run activation could not intentionally use `switch --backup-unmanaged --yes` when the synced Loki store should win, and had no optional Infisical setup step.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` — needed an explicit post-release npm registry check for `v0.1.6`.
- `docs/ai-ops/windows-arm64-vm-test.ai.md` — optional real-profile validation did not document the store-wins `--backup-unmanaged --yes` branch.
- `docs/ai-ops/release.ai.md` — local tarball verification existed, but post-Actions npm registry verification did not check new `v0.1.6` command surfaces.
- `docs/test-plans/windows-installer-vm-smoke.md` — example version still used an old installer prerelease tag.

### Missing docs
- None. Existing Tier 3 doc set already includes README, install, usage, architecture, development, release, AI-operator install/release, profile guide, changelog, security, and validation plans.

### Orphans
- Historical planning docs remain intentionally marked/planned artifacts. No orphan requiring removal.

### Fine (no changes needed)
- `docs/USAGE.md` already documents `secrets configure infisical`, TUI `c`, `switch --backup-unmanaged`, and first-run safety behavior.
- `docs/ARCHITECTURE.md` already describes Infisical setup/security and unmanaged-backup switch safety.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` already covers the wizard and unmanaged backup smoke path; only post-release npm validation was missing.
- `docs/RELEASE.md` remains accurate for tag-based release and npm publish flow.
- `CONTRIBUTING.md` and `SECURITY.md` remain accurate; no new secret-handling rule needed.

## Proposed actions

### Update in place
- `README.md` — distinguish current npm release `v0.1.6` from latest full Windows app/manual dogfood validation `v0.1.5`.
- `CHANGELOG.md` — move released setup/remediation notes into a new `v0.1.6 — 2026-05-13` section and reset `Unreleased`.
- `docs/INSTALL.md` — add `v0.1.6` setup-flow note and update troubleshooting for `--backup-unmanaged --yes` and `secrets configure infisical`.
- `docs/DEVELOPMENT.md` — mention first-install unmanaged-backup remediation and Infisical configuration in current hardening focus.
- `docs/ai-ops/install.ai.md` — add `BACKUP_UNMANAGED` variable and guarded activation path for headless first-run remediation; add optional Infisical setup via `RUN_INFISICAL_SETUP` and `INFISICAL_SETUP_METHOD`.
- `docs/ai-ops/release.ai.md` — add post-Actions npm registry verification for the published version and new command surfaces.
- `docs/ai-ops/windows-arm64-vm-test.ai.md` — add optional real-profile store-wins unmanaged-backup branch.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` — add npm registry release check for `v0.1.6`.
- `docs/test-plans/windows-installer-vm-smoke.md` — update example version to current release tag under test.
- `SCRIBE-PLAN.md` — replace stale `v0.1.5` Scribe plan with this audit.
- `SCRIBE-REPORT.md` — summarize doc updates and validation.

### Create new
- None.

### Diagrams to produce
- None. Existing architecture diagrams remain current; new changes are command/setup flows, not new components.

### Out of scope
- Publishing another npm release for documentation-only edits unless user requests it.
- Reworking historical planning docs (`spec-*`, `plan-*`, `tasks-*`) beyond keeping them clearly listed as planning artifacts.
- Running real Windows VM/manual smoke checks from this macOS session.
