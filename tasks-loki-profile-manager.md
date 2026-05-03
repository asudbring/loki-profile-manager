# Tasks: Loki Profile Manager

**Status:** Phase 4 complete; Phase 4.5 migration/adoption next  
**Owner:** Allen Su  
**Last updated:** 2026-05-03

## Task rules

- Implement phases in order.
- Do not write to real home directories in tests. Use injectable paths and temp dirs.
- Every task must end with tests or a concrete validation command.
- Keep CLI/core behavior working before TUI polish.
- No secrets in code, logs, fixtures, or docs.

## Phase 1 — Foundations

### 1.1 Initialize Go project

**Work:**
- Create `go.mod`.
- Add `cmd/loki/main.go`.
- Add internal package skeletons for CLI, app service, config, logging.
- Choose CLI framework, preferably Cobra or a small standard-library parser if simpler.

**Acceptance:**
- `go test ./...` passes.
- `go run ./cmd/loki --help` prints help.

### 1.2 Implement app path resolution

**Work:**
- Add OS-specific local state path resolver.
- Support injected home/appdata paths for tests.
- Add store path override support.

**Acceptance:**
- Unit tests cover Windows-style and macOS-style paths.
- No tests depend on the real home directory.

### 1.3 Implement structured logging and redaction

**Work:**
- Add local log writer under local app state.
- Add `--verbose` terminal logging path.
- Add redaction helper for known secret fields and arbitrary registered secret values.

**Acceptance:**
- Unit tests prove secret values are redacted.
- Logs can be written to a temp local state dir.

### 1.4 Implement SQLite local state bootstrap

**Work:**
- Add SQLite dependency.
- Create migration mechanism.
- Add tables for key-value state, managed targets, snapshots, pending captures, and command history.

**Acceptance:**
- Fresh DB migrates successfully.
- Re-running migrations is idempotent.
- DB can be deleted and recreated without needing store manifests.

### 1.5 Add root CLI and status placeholder

**Work:**
- Wire `loki status`.
- If not configured, print clear not-configured output.
- Add `--json` output support for status placeholder.

**Acceptance:**
- `loki status` exits 0 with not-configured state.
- `loki status --json` emits valid JSON.

## Phase 2 — Store and machine registry

### 2.1 Implement provider folder discovery

**Work:**
- Detect common OneDrive and Dropbox folders on Windows/macOS.
- Return candidates; do not auto-select without user confirmation.
- Support manual override path.

**Acceptance:**
- Unit tests cover environment/path-based detection.
- If no candidates, setup can proceed with manual path.

### 2.2 Implement store layout creation/validation

**Work:**
- Create `loki/` layout.
- Validate required directories.
- Add base manifests for `common`, `work`, `dev`, `writer` if creating new store.

**Acceptance:**
- Temp empty folder becomes valid store.
- Existing valid store is preserved.
- Invalid store reports missing paths.

### 2.3 Implement machine ID creation

**Work:**
- Generate UUID.
- Store in local app state.
- Reuse existing ID on subsequent runs.

**Acceptance:**
- First run creates ID.
- Second run returns same ID.
- Tests use temp local state.

### 2.4 Implement machine registry JSON

**Work:**
- Read/write `registry/machines.json`.
- Read/write `registry/machines/<machine_id>.json` heartbeat.
- Include required fields.

**Acceptance:**
- Register machine writes both files.
- Heartbeat updates `last_seen`, active profile/buckets, version.

### 2.5 Implement machine policy enforcement

**Work:**
- Validate requested parent profile and buckets against current machine allowed lists.
- Add policy errors with remediation text.

**Acceptance:**
- Allowed switch passes validation.
- Disallowed profile fails.
- Disallowed bucket fails.

### 2.6 Implement machine deletion

**Work:**
- Delete registry entry.
- Delete machine heartbeat/state file.
- Expose through service; CLI/TUI command can be added later if needed.

**Acceptance:**
- Deleting machine removes both registry record and file.
- Deleting unknown machine returns clear error.

## Phase 3 — Manifest parser and verifier

### 3.1 Define manifest structs and YAML parser

