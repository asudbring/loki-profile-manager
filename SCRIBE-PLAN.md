# Scribe Plan: loki-profile-manager

**Tier**: 3
**Project type**: multi-command Go CLI with Bubble Tea TUI, npm wrapper, release packaging scripts, and GitHub Actions workflows
**Date**: 2026-05-08

## Findings

### Wrong (docs contradict code or current repo state)
- Initial public-readiness gaps were found in `README.md`, `docs/ai-ops/install.ai.md`, and `docs/ai-ops/windows-arm64-vm-test.ai.md`. These have been updated in the Scribe pass.
- `docs/installer-release-plan.md` — describes implementation planning for installer work that is now mostly implemented. Keep only if it is clearly marked historical.
- `docs/store-tui-management-plan.md` and `docs/TUI_PLAN.md` — planning documents for features now implemented. Keep only if they are marked historical, or move them out of the main public doc path.

### Stale
- `plan-loki-profile-manager.md`, `spec-loki-profile-manager.md`, and `tasks-loki-profile-manager.md` still read like active project-control documents. README already warns they are planning docs, but they need a historical/status banner.
- `docs/test-plans/windows-vm-infisical-tui-smoke.md` and `tests/*dogfood*.ai.md` are useful validation artifacts, but some are named around internal dogfood workflows rather than reusable public test procedures.
- `CHANGELOG.md` has an Unreleased section with many dogfood milestones but no entry summarizing the public-readiness/license/module-path change.

### Missing detail
- `docs/RELEASE.md` documents the no-Actions local fallback, but it should also state the normal public-repo release path once Actions is free on public repositories.
- `docs/RELEASE.md` should explicitly say existing local release tags must point at the current commit before upload clobbering.
- `docs/INSTALL.md` mentions release assets and npm tarballs but should call out the public GitHub Releases path after visibility changes.
- `docs/ARCHITECTURE.md` should be checked against the current module path, release-local script, Infisical local env loading, zip import, and TUI flows.
- `docs/USAGE.md` should be regenerated or spot-checked against Cobra command definitions after recent command additions.

### Missing docs
- `docs/ai-ops/release.ai.md` — AI-operator companion for `docs/RELEASE.md` local release and public Actions release procedures.
- `CONTRIBUTING.md` — optional but recommended once public, even if it says external contributions are reviewed case-by-case.
- A short public-readiness/security audit summary could live in `docs/RELEASE.md` or `SECURITY.md`; do not commit raw scanner output.

### Orphans
- Removed old `SCRIBE-PLAN.md`, `SCRIBE-REPORT.md`, and `docs/handoffs/multi-os-phase-4.5-handoff.md` because they were stale internal handoff artifacts.
- Remaining planning docs are not orphans if marked historical. Without banners, they look like current docs.

### Fine (no changes needed or already corrected)
- `LICENSE` — MIT license added.
- `SECURITY.md` — public vulnerability reporting policy added.
- `npm/package.json` — license now MIT.
- `go.mod` and internal imports — module path now matches `github.com/asudbring/loki-profile-manager`.
- `.github/workflows/ci.yml` and `.github/workflows/release.yml` — include `scripts/release-local.sh` in shell syntax checks.
- Current tracked-file secret/PII scans pass with zero findings after sanitization.

## Proposed actions

### Update in place
- `README.md`
  - Use final public/MIT wording.
  - Keep concise install path and link to `docs/INSTALL.md`, `docs/RELEASE.md`, `SECURITY.md`, and `LICENSE`.
  - Keep planning-doc warning but point readers to current docs first.
- `docs/INSTALL.md`
  - Refresh GitHub Releases wording for a public repo.
  - Verify Windows/macOS/Linux install examples against current scripts.
  - Keep npm tarball as recommended path.
- `docs/RELEASE.md`
  - Add normal public-repo release path using GitHub Actions.
  - Keep local no-Actions fallback.
  - Add safety note for tag/commit matching and output-dir deletion guard.
- `docs/DEVELOPMENT.md`
  - Align module path, validation commands, Actions/local fallback, and public contribution expectations.
- `docs/ARCHITECTURE.md`
  - Refresh component diagram and data flow for current packages: CLI, app service layer, activation, store/machine/manifest/verify, Infisical, TUI, npm wrapper, release scripts.
- `docs/USAGE.md`
  - Reconcile command/flag list with `internal/cli/*` Cobra definitions.
  - Keep secret examples placeholder-only.
- `CHANGELOG.md`
  - Add public-readiness entry: MIT license, module path change, release-local script, public docs cleanup, security scan summary.
- `docs/ai-ops/install.ai.md`
  - Remove GitHub authentication as a default prerequisite.
  - Keep optional auth note only for rate-limited or private forks.
- `docs/ai-ops/windows-arm64-vm-test.ai.md`
  - Same auth cleanup.
  - Keep OneDrive smoke test generic.
- `docs/TUI_PLAN.md`, `docs/store-tui-management-plan.md`, `docs/installer-release-plan.md`, `plan-loki-profile-manager.md`, `spec-loki-profile-manager.md`, `tasks-loki-profile-manager.md`
  - Add historical/planning banner or move to a `docs/planning/` folder.

### Create new
- `docs/ai-ops/release.ai.md` — deterministic AI-operator release procedure for local package validation, optional GitHub Release upload, and public Actions rerun.
- `CONTRIBUTING.md` — setup, tests, PR expectations, security caveat, and issue guidance.
- `SCRIBE-REPORT.md` — final Scribe output after doc updates.

### Diagrams to produce
- `docs/ARCHITECTURE.md` — update or add a Mermaid `flowchart` showing CLI/TUI -> app service -> store/manifest/machine/activation/Infisical layers.
- `docs/ARCHITECTURE.md` — update or add a Mermaid `sequenceDiagram` for `loki switch` safety flow.
- `docs/RELEASE.md` or `docs/ARCHITECTURE.md` — optional Mermaid `flowchart` for public Actions release path and local fallback.

### Out of scope
- GitHub visibility flip. Requires explicit user approval after reviewing the history exposure note.
- Git history rewrite or fresh public repository creation. Current tree is sanitized, but old commits still contain historical personal paths and old module owner strings. No high-confidence secrets were found in history. If historical personal paths are unacceptable, use a fresh public repo or rewrite history before making anything public.
- Stable `v0.1.0` release decision. Make public first, get public Actions green, then choose dogfood vs stable release.

## Approval request

Approve this Scribe plan to update docs. After approval, Scribe will write the doc changes above, regenerate `SCRIBE-REPORT.md`, rerun secret/PII scans, and rerun local validation.
