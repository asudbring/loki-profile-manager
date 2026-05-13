# Scribe Plan: loki-profile-manager

**Tier**: 3
**Project type**: Go CLI with npm/release installers, profile-store workflows, migration/adoption flows, TUI, validation scripts, and AI-operator procedures
**Date**: 2026-05-12

## Inventory

### Documentation artifacts reviewed
- `README.md` — top-level project description, install links, quick examples, documentation index.
- `CHANGELOG.md` — release history through `v0.1.5` and unreleased changes.
- `CONTRIBUTING.md` — contribution workflow.
- `SECURITY.md` — security reporting and secret-handling guidance.
- `docs/INSTALL.md` — current install/build/smoke guide.
- `docs/USAGE.md` — CLI command reference.
- `docs/ARCHITECTURE.md` — component and data-flow documentation.
- `docs/DEVELOPMENT.md` — package map, build/test commands, release packaging notes.
- `docs/RELEASE.md`, `docs/installer-release-plan.md` — release/package procedures and planning notes.
- `docs/TUI_PLAN.md`, `docs/store-tui-management-plan.md` — implementation plans retained as historical/planning docs.
- `docs/ai-ops/install.ai.md`, `docs/ai-ops/release.ai.md`, `docs/ai-ops/windows-arm64-vm-test.ai.md` — AI-operator procedures.
- `docs/test-plans/windows-installer-vm-smoke.md`, `docs/test-plans/windows-vm-infisical-tui-smoke.md` — manual validation plans.
- `tests/*.ai.md` — Copilot/agent dogfood and restore validation prompts.

### As-coded picture
- Entry point: `cmd/loki/main.go`.
- Go packages: `internal/activation`, `internal/app`, `internal/cli`, `internal/config`, `internal/db`, `internal/doctor`, `internal/infisical`, `internal/machine`, `internal/manifest`, `internal/migration`, `internal/profile`, `internal/secrets`, `internal/skills`, `internal/store`, `internal/storesync`, `internal/tui`, `internal/verify`.
- Commands exposed by current CLI: `adopt`, `completion`, `doctor`, `help`, `import-skill`, `machine`, `migrate`, `secrets`, `snapshots`, `status`, `store`, `switch`, `sync`, `tui`, `verify`.
- Current release state: repo head `1b5a20d`, tag `v0.1.5`; local repo clean before this plan rewrite.
- Install surfaces: Go source build, GitHub release binaries/install scripts, npm wrapper under `npm/bin/loki.js`, Windows PowerShell installer scripts, shell installer scripts, CI/release workflows.

## Findings

### Wrong (docs contradict code or current behavior)
- `docs/INSTALL.md` — existing headings are mostly package/build oriented. It does not match the now-validated user workflow: fresh machine bootstrap, then safe profile activation from a Loki store.
- `docs/INSTALL.md` — no section exists for an existing machine with unmanaged/local profiles that have not been migrated yet, even though the CLI exposes `migrate local`, `migrate repo`, `adopt`, `verify`, `switch --dry-run`, and `switch --capture-local` specifically for that path.
- `docs/ai-ops/install.ai.md` — describes development checkout validation more than release/npm install and first-run setup. It is not a complete AI-operator companion for the human install guide.

### Stale
- `README.md` — quick install/routing needs to reflect the current validated release (`v0.1.5`), npm package path, and the two supported first-run paths: fresh machine and existing unmanaged machine.
- `docs/ai-ops/windows-arm64-vm-test.ai.md` and `docs/test-plans/windows-vm-infisical-tui-smoke.md` — need alignment with the completed app/manual switch validation and the current Loki-managed prompt/app-settings flow.
- `docs/USAGE.md` — command reference already covers many workflows, but it must be checked against current help output for `switch --capture-local`, `migrate local`, `migrate repo`, `adopt`, `store`, `machine register`, `verify`, `doctor`, `secrets --infisical`, and snapshots.

### Missing detail
- Fresh-machine install path needs OS-specific steps for Windows, macOS, and Linux: install Loki, sign in to sync provider, choose/use/init store, register machine, verify profile, dry-run switch, real switch, inspect status, and run app smoke tests.
- Existing-machine path needs no-data-loss guidance: inventory current dotfiles/settings, back up unmanaged targets, run migration/adoption dry-runs, review generated manifests, verify, switch dry-run, resolve blockers, then use `--yes` or `--capture-local --yes` only when appropriate.
- Windows notes need current profile-specific details: OneDrive/Parallels redirected Documents, `${DOCUMENTS}` / `${DOCUMENTS_DIR}` expansion, symlink requirements, npm PATH, and OneDrive lock/retry guidance.
- Shell/app smoke checks need explicit PowerShell, Git Bash, VS Code, Codex/Pi/Claude/Copilot, Git, and Warp expectations without depending on legacy `.dotfiles` repos.
- `--capture-local` needs a concise explanation in docs: safe managed-target local changes are written back to the store before switching; it does not bypass unmanaged overwrite protection.

