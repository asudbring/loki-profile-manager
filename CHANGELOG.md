# Changelog

All notable Loki Profile Manager changes are tracked here.

This project uses 0.x semver. Hyphenated tags are dogfood or prerelease milestones; plain semver tags are public releases.

## Unreleased

## v0.1.20 — 2026-05-23

Feature release for in-TUI switch blocker remediation.

- TUI switch dry-run now enumerates per-target blockers (capture conflicts, unsupported merge/symlink captures, cleanup blockers, unmanaged blockers) with actionable hints instead of a single generic `Blocker:` line.
- Added `c` and `b` hotkeys on the switch screen to toggle `--capture-local` and `--backup-unmanaged` without leaving the TUI. Toggles clear the cached dry-run so the fingerprint guard forces a fresh validation before execute.
- Switch execute recheck now preserves `Yes` so `--backup-unmanaged` dry-run rechecks pass app-layer validation.

## v0.1.19 — 2026-05-20

Feature release for markdown skill import.

- Added standalone `.md` file support to `loki import-skill`. Markdown files are auto-wrapped into a valid skill folder with derived frontmatter when `name` or `description` are missing.

## v0.1.18 — 2026-05-20

Patch release for code review fixes.

- Removed redundant file hash computation in managed-state doctor scan (performance).
- Added `repair_failed` detail to `loki doctor --repair-managed-state --json` output for accurate repair outcome reporting.
- Scoped `id-token: write` permission to only the npm-publish job in the release workflow (least privilege).

## v0.1.17 — 2026-05-20

Documentation release for current GitHub and npm docs.

- Updated the README and user docs to show the current npm release, clarify that `loki import-skill` supports valid skill folders and zip archives, and avoid implying the command is missing.
- Refreshed Windows smoke-test docs to install `@latest` and verify `import-skill` help instead of hardcoding an old npm version.

## v0.1.16 — 2026-05-20

Documentation release for the npm package README.

- Refreshed the GitHub and npm README with the current npm release number and managed-state doctor repair usage.

## v0.1.15 — 2026-05-20

Patch release for safe managed-target state repair.

- Added `loki doctor --repair-managed-state [--write-safe-files]` to detect and repair safe stale managed-target SQLite state after manifest mode/hash changes.

## v0.1.14 — 2026-05-19

Feature release for Loki self-update support.

- Added `loki update` to install the latest `@asudbring/loki-profile-manager` npm build with `npm install -g @asudbring/loki-profile-manager@latest`.
- Added cached npm update notices for human CLI commands, with 24-hour local SQLite caching and `LOKI_NO_UPDATE_CHECK=1` opt-out.

## v0.1.13 — 2026-05-19

Patch release for Windows CI coverage on plugin import.

- Fixed the new plugin-import activation-plan regression test to compare managed Pi settings targets portably across Windows and Unix path formats.

## v0.1.12 — 2026-05-19

Feature release for plugin bundle import.

- Added `loki import-plugin <source>` for multi-runtime plugin/package bundles with explicit `--runtime` selection and `--runtime all` opt-in.
- Added a Pi runtime adapter that stores canonical bundles under `plugins/<name>`, activates them through managed `~/.pi/agent/packages/<name>`, and merges Pi settings packages without duplicate target conflicts.
- Added Copilot manual install guidance while intentionally avoiding direct edits to Copilot internals, plus plan-only warnings for Claude, Codex, and VS Code adapters.
- Hardened plugin import safety: dry-runs write nothing, non-dry-run copy staging stays inside the store, `.git` directories are excluded, existing destinations require `--overwrite`, and destination state is rechecked under the store lock.

## v0.1.11 — 2026-05-15

Patch release for store migration retarget repair.

- Fixed store migration symlink retargeting to preserve broken-but-managed symlinks by moving them from the old store root to the new store root even when the source file is missing.

## v0.1.10 — 2026-05-15