**Work:**
- Add typed manifest structs.
- Parse `version`, `name`, `files`, `skills`, `ignore`, `merge_rules`, `targets`.
- Reject unsupported versions.

**Acceptance:**
- Valid fixture parses.
- Malformed YAML fails with file path.
- Unsupported version fails.

### 3.2 Implement path expansion

**Work:**
- Expand `~`, environment variables, and manifest target variables.
- Support OS-specific target values.

**Acceptance:**
- Tests cover Windows and macOS expansion.
- Unknown variables fail validation.

### 3.3 Implement profile layer resolver

**Work:**
- Resolve common + parent core + ordered buckets.
- Validate bucket belongs to parent.
- Load manifests in order.

**Acceptance:**
- Resolver returns expected layer order.
- Unknown parent/bucket fails.

### 3.4 Implement manifest source validation

**Work:**
- Check source files/directories exist.
- Validate modes and formats.
- Validate ignore patterns compile.

**Acceptance:**
- Missing source fails with path.
- Unknown mode/format fails.
- Bad ignore pattern fails.

### 3.5 Implement skill validation core

**Work:**
- Validate `SKILL.md` existence.
- Parse frontmatter for `name` and `description`.
- Detect broken relative references in `SKILL.md`.

**Acceptance:**
- Valid skill fixture passes.
- Missing `SKILL.md` fails.
- Missing frontmatter fails.
- Broken relative reference fails.

### 3.6 Implement mergeability dry-run

**Work:**
- Group operations by target.
- Detect structured merge candidates.
- Detect unsupported duplicate targets.
- Detect parse/type conflicts without writing output.

**Acceptance:**
- JSON/YAML/TOML compatible fixtures pass.
- Type conflict fixtures fail.
- Unsupported duplicate text target fails unless identical.

### 3.7 Implement `loki verify`

**Work:**
- Wire store, machine, manifest, skill, target, mergeability checks.
- Add JSON and human output.

**Acceptance:**
- Valid test store passes.
- Invalid test store returns nonzero and grouped issues.
- `--json` emits valid structured report.

## Phase 4 — Activation engine

### 4.1 Implement activation plan data model

**Work:**
- Define operations for symlink, copy, merge, render, mirror.
- Include target path, source path, layer, expected hash, safety classification.

**Acceptance:**
- Plan can be generated from fixture store.
- Plan serializes for dry-run output.

### 4.2 Implement unsafe overwrite detector

**Work:**
- Classify missing, Loki-managed symlink, Loki-managed file hash, unmanaged file, unmanaged directory, broken symlink.
- Use SQLite managed target records.

**Acceptance:**
- Missing target safe.
- Managed hash match safe.
- Unmanaged file blocks.
- Broken symlink behavior tested.

### 4.3 Implement symlink operation

**Work:**
- Create/update symlink safely.
- Handle Windows permission errors clearly.
- Do not fallback to copy unless manifest allows future fallback.

**Acceptance:**
- Symlink created in supported test environment.
- Permission/unsupported error returns remediation.

### 4.4 Implement copy operation

**Work:**
- Copy files/directories.
- Use temp files and atomic rename where possible.
- Record hashes in SQLite.

**Acceptance:**
- File and directory copy tests pass.
- Hash records update.

### 4.5 Implement JSON/YAML/TOML merge writers

**Work:**
- Deep merge JSON objects.
- Deep merge YAML mappings.
- Deep merge TOML tables.
- Later layer wins.

**Acceptance:**
- Merge fixtures produce exact expected output.
- Array replacement behavior documented and tested.
- Type conflict fails before write.

### 4.6 Implement Infisical CLI wrapper and template render

**Work:**
- Detect Infisical CLI.
- Retrieve required values without logging values.
- Render templates.
- Missing secrets fail with names only.

**Acceptance:**
- CLI missing path tested.
- Renderer redacts secret values in logs.
- Missing secret names shown; values never shown.

### 4.7 Implement snapshots

**Work:**
- Snapshot affected managed targets before activation.
- Store metadata.
- Keep last two snapshots.

**Acceptance:**
- Snapshot created before writes.
- Retention deletes older snapshots.
- Metadata includes previous active profile/buckets.

