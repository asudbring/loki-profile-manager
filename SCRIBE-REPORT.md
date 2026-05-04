# Scribe Report: loki-profile-manager

## Current documentation state

- `README.md` — root overview, implemented command list, install-from-source quick path, snapshot restore examples, and documentation links.
- `docs/USAGE.md` — current CLI reference for `status`, `verify`, `switch`, `snapshots list/show/restore`, `machine`, `adopt`, and `migrate` commands.
- `docs/INSTALL.md` — cross-platform install and validation for Windows, macOS, Linux, Docker fallback, and smoke testing.
- `docs/ARCHITECTURE.md` — system overview, data ownership, store layout, local state, SQLite schema, manifest model, switch flow, safety model, rollback, and current limitations.
- `docs/DEVELOPMENT.md` — package map, test/build commands, fixture guidance, and secret-handling rules.
- `docs/ai-ops/install.ai.md` — AI-operator companion for cloning, validating, building, and smoke-testing the private repository.
- `tests/cross-machine-dogfood-copilot.ai.md` — Windows ARM64 disposable target dogfood prompt, including targeted snapshot restore.
- `tests/real-dotfile-dogfood-copilot.ai.md` — Windows ARM64 real low-risk dotfile switch/status/snapshot dry-run prompt.
- `tests/real-dotfile-targeted-restore-consent-copilot.ai.md` — consent-gated real-dotfile targeted restore prompt.

## Latest validation

- Windows ARM64 disposable target restore: passed with `dogfood-crossos` and `C:\Users\allen\loki-dogfood\probe.txt`.
- Windows ARM64 real-dotfile targeted restore: passed for `C:\Users\allen\.config\git\ignore`.
- Windows ARM64 real-dotfile targeted restore: passed for `C:\Users\allen\.gitconfig`.
- GitHub Actions CI: green on `4003493 fix: serialize machine id creation`.
- Milestone tags pushed:
  - `targeted-snapshot-restore-dogfood`
  - `real-dotfile-targeted-restore-dogfood`

## Safety decisions documented

- Snapshot restore `--yes` requires a matching prior `--dry-run` guard.
- Full snapshot restore without `--target` requires an interactive `RESTORE <snapshot-id>` confirmation phrase before service execution.
- Targeted restore with `--target <path>` still requires the dry-run guard but does not prompt for full active-state restore consent.
- Real-dotfile dogfood used targeted restore only.
- Full real-dotfile restore was intentionally not run.
- Sensitive-looking paths such as `.ssh`, `.env`, tokens, credentials, private keys, `.pem`, and `.key` remain blocked/redacted by default.

## Diagrams

- 1 system architecture flowchart in `docs/ARCHITECTURE.md`.
- 1 data ownership flowchart in `docs/ARCHITECTURE.md`.
- 1 switch flow sequence diagram in `docs/ARCHITECTURE.md`.

## Deferred / out of scope

- `LICENSE` — not created because no license choice exists. README states all rights reserved until a license file is added.
- `CHANGELOG.md` — not created because there are no formal releases yet.
- `docs/DEPLOY.md` — not applicable; this is a local CLI with no deploy target.
- `sync`, `import-skill`, `doctor`, and `tui` — documented as planned but not implemented.
- Full real-dotfile snapshot restore dogfood — deferred until there is explicit need beyond targeted restore validation.

## Suggested follow-ups

- Add `CHANGELOG.md` if tags become release milestones.
- Re-run Scribe after `sync`, `import-skill`, `doctor`, or TUI land.
- Consider a non-interactive full-restore override only if a future automation workflow needs it; keep default interactive consent.