Feature release for observable, staged store migration.

- Reworked `loki store migrate` into a staged V2 pipeline: rich cloud-only preflight, opt-in `--hydrate`, progress events, per-file timeout, hidden staging copy, validated promotion, interrupted staging cleanup, transactional DB rewire, and active managed-symlink retargeting.
- Added cross-platform cloud placeholder detector plumbing: macOS dataless flags, conservative Windows Cloud Files attributes where available, and safe no-op detection on Linux/other platforms while retaining staging/progress/cleanup behavior.
- Updated store migration docs for OneDrive/Dropbox cloud-only handling, `--hydrate`, `--file-timeout`, `--progress-interval`, and `--cleanup`.

## v0.1.9 — 2026-05-15

Patch release for macOS cloud-only source-store safety.

- Changed `loki store migrate` dry-run planning to detect macOS dataless/cloud-only source files before copy, fail quickly, and report the first file that must be downloaded.
- Prevented silent long-running migration attempts when OneDrive/File Provider would need to hydrate hundreds of source files during copy.

## v0.1.8 — 2026-05-15

Feature release for first-class store-root migration.

- Added `loki store migrate --to <path> (--dry-run|--yes)` to copy an existing valid Loki store to a missing/empty destination and rewire local SQLite state by default.
- Added `--copy-only`, `--capture-local`, provider labels, destination safety checks, provider conflict-copy blocking, and transaction-safe managed-target source-path rebasing.
- Added OneDrive for Business and Dropbox discovery hardening, including Windows `OneDriveCommercial`, macOS CloudStorage OneDrive tenant folders, and Dropbox `info.json` parsing.
- Added regression coverage for dry-run no-write behavior, copy validation, symlink/nested destination safety, local state rebase, and CLI JSON output.
- Updated README, install, usage, and architecture docs for OneDrive Business and Dropbox store migration workflows.

## v0.1.7 — 2026-05-14

Patch release for Infisical first-configure hardening.

- Fixed `loki secrets configure infisical` and the TUI Infisical wizard to validate Universal Auth before writing the local env file, preserving existing config on validation failure.
- Fixed sanitized status reporting for invalid local machine-auth config so `loki secrets status` now points users to rerun configure or remove the local env file to use CLI login.
- Fixed `loki secrets --infisical` to succeed when interactive Infisical CLI login is already ready and only `.infisical.json` supplies project/env metadata.
- Added regression coverage for failed validation, ambient `INFISICAL_*` isolation, CLI-login fallback, and sanitized output.

## v0.1.6 — 2026-05-13

Feature release for first-run setup remediation and safer Infisical configuration.

- Added `loki secrets configure infisical`, an interactive local-only Infisical Universal Auth setup wizard that prompts for project ID, environment, client ID, masked client secret/key, and optional host/API URL.
- Added TUI Secrets-screen Infisical configuration on `c`, with masked secret input and form scrubbing after submit/cancel.
- Added `loki switch <profile> [buckets...] --backup-unmanaged --yes` to move unmanaged file/directory blockers into machine-local backups, write a backup `manifest.json`, re-run safety checks, and then activate when the synced Loki store should win.
- Hardened Infisical configuration and runtime auth: no persisted minted tokens from noninteractive setup when Universal Auth is configured, stale host/token values cleared on explicit configure, plain HTTP hosts rejected except loopback/local dev, runtime host validation added, and Universal Auth token minting moved to the Infisical API instead of `infisical login --client-secret` process args.
- Documentation: refreshed README, install, usage, architecture, AI-operator, and Windows VM smoke docs for the new Infisical wizard and unmanaged-backup first-run flow.
- Documentation: added `docs/PROFILES.md` for starting from a blank store, naming profiles/buckets, registering machines, adding settings, and organizing skills.
- Validation note: Windows VM app/manual switch dogfood passed with Loki-owned shell prompt state and app settings after legacy profile repositories were removed from active paths. Release workflow and npm smoke validation passed for `v0.1.6`.
- Planned: `import-skill` markdown conversion and Azure Key Vault/other secret providers remain unimplemented.