### 4.8 Implement rollback

**Work:**
- Restore files from snapshot on activation failure.
- Restore previous active state in SQLite/heartbeat.
- Preserve failed snapshot for inspection.

**Acceptance:**
- Simulated mid-activation failure restores previous files.
- Rollback failure logs emergency recovery instructions.

### 4.9 Implement `loki switch`

**Work:**
- Wire policy validation, planning, dry-run, snapshots, execute, rollback, heartbeat update.

**Acceptance:**
- Valid switch modifies temp home as expected.
- `--dry-run` writes nothing.
- Unsafe overwrite blocks.
- Failure rolls back.

## Phase 4.5 — Migration and adoption bootstrap

### 4.5.1 Implement migration plan data model

**Work:**
- Define migration sources: legacy repo, local home, explicit target adoption.
- Define planned imports with source path, store destination, target path, mode, format, profile, bucket, manifest file, and collision status.
- Support dry-run JSON output.

**Acceptance:**
- Migration plan can be generated from fixture legacy repo.
- Plan serializes without absolute real-home paths in tests.
- No files are modified during planning.

### 4.5.2 Implement legacy repo migration

**Work:**
- Add `loki migrate repo <path> --profile <profile> [--bucket <bucket>] [--dry-run] [--yes]`.
- Scan a user-selected existing dotfiles/settings repo.
- Copy selected files into the Loki store profile layer.
- Generate or append manifest entries.
- Preserve relative folder structure under `files/`.
- Detect common config formats and default modes:
  - dotfiles and stable config: `symlink`
  - app-mutated settings: `copy`
  - JSON/YAML/TOML settings with duplicate targets: `merge`
  - templates containing secret placeholders: `render`

**Acceptance:**
- Fixture repo imports into `profiles/<profile>/core`.
- Bucket import writes into `profiles/<profile>/buckets/<bucket>`.
- Generated manifest passes `loki verify`.
- Subsequent `loki switch --dry-run` plans without unmanaged overwrite errors in temp home.

### 4.5.3 Implement local machine migration

**Work:**
- Add `loki migrate local --profile <profile> [--bucket <bucket>] [--dry-run] [--yes]`.
- Scan injected/current home for known paths:
  - `~/.pi/agent/skills`
  - `~/.agents/skills`
  - `~/.claude/skills`
  - VS Code `settings.json` and `mcp.json`
  - Copilot instructions/prompts under `.github/`
  - common dotfiles such as `.gitconfig`, shell rc files, PowerShell profile.
- Build a reviewable plan before writes.
- Never read or print secret values.

**Acceptance:**
- Temp-home fixtures produce expected migration plan.
- Accepted migration copies files into store and writes manifests.
- Generated store can be switched against same temp home without unsafe overwrite block after adoption records are written.

### 4.5.4 Implement target adoption

**Work:**
- Add `loki adopt <target> --profile <profile> [--bucket <bucket>] [--mode copy|symlink|merge|render] [--source-name <name>] [--dry-run] [--yes]`.
- Copy an existing unmanaged local target into the store.
- Add or update manifest entry.
- Record the current target as managed in SQLite with content hash.
- Resolve the Phase 4 unsafe-overwrite dead end for existing local files.

**Acceptance:**
- Existing unmanaged file becomes Loki-managed without data loss.
- Subsequent `loki switch` may replace it only if hash still matches adopted state.
- Changed adopted file blocks switch until user captures or re-adopts.

### 4.5.5 Implement skill migration

**Work:**
- Detect skill folders by `SKILL.md`.
- Preserve folder names and relative references.
- Import into store skill layer and add `skills:` manifest entries.
- De-duplicate identical skills across Pi, generic agents, and Claude folders.

**Acceptance:**
- Temp skill folders import and validate with existing skill validator.
- Duplicate identical skill imports only once.
- Conflicting skill names are reported before write.

### 4.5.6 Add cross-platform migration tests

**Work:**
- Add fixtures for macOS-like and Windows-like homes.
- Test path expansion, manifest generation, adoption, and switch after migration.
- Keep tests temp-dir only.