### Missing docs
- No new top-level document is required if `docs/INSTALL.md` becomes the detailed guide.
- `docs/ai-ops/install.ai.md` needs expansion into a true AI-operator companion for both first-run paths.
- `SCRIBE-REPORT.md` needs regeneration after edits and validation.

### Orphans
- No docs should be deleted in this pass.
- Planning docs under `docs/*PLAN*.md` are historical/planning material; leave them unless a later cleanup request asks to archive or remove them.

### Fine (no changes needed or only light touch expected)
- `LICENSE`, `SECURITY.md`, and `CONTRIBUTING.md` appear structurally fine for this docs pass.
- `docs/ARCHITECTURE.md` is already substantial; update only if the install/switch-flow audit finds stale safety or active-profile-marker details.
- `docs/ai-ops/release.ai.md` is release-specific; update only if cross-links need adjustment.

## Proposed actions

### Update in place
- `README.md` — refresh status/install routing, point to fresh-machine and existing-machine sections, keep quick examples aligned with current CLI flags.
- `docs/INSTALL.md` — rewrite/expand into the detailed install and first-run guide requested:
  - Windows / macOS / Linux install sections.
  - Fresh machine path.
  - Existing machine with existing profiles not migrated path.
  - Safe switch/capture workflow.
  - App/manual smoke checklist based on the successful validation.
  - Troubleshooting for PATH, symlinks, OneDrive locks, redirected Documents, and stale local profile markers.
- `docs/USAGE.md` — align command reference and safety notes with the install guide.
- `docs/ai-ops/install.ai.md` — make a structured AI-operator companion for npm/release/source installs plus fresh/existing-machine first-run paths.
- `docs/ai-ops/windows-arm64-vm-test.ai.md` — align VM validation procedure with the new install guide and current `v0.1.5` flow.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` — update TUI/manual smoke expectations and link to the install guide.
- `tests/cross-machine-dogfood-copilot.ai.md` — update only stale store/switch/migration wording.
- `tests/real-dotfile-dogfood-copilot.ai.md` — update only stale real-target safety wording.
- `tests/real-dotfile-targeted-restore-consent-copilot.ai.md` — update only if restore wording conflicts with install/safety docs.
- `docs/ARCHITECTURE.md` — light update only if needed for active profile marker, Windows Documents expansion, or switch/capture safety.
- `docs/DEVELOPMENT.md` — light update only if docs validation commands or test matrix references are stale.
- `CHANGELOG.md` — add an Unreleased documentation note for the install-guide refresh and completed app/manual switch validation.
- `SCRIBE-REPORT.md` — regenerate final report after edits and validation.

### Create new
- None planned. Use existing `docs/INSTALL.md` and `docs/ai-ops/install.ai.md`.

### Diagrams to produce
- No new diagram planned unless `docs/ARCHITECTURE.md` needs a small switch/capture flow diagram update.

### Out of scope
- No code changes.
- No deletion of VM or local `.dotfiles` backups.
- No migration of the local Mac `.dotfiles-work` or `.dotfiles-play` repos.
- No secret inspection, token printing, or Infisical value output.
- No real profile switch on the local Mac during docs validation unless separately approved.
- No release/tag/npm publish.

## Validation plan

After approved doc edits:
- `go test ./...`
- `go vet ./...`
- `go mod verify`
- `go build -trimpath -o /tmp/loki-doc-audit ./cmd/loki`
- `bash -n scripts/install.sh scripts/uninstall.sh scripts/package-release.sh scripts/package-npm.sh scripts/release-local.sh scripts/validate-cross.sh scripts/parallels-windows-admin-probe.sh`
- `node --check npm/bin/loki.js`
- PowerShell parser check for `scripts/*.ps1` when `pwsh` is available.
- Secret-pattern scan across `README.md`, `docs/`, `tests/`, and root docs.
- Markdown link/command spot checks for every command used in `docs/INSTALL.md`.

## Approval request

Scribe requires plan approval before writing docs. Approve this plan and I will update the documentation files listed above, then run validation and regenerate `SCRIBE-REPORT.md`.