## v0.1.5 — 2026-05-11

Patch release for Loki-owned shell prompt/profile activation on redirected Windows profile folders.

- Added Windows Known Folder support via `${DOCUMENTS}`/`${DOCUMENTS_DIR}` target expansion so PowerShell profiles land under redirected Documents folders such as Parallels `C:\Mac\Home\Documents`.
- Added a local active-profile marker at Loki's state directory so shell prompts can read `profile:buckets` without depending on legacy `.dotfiles` repositories.
- Updated snapshot restore to refresh the active-profile marker after full restores.

## v0.1.4 — 2026-05-11

Patch release for Infisical setup automation during clean-machine bootstrap.

- Added `loki secrets --infisical` to create/update local Infisical machine-auth env config from existing local inputs and run readiness checks without printing secret values.

## v0.1.3 — 2026-05-11

Patch release for profile switching reliability during Windows dogfood.

- Added hash-guarded cleanup of obsolete managed targets after profile switches so old profile skills and app config do not leak into the newly active profile.
- Render targets are regenerated instead of captured during switches, preventing generated Codex/Pi/Claude config drift from blocking profile changes.

## v0.1.2 — 2026-05-09

Patch release for tag-based npm publishing.

- Fixed release workflow tarball paths so `npm publish` treats generated `.tgz` files as local files.

## v0.1.1 — 2026-05-09

Patch release for npm registry distribution.

- Added npm registry publishing to the tag-based release workflow.
- Added a manual npm publish workflow for republishing or repairing package access.
- Updated install and release documentation to prefer `npm install -g @asudbring/loki-profile-manager`.
- Fixed npm access repair command in the manual publish workflow.

## v0.1.0 — 2026-05-09

First public 0.x release. Validated with public GitHub Actions CI/release workflows and dogfood installs on macOS and Windows.

- Prepared the repository for public release with MIT licensing, a public security policy, sanitized current docs, and a module path aligned to `github.com/asudbring/loki-profile-manager`.
- Added public release documentation, an AI-operator release procedure, and a local release fallback for periods when GitHub Actions is unavailable.
- Added Windows PowerShell and macOS/Linux shell install/uninstall scripts, release manifests, installer-aware packaging, CI installer smoke jobs, and release workflow smoke-before-upload gates.
- Added no-args TUI launch: `loki` opens the terminal UI, while commands/flags keep CLI behavior.
- Added persistent store management with `loki store status|discover|use|init|unset`, plus TUI Store setup and TUI machine registration flows.
- Added `loki tui` Bubble Tea MVP with dashboard, doctor/machine/secrets/profile views, guarded switch flow, guarded sync conflict cleanup, and snapshot list/show/restore dry-run handoff.
- Added `loki sync --dry-run|--yes` MVP for provider conflict-copy detection and deletion with current-machine-wins semantics; TUI/app execution guards now use stable conflict fingerprints and dry-run writes nothing.
- Added `loki import-skill <source>` folder/zip-import MVP for validated skill folders and safe `.zip` archives into common, profile core, or profile bucket store layers.
- Added a local no-GitHub-Actions dogfood release script and manual release guide.
- Added `loki secrets login|status|check` Infisical V1 UX for render-template readiness without storing or printing secret values.
- Added Infisical machine identity support via `INFISICAL_TOKEN` or Universal Auth environment variables with explicit `--projectId` routing for machine-auth secret reads.
- Fixed Infisical secret checks to treat existing empty-valued secrets as available, keep non-missing read failures redacted, and retry stale `INFISICAL_TOKEN` values with Universal Auth when configured.
- Added `INFISICAL_HOST_URL` as a legacy host alias for Infisical machine identity auth.
- Added automatic local loading of Infisical machine identity variables from `~/.config/infisical/.env`.
- Fixed skill validation for Markdown references with optional titles, UTF-8 BOM-prefixed `SKILL.md` files, and root-relative web paths; missing or escaping relative reference assets now warn instead of blocking migration.
- Documented dogfood install troubleshooting for GitHub release npm tarballs, Windows npm global PATH, and transient OneDrive locks on `registry\machines.json`.
- Added pre-switch local-change capture for copied managed targets with `loki switch --capture-local`; render and merge drift are reported but not auto-captured.