**Acceptance:**
- `go test ./...` passes on Docker/Linux, macOS, and Windows VM.
- Native macOS and Windows smoke tests can start from migrated fixture data, not an empty store.

## Phase 5 — Skill import and mirroring

### 5.1 Implement skill folder import

**Work:**
- Validate folder skill.
- Ask/accept explicit target.
- Copy entire folder to destination.

**Acceptance:**
- Valid folder imports.
- Invalid folder rejects with no partial copy.

### 5.2 Implement zip import

**Work:**
- Safely extract zip to temp dir.
- Reject zip-slip paths.
- Locate skill folder and validate.

**Acceptance:**
- Valid zip imports.
- Zip-slip fixture rejects.
- Invalid zip leaves no partial destination.

### 5.3 Implement markdown-to-skill import

**Work:**
- Convert single markdown file to folder with `SKILL.md`.
- Require or infer `name` and `description` frontmatter.
- Reject if metadata cannot be built.

**Acceptance:**
- Markdown with frontmatter imports.
- Markdown without enough metadata rejects with advice.

### 5.4 Implement explicit target selector abstraction

**Work:**
- Prompt interface for CLI/TUI/tests.
- Target choices: common, parent core, bucket.
- No silent active-target default.

**Acceptance:**
- CLI prompts when target omitted.
- Tests inject target without terminal.

### 5.5 Implement duplicate overwrite prompt

**Work:**
- Detect same skill name in target.
- Ask before overwrite unless `--overwrite` provided.

**Acceptance:**
- Duplicate without confirmation aborts.
- Duplicate with confirmation overwrites.
- Duplicate with `--overwrite` overwrites.

### 5.6 Implement tool target adapters

**Work:**
- Pi adapter: `~/.pi/agent/skills`.
- Generic agents adapter: `~/.agents/skills`.
- Claude global adapter: `~/.claude/skills`.
- Claude repo adapter: `.claude/skills`.
- Copilot adapter for configured instruction/prompt paths.
- VS Code adapter for `settings.json` and `mcp.json`.

**Acceptance:**
- Each adapter writes to injected temp paths in tests.
- Missing target roots are created only where safe.
- Adapter errors include target name/path.

### 5.7 Wire `loki import-skill`

**Work:**
- Support folder, zip, markdown.
- Validate, target-select, duplicate-handle, copy, mirror if appropriate.

**Acceptance:**
- All three source types tested through CLI command.
- Invalid source returns nonzero and leaves no partial destination.

## Phase 6 — Sync, conflicts, locking, watcher

### 6.1 Implement local command lock

**Work:**
- Lock file in local app state.
- Acquire for state-changing commands.
- Second command exits with operation-in-progress.
- Doctor can report stale locks.

**Acceptance:**
- Concurrent test shows second command blocked.
- Lock released after command.

### 6.2 Implement sync status

**Work:**
- Determine store exists/writable.
- Track pending/offline status.
- Update heartbeat.

**Acceptance:**
- Writable store reports available.
- Missing/unwritable store reports offline/error.

### 6.3 Implement conflict-copy detection

**Work:**
- Search store for OneDrive/Dropbox conflict-copy patterns.
- Return list with paths and probable originals.

**Acceptance:**
- Pattern fixtures detected.
- Normal files ignored.

### 6.4 Implement current-machine-wins conflict resolution

**Work:**
- During sync, delete conflict copies.
- Prefer current machine manifest/state.
- Log decision.

**Acceptance:**
- Conflict-copy file deleted in test.
- Log entry includes path and current-machine-wins decision.
- Losing content is not copied to extra Loki backup.

### 6.5 Wire `loki sync`

**Work:**
- Acquire lock.
- Resolve conflict copies.
- Write heartbeat.
- Report status.
- Support dry-run.

**Acceptance:**
- Dry-run deletes nothing.
- Normal run deletes conflict copies and updates heartbeat.

### 6.6 Implement watcher foundation

**Work:**
- Add file watcher library.
- Watch copied targets marked `capture: true`.
- Support start/stop from TUI or command service.

**Acceptance:**
- Watcher sees file changes in temp dir.
- Non-capture targets are not watched.

