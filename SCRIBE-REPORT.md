# Scribe Report: loki-profile-manager

## Current documentation state

- `README.md` — root overview, implemented command list, install-from-source quick path, snapshot restore examples, and documentation links.
- `docs/USAGE.md` — current CLI reference for `status`, `verify`, `doctor`, `switch`, `sync`, `snapshots list/show/restore`, `machine`, `adopt`, and `migrate` commands.
- `docs/INSTALL.md` — release archive install, checksum verification, source install, cross-platform validation, Docker fallback, and smoke testing.
- `docs/ARCHITECTURE.md` — system overview, data ownership, store layout, local state, SQLite schema, manifest model, switch flow, safety model, rollback, and current limitations.
- `docs/DEVELOPMENT.md` — package map, test/build/release commands, fixture guidance, and secret-handling rules.
- `CHANGELOG.md` — pre-release dogfood and safety milestones for snapshot reporting, guarded restore, targeted restore, real-dotfile targeted restore, and full restore consent.
- `docs/ai-ops/install.ai.md` — AI-operator companion for cloning, validating, building, and smoke-testing the private repository.
- `tests/cross-machine-dogfood-copilot.ai.md` — Windows ARM64 disposable target dogfood prompt, including targeted snapshot restore.
- `tests/real-dotfile-dogfood-copilot.ai.md` — Windows ARM64 real low-risk dotfile switch/status/snapshot dry-run prompt.
- `tests/real-dotfile-targeted-restore-consent-copilot.ai.md` — consent-gated real-dotfile targeted restore prompt.

## Latest validation

- Windows ARM64 disposable target restore: passed with `dogfood-crossos` and `C:\Users\allen\loki-dogfood\probe.txt`.
- Windows ARM64 real-dotfile targeted restore: passed for `C:\Users\allen\.config\git\ignore`.
- Windows ARM64 real-dotfile targeted restore: passed for `C:\Users\allen\.gitconfig`.
- Windows ARM64 disposable full-restore consent prompt: passed for `dogfood-crossos` and `C:\Users\allen\loki-dogfood\probe.txt`; wrong consent blocked, exact `RESTORE <snapshot-id>` consent restored only the disposable target and active local state.
- GitHub Actions CI: green on `34fde0d feat: gate full snapshot restore consent`.
- Milestone tags pushed:
  - `targeted-snapshot-restore-dogfood`
  - `real-dotfile-targeted-restore-dogfood`

## Safety decisions documented

- Snapshot restore `--yes` requires a matching prior `--dry-run` guard.
- Full snapshot restore without `--target` requires an interactive `RESTORE <snapshot-id>` confirmation phrase before service execution.
- Targeted restore with `--target <path>` still requires the dry-run guard but does not prompt for full active-state restore consent.
- Real-dotfile dogfood used targeted restore only.
- Full real-dotfile restore was intentionally not run.
- Full restore consent was dogfooded only on a disposable target snapshot, not real dotfiles.
- Sensitive-looking paths such as `.ssh`, `.env`, tokens, credentials, private keys, `.pem`, and `.key` remain blocked/redacted by default.

## Diagrams

- 1 system architecture flowchart in `docs/ARCHITECTURE.md`.
- 1 data ownership flowchart in `docs/ARCHITECTURE.md`.
- 1 switch flow sequence diagram in `docs/ARCHITECTURE.md`.
- 1 sync flow sequence diagram and 1 skill import flow sequence diagram in `docs/ARCHITECTURE.md`.

## Deferred / out of scope

- `LICENSE` — not created because no license choice exists. README states all rights reserved until a license file is added.
- `docs/DEPLOY.md` — not applicable; this is a local CLI with no deploy target.
- `import-skill` zip/markdown import and `tui` — documented as planned but not implemented. Skill folder import is implemented. `sync` is implemented only for provider conflict-copy cleanup, not watcher capture/full reconciliation.
- Full real-dotfile snapshot restore dogfood — deferred until there is explicit need beyond targeted restore validation.

## Suggested follow-ups

- Use semver tags such as `v0.1.0-doctor.1` for packaged prereleases; milestone tags remain dogfood markers only.
- Re-run Scribe after `import-skill` zip/markdown import, watcher/full sync, or TUI land.
- Consider a non-interactive full-restore override only if a future automation workflow needs it; keep default interactive consent.