## v0.1.0-doctor.1 — 2026-05-04

First packaged dogfood prerelease.

- Added `loki doctor [--json]` for read-only environment, store, machine, snapshot, lock, SQLite, conflict-copy, and Infisical CLI diagnostics.
- Added release packaging workflow for Windows/macOS/Linux amd64/arm64 archives with checksum generation and version injection.

## full-restore-consent-dogfood — 2026-05-04

Validation milestone for the CLI-native full snapshot restore consent gate.

- Dogfooded full snapshot restore consent on Windows ARM64 using disposable `dogfood-crossos` target only.
- Verified wrong consent is blocked before restore execution.
- Verified exact `RESTORE <snapshot-id>` consent executes full restore after matching dry-run guard.
- Verified target hash before/after stayed unchanged for `%USERPROFILE%\loki-dogfood\probe.txt`.
- Did not run full restore against real dotfiles.
- Commit: `b7b3910 docs: record full restore consent dogfood`.

## full-restore-consent-gate — 2026-05-04

Safety milestone for full snapshot restore UX.

- Added CLI-native confirmation gate for `loki snapshots restore <snapshot-id> --yes` without `--target`.
- Full restore now requires typing exact `RESTORE <snapshot-id>` before service execution.
- Kept targeted restore behavior unchanged: `--target <path>` still requires a scoped dry-run guard but no full active-state prompt.
- Added tests for accepted full restore consent, rejected wrong consent, and targeted restore prompt bypass.
- Updated `README.md` and `docs/USAGE.md`.
- Commit: `34fde0d feat: gate full snapshot restore consent`.

## real-dotfile-targeted-restore-dogfood — 2026-05-04

Validation milestone for guarded targeted restore on real low-risk dotfiles.

- Dogfooded consent-gated targeted restore on Windows ARM64 for `%USERPROFILE%\.config\git\ignore`.
- Dogfooded consent-gated targeted restore on Windows ARM64 for `%USERPROFILE%\.gitconfig`.
- Used `snapshots restore <snapshot-id> --dry-run --target <path>` before each matching `--yes --target <path>`.
- Verified exact user consent phrase before each real-dotfile targeted restore.
- Verified before/after hashes matched for both targets.
- Did not run full restore against real dotfiles.
- Commit: `4003493 fix: serialize machine id creation`.

## targeted-snapshot-restore-dogfood — 2026-05-04

Validation milestone for targeted snapshot restore.

- Added `loki snapshots restore <snapshot-id> --target <path>` for exact one-target restore.
- Scoped restore guards by snapshot and target filter.
- Kept targeted restore limited to the selected target and selected managed-target row.
- Avoided restoring global active profile/buckets for targeted restores.
- Dogfooded on macOS and Windows ARM64 with disposable `dogfood-crossos` target.
- Commit: `d53003f test: fix symlink adoption paths on Windows`.

## Earlier restore milestones — 2026-05-04

- `9084b8f feat: support targeted snapshot restore` — implemented targeted restore.
- `bee551f feat: add guarded snapshot restore` — implemented `--yes` restore after matching dry-run guard, pre-restore snapshots, DB restore transactionality, and rollback fixes.
- `da26862 feat: add snapshot restore dry-run` — implemented read-only restore preview and guard recording.
- `f76829f feat: add snapshot reporting CLI` — implemented `loki snapshots list` and `loki snapshots show`.