### 6.7 Implement debounce and hash guard

**Work:**
- Debounce changes for about 2 seconds.
- Hash before capture.
- Ignore unchanged files.

**Acceptance:**
- Rapid writes produce one capture event.
- Unchanged hash ignored.

### 6.8 Implement capture write-back

**Work:**
- Write changed copied config back to owning active layer when unambiguous.
- Apply ignore rules.
- Record pending change when ambiguous.

**Acceptance:**
- Simple copied file writes back to source.
- Ignored cache path ignored.
- Ambiguous merged target creates pending change.

## Phase 7 — TUI

### 7.1 Create Bubble Tea app shell

**Work:**
- Add TUI model, views, keybindings, styles.
- Wire `loki tui` and default no-arg launch.

**Acceptance:**
- TUI starts without panic.
- First paint displays not-configured or configured dashboard.

### 7.2 Implement dashboard view

**Work:**
- Show active profile/buckets, machine, store path, sync status, pending changes, skill counts, recent errors/conflicts, quick actions.

**Acceptance:**
- Model test renders all required fields from fake status.
- Manual first paint under 1 second on normal store.

### 7.3 Implement switcher view

**Work:**
- Choose allowed parent profile.
- Choose allowed buckets.
- Show dry-run plan.
- Confirm and execute switch.

**Acceptance:**
- Disallowed profile hidden or blocked.
- Dry-run summary shown before execution.
- Success and rollback errors render clearly.

### 7.4 Implement import skill view

**Work:**
- Enter path.
- Validate.
- Choose explicit target.
- Confirm duplicate overwrite.
- Import.

**Acceptance:**
- Valid import succeeds in model/integration path.
- Invalid import shows advice.
- Duplicate asks before overwrite.

### 7.5 Implement verify/doctor views

**Work:**
- Run checks.
- Group blocking/warning/info issues.
- Show remediation text.

**Acceptance:**
- Fake report renders grouped issues.
- User can return to dashboard.

### 7.6 Implement sync action view

**Work:**
- Show dry-run conflict summary.
- Execute sync.
- Show deleted conflict-copy files and heartbeat update.

**Acceptance:**
- Conflict summary visible.
- Sync errors render without crashing TUI.

## Phase 8 — Packaging, docs, migration

### 8.1 Add build scripts

**Work:**
- Add cross-platform `go build` commands or `justfile`.
- Build Windows and macOS binaries.

**Acceptance:**
- Clean clone builds binaries.
- Build output names include OS/arch.

### 8.2 Add CI basics

**Work:**
- Add GitHub Actions or equivalent for `go test ./...` and builds.

**Acceptance:**
- CI passes on PR/push.

### 8.3 Write README

**Work:**
- Explain purpose, install, first setup, commands, safety model, sync model.

**Acceptance:**
- New implementer can run setup from README.

### 8.4 Write manifest schema docs

**Work:**
- Document YAML schema, modes, merge rules, variables, examples.

**Acceptance:**
- Example manifests pass `loki verify`.

### 8.5 Write tool target docs

**Work:**
- Document Pi, generic agents, Claude, Copilot, and VS Code destinations.
- Explain path overrides.

**Acceptance:**
- Docs list every V1 target adapter.

### 8.6 Write migration guide from `.dotfiles-work`

**Work:**
- Explain mapping from `common`, parent profile, buckets, symlinks/scripts into Loki store.
- Mention current active example `work:content-dev,azure`.

**Acceptance:**
- Guide includes step-by-step migration checklist.

### 8.7 Final validation

**Work:**
- Run full tests.
- Run manual temp-store smoke test.
- Run TUI smoke test.

**Acceptance:**
- `go test ./...` passes.
- `loki verify` passes against example store.
- `loki switch` works against temp home.
- `loki tui` opens and shows dashboard.

## Deferred tasks / not V1

- Direct OneDrive/Dropbox APIs.
- Homebrew/winget installers.
- Team sharing.
- Linux support.
- Package/app installation.
- Background daemon service.
- Rich three-way conflict merge UI.
- Plugin SDK for target adapters.
