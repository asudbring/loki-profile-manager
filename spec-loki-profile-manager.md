<!-- historical-doc -->

> **Historical note:** Historical product specification. It records design intent and may not match every current command or implementation detail.

# Spec: Loki Profile Manager

**Status:** Ready for implementation  
**Owner:** asudbring  
**Last updated:** 2026-05-03

## 1. Summary

Loki Profile Manager is a local, cross-platform TUI/CLI application for one technical user who works across multiple machines, roles, AI tooling stacks, and dotfile/profile configurations. It replaces fragile symlink scripts, split dotfile repositories, manual app config copying, and ad hoc AI skill mirroring with one unified local store backed by a user-selected OneDrive or Dropbox synced folder. Loki manages profile activation, machine-aware profile policy, skill import and mirroring, secret template rendering, conflict resolution, snapshots, and verification while leaving actual cloud authentication and file replication to the provider's desktop sync client.

## 2. Problem & goals

**Problem statement.** The current dotfile/profile workflow is spread across separate repos such as `.dotfiles-work` and `.dotfiles-play`, shell/PowerShell scripts, symlinks, manual profile selection, app settings that mutate managed files, handoff files that dirty repos, and manually copied AI skills. This makes switching contexts risky and slow, makes machines drift, and makes it hard to know which config is active or safe to overwrite. The system needs one local source of truth that can follow machines through a normal cloud sync folder without storing cloud API tokens.

**Goals** (prioritized):

1. Provide one unified local profile store that supports parent profiles (`work`, `dev`, `writer`) plus additive buckets and a common layer.
2. Safely activate profiles on Windows and macOS using symlinks, copies, structured merges, and secret template rendering according to tool definitions.
3. Track machine identity, allowed profiles/buckets, current active state, and heartbeat files so each workstation can be managed without accidental cross-role activation.
4. Import, validate, organize, and mirror AI skills/prompts/instructions across Pi, generic agent skill folders, Claude, Copilot, and VS Code targets.
5. Sync through a local OneDrive or Dropbox folder named `loki`, using manifests, hashes, and machine heartbeats while provider clients handle actual cloud transfer.
6. Detect and remediate risky states: unmanaged target overwrite, merge failure, provider conflict copies, missing secrets, dirty captured configs, stale machine registry entries.
7. Provide both a fast CLI and usable Bubble Tea TUI with dashboard, switcher, sync status, skill management, verification, doctor output, and quick actions.

**Non-goals**:

- No direct Microsoft Graph, Dropbox API, or cloud API integration in V1. Loki does not store provider OAuth tokens.
- No team sharing or multi-user permissions model in V1. The store is for one user across their own machines.
- No package manager or dependency installer in V1. Loki manages config and skills, not app installation.
- No hosted service, web app, daemon cloud backend, or remote control plane in V1.
- No automatic preservation of the losing cloud conflict version as an additional Loki backup. Current machine wins by policy.
- No Linux support requirement in V1, although design should not deliberately block later Linux support.

**Success metrics**:

- A fresh machine can register, select allowed profiles, discover the synced `loki` folder, and activate a profile without manually editing scripts.
- `loki status` returns in under 1 second on a normal store.
- TUI first paint occurs in under 1 second on a normal store.
- `loki switch` completes in under 10 seconds for 500 managed files and 200 skills.
- Loki refuses to overwrite unmanaged local files and presents remediation choices.
- Loki can import a folder, zip archive, or single markdown file as a skill and mirror it to all configured V1 tool targets.
- Loki can render Infisical-backed templates without printing secret values.
- Loki can roll back a failed activation using a snapshot.
- OneDrive/Dropbox conflict copies are detected and resolved in favor of the current machine.

## 3. Users & stories

**Primary user.** An advanced technical user working across Windows and macOS machines with separate work, development, and writing contexts. He uses many local tools, AI coding agents, profile-specific dotfiles, Microsoft documentation workflows, and Infisical-managed secrets. He can edit YAML and use a CLI, but the day-to-day workflow should be driven by a TUI/dashboard instead of manual symlinks and scripts.

**Key user stories:**

- **US-1:** As Allen, I want to switch between parent profiles and buckets, so that my machine reflects the role I am currently doing without manual repo/script work.
- **US-2:** As Allen, I want Loki to know which profiles are allowed on each machine, so that a work-only or personal machine cannot accidentally activate the wrong context.
- **US-3:** As Allen, I want Loki to import and mirror AI skills into multiple agent/tool folders, so that skills remain consistent across Pi, Claude, Copilot, VS Code, and generic agent environments.
- **US-4:** As Allen, I want fragile app configs copied and watched instead of symlinked, so that apps can mutate their settings without corrupting the source store or dirtying a repo.
- **US-5:** As Allen, I want templates rendered with Infisical secrets at activation time, so that real secrets never live in the synced store.
- **US-6:** As Allen, I want status, verify, doctor, and logs to explain what Loki will do or did, so that profile changes are auditable and recoverable.
- **US-7:** As Allen, I want the cloud backend to be only a local OneDrive/Dropbox folder, so that Loki avoids provider API tokens while still syncing across machines.

## 4. Functional requirements

### FR-1: Store discovery and initialization

**Statement:** The system SHALL create or discover a local synced store named `loki` inside a user-confirmed OneDrive or Dropbox folder.

**Acceptance criteria:**

- GIVEN OneDrive or Dropbox sync folders are detectable, WHEN the user runs first-time setup, THEN Loki lists candidate folders and requires confirmation or override before creating/using `loki/`.
- GIVEN no provider folder is detected, WHEN the user runs first-time setup, THEN Loki prompts for a local path and records it as the store root.
- GIVEN a selected folder does not contain `loki/`, WHEN setup completes, THEN Loki creates the store layout and base manifests.
- GIVEN a selected folder contains a valid `loki/` store, WHEN setup completes, THEN Loki records the existing store and does not overwrite existing manifests.

### FR-2: Machine registration and policy

**Statement:** The system SHALL register each workstation with a stable machine record and enforce allowed parent profiles and buckets for that machine.

**Acceptance criteria:**

- GIVEN an unregistered machine, WHEN Loki starts setup, THEN it creates a machine ID and asks for a display name plus allowed parent profiles and buckets.
- GIVEN a registered machine, WHEN Loki writes heartbeat state, THEN it includes `machine_id`, `display_name`, `os`, `hostname`, `allowed_parent_profiles`, `allowed_buckets`, `last_seen`, `active_profile`, `active_buckets`, and `loki_version`.
- GIVEN a user tries to switch to a profile or bucket not allowed on the current machine, WHEN `loki switch` runs, THEN Loki refuses and explains the machine policy violation.
- GIVEN the user deletes a machine from the registry, WHEN deletion is confirmed, THEN Loki removes the registry entry and associated heartbeat/state files.

### FR-3: Profile and bucket model

**Statement:** The system SHALL support exactly one active parent profile plus zero or more additive buckets from that parent, with a common layer always available.

**Acceptance criteria:**

- GIVEN parent profiles `work`, `dev`, and `writer`, WHEN a user switches, THEN Loki activates exactly one parent profile.
- GIVEN buckets under the selected parent profile, WHEN a user switches with buckets, THEN Loki activates only buckets that belong to the selected parent.
- GIVEN common resources exist, WHEN any parent profile is active, THEN common resources are included according to manifests.
- GIVEN duplicate managed targets across common, parent core, and buckets, WHEN activation resolves layers, THEN common applies first, parent core second, and buckets last.

### FR-4: Manifest-driven activation

**Statement:** The system SHALL activate files using YAML manifests that define target paths, activation mode, merge behavior, capture behavior, ignore rules, and template requirements.

**Acceptance criteria:**

- GIVEN a manifest file, WHEN Loki parses it, THEN invalid YAML or unknown required fields cause validation failure before activation.
- GIVEN a file target marked `symlink`, WHEN activation runs, THEN Loki creates or updates a symlink to the store file where supported and safe.
- GIVEN a file target marked `copy`, WHEN activation runs, THEN Loki copies the file to the target path and records a hash.
- GIVEN a template target marked `render`, WHEN activation runs, THEN Loki renders the template to the target path using Infisical-supplied secret values.
- GIVEN a target path matches an ignore rule, WHEN activation runs, THEN Loki does not manage that path.

### FR-5: Structured merge

**Statement:** The system SHALL merge duplicate active JSON, YAML, and TOML targets where possible, with later layers overriding earlier keys.

**Acceptance criteria:**

- GIVEN multiple active layers target the same JSON file, WHEN activation runs, THEN Loki deep-merges objects and uses bucket values over parent/core values on key conflicts.
- GIVEN multiple active layers target the same YAML file, WHEN activation runs, THEN Loki deep-merges mappings and uses bucket values over parent/core values on key conflicts.
- GIVEN multiple active layers target the same TOML file, WHEN activation runs, THEN Loki merges tables and uses bucket values over parent/core values on key conflicts.
- GIVEN duplicate targets cannot be safely merged because of type conflicts, parse errors, or unsupported file formats, WHEN activation runs, THEN Loki stops and reports the exact files and layers involved.

### FR-6: Unsafe overwrite protection

**Statement:** The system SHALL stop before overwriting real unmanaged files and present remediation guidance.

**Acceptance criteria:**

- GIVEN a target path exists and is not recognized as managed by Loki, WHEN activation would replace it, THEN Loki stops before modifying the file.
- GIVEN Loki stops for an unmanaged file, WHEN it reports the issue, THEN it offers concrete remediation choices such as adopt, backup and replace, skip, or cancel.
- GIVEN a target path is already a Loki-managed symlink or copied file with matching recorded hash, WHEN activation runs, THEN Loki can update it without prompting.

### FR-7: Snapshots and rollback

**Statement:** The system SHALL create activation snapshots and roll back failed switches.

**Acceptance criteria:**

- GIVEN a switch operation will modify local files, WHEN activation starts, THEN Loki creates a local snapshot of affected managed targets.
- GIVEN activation fails halfway, WHEN rollback is possible, THEN Loki restores the previous active state from the snapshot and reports the failure.
- GIVEN more than two snapshots exist for a machine, WHEN snapshot cleanup runs, THEN Loki keeps only the last two snapshots.

### FR-8: Watcher capture for fragile app configs

**Statement:** The system SHALL watch copied targets marked `capture: true` and write user/app edits back into the active store after debounce and hash checks.

**Acceptance criteria:**

- GIVEN a copied target is marked `capture: true`, WHEN the live target file changes, THEN Loki waits approximately 2 seconds before evaluating capture.
- GIVEN the changed file hash differs from the last managed hash, WHEN capture runs, THEN Loki writes the new content back to the owning active layer according to manifest policy.
- GIVEN a watched change occurs in an ignored runtime/cache/session path, WHEN capture evaluates it, THEN Loki ignores the change.
- GIVEN capture cannot determine a safe owning layer, WHEN capture runs, THEN Loki records a pending change and asks the user to choose a destination.

### FR-9: Infisical template rendering

**Statement:** The system SHALL integrate with Infisical through the `infisical` CLI for rendering secret templates without storing or logging secret values.

**Acceptance criteria:**

- GIVEN a template references Infisical secret names, WHEN activation runs, THEN Loki invokes the Infisical CLI and injects values only into the rendered output file.
- GIVEN a required secret is missing, WHEN activation runs, THEN Loki fails activation and lists missing secret names without printing values.
- GIVEN logs or error reports are generated, WHEN they include template context, THEN secret values are redacted.
- GIVEN Infisical CLI is not installed or not authenticated, WHEN `loki verify` or activation runs, THEN Loki reports the missing prerequisite and remediation steps.

### FR-10: Skill import

**Statement:** The system SHALL import skills from folders, zip archives, and single markdown files into a user-selected common, parent core, or bucket target.

**Acceptance criteria:**

- GIVEN a folder containing `SKILL.md`, WHEN the user imports it, THEN Loki validates the skill and copies the entire folder into the selected target.
- GIVEN a zip archive containing a skill folder, WHEN the user imports it, THEN Loki extracts to a temporary location, validates it, and copies it into the selected target.
- GIVEN a single markdown file, WHEN the user imports it, THEN Loki converts it to a skill folder with `SKILL.md` and validates required metadata.
- GIVEN multiple possible targets, WHEN import starts, THEN the UI/CLI asks the user to explicitly select the import target and does not silently default.
- GIVEN the target already has a skill with the same name, WHEN import runs, THEN Loki asks whether to overwrite before modifying the existing skill.

### FR-11: Skill validation

**Statement:** The system SHALL strictly validate imported skills and reject invalid skills with actionable advice.

**Acceptance criteria:**

- GIVEN an imported skill lacks `SKILL.md`, WHEN validation runs, THEN Loki rejects the import and explains the missing file.
- GIVEN `SKILL.md` lacks `name` or `description` frontmatter, WHEN validation runs, THEN Loki rejects the import and names the missing fields.
- GIVEN `SKILL.md` references relative files that are missing, WHEN validation runs, THEN Loki rejects the import and lists broken references.
- GIVEN validation fails, WHEN import ends, THEN no partial skill is left in the destination target.

### FR-12: Skill and prompt mirroring

**Statement:** The system SHALL mirror active skills/prompts/instructions to all configured V1 tool destinations.

**Acceptance criteria:**

- GIVEN active skills exist, WHEN activation or `loki sync` runs, THEN Loki mirrors skills to Pi `~/.pi/agent/skills`.
- GIVEN active skills exist, WHEN activation or `loki sync` runs, THEN Loki mirrors skills to generic agent `~/.agents/skills`.
- GIVEN active skills exist, WHEN activation or `loki sync` runs, THEN Loki mirrors skills to Claude Code global `~/.claude/skills` where available.
- GIVEN repo-local Claude targets are configured, WHEN activation runs from or against a repo, THEN Loki can mirror configured skills to `.claude/skills`.
- GIVEN Copilot targets are configured, WHEN activation runs, THEN Loki manages global Copilot instructions if present, repo `.github/copilot-instructions.md`, `.github/instructions/*.instructions.md`, `.github/prompts/*.prompt.md`, and VS Code Copilot settings.
- GIVEN VS Code targets are configured, WHEN activation runs, THEN Loki manages VS Code `settings.json` and `mcp.json` according to manifests.

### FR-13: Sync and conflict policy

**Statement:** The system SHALL sync local manifests and state through the provider-synced folder and resolve conflicts with current machine wins.

**Acceptance criteria:**

- GIVEN the provider folder is offline but locally writable, WHEN the user changes profiles or imports skills, THEN Loki writes local manifests and shows sync status as pending/offline.
- GIVEN Loki sees OneDrive or Dropbox conflict-copy files, WHEN `loki sync` runs, THEN Loki chooses the current machine version and deletes the provider conflict copy.
- GIVEN another machine modified cloud state, WHEN current machine syncs, THEN current machine wins for conflicts and writes the resulting manifest/state.
- GIVEN sync deletes a conflict copy, WHEN logging runs, THEN Loki records the path and decision without preserving losing content as an additional Loki backup.

### FR-14: CLI commands

**Statement:** The system SHALL provide the V1 CLI commands `tui`, `status`, `switch`, `sync`, `import-skill`, `verify`, and `doctor`.

**Acceptance criteria:**

- GIVEN Loki is installed, WHEN `loki status` runs, THEN it prints active profile, buckets, machine ID/name, store path, sync state, pending changes, and recent errors.
- GIVEN Loki is installed, WHEN `loki switch <profile> [buckets...]` runs, THEN it validates policy and performs activation or exits with a clear error.
- GIVEN Loki is installed, WHEN `loki sync` runs, THEN it reconciles manifests/state according to current-machine-wins policy.
- GIVEN Loki is installed, WHEN `loki import-skill <path>` runs, THEN it validates the source and asks for an explicit target.
- GIVEN Loki is installed, WHEN `loki verify` runs, THEN it validates manifests, targets, secrets prerequisites, mergeability, machine policy, and skill references without changing files.
- GIVEN Loki is installed, WHEN `loki doctor` runs, THEN it reports environment issues, provider folder state, path permissions, conflict copies, stale machines, missing tools, and remediation advice.
- GIVEN Loki is installed, WHEN `loki tui` runs or no subcommand is provided, THEN it launches the Bubble Tea TUI.

### FR-15: TUI dashboard and workflows

**Statement:** The system SHALL provide a Bubble Tea TUI with dashboard, switcher, sync state, skill counts, change summaries, errors, conflicts, and quick actions.

**Acceptance criteria:**

- GIVEN the TUI starts, WHEN first paint renders, THEN it shows active profile/buckets, current machine, store path, sync status, pending local changes, skill counts by target, and recent conflicts/errors.
- GIVEN the user opens the switcher, WHEN they choose a parent profile and buckets, THEN Loki validates allowed profiles/buckets before activation.
- GIVEN the user chooses import skill, WHEN the source is selected/provided, THEN the TUI asks for target and shows validation results.
- GIVEN the user chooses doctor, WHEN checks complete, THEN the TUI shows grouped issues and suggested fixes.

### FR-16: Logging and observability

**Statement:** The system SHALL write local logs and concise heartbeat/error summaries without exposing secrets.

**Acceptance criteria:**

- GIVEN any command runs, WHEN logging is enabled by default, THEN Loki writes structured local logs under the local state directory.
- GIVEN `--verbose` is supplied, WHEN a command runs, THEN Loki prints additional diagnostic detail to the terminal.
- GIVEN secret values appear in command output or template context, WHEN Loki logs, THEN values are redacted.
- GIVEN a heartbeat is written to the shared store, WHEN errors exist, THEN it includes only a summary and no secret values.

## 5. Non-functional requirements

- **Performance:** `loki status` under 1 second, TUI first paint under 1 second, and `loki switch` under 10 seconds for 500 managed files and 200 skills on a typical workstation.
- **Reliability:** Activation is transactional enough to roll back failed switches from snapshots. Last two snapshots are retained per machine.
- **Security:** Secrets are never stored in source manifests or logs. Infisical values are retrieved through the CLI and only written to intended rendered target files.
- **Observability:** Structured local logs, verbose mode, status summaries, and doctor remediation output are required. Cloud-shared observability is limited to heartbeat/error summaries.
- **Compatibility:** V1 supports Windows and macOS. Paths must use OS-specific conventions and avoid Unix-only assumptions.
- **Cost:** Local-only application; no hosted services or API costs required by Loki.
- **Usability:** The TUI is the preferred everyday interface. CLI commands must be scriptable and clear enough for automation.
- **Data ownership:** YAML manifests and profile contents in `loki/` are the portable source of truth. SQLite is local cache/runtime state only.

## 6. Technical design

**Stack & platform:**

- Language: Go.
- TUI: Bubble Tea ecosystem.
- CLI: Go command framework such as Cobra or equivalent.
- Persistence: SQLite for local state/cache only.
- Manifest format: YAML for human-authored store manifests and tool definitions.
- Generated registry/state: JSON for machine registry and heartbeat files.
- Platform: local CLI/TUI binary for Windows and macOS.

**Key components:**

- **CLI command layer:** Parses commands, flags, interactive prompts, and exit codes.
- **TUI layer:** Dashboard and guided workflows for switch, sync, import, verify, and doctor.
- **Store manager:** Discovers/initializes OneDrive/Dropbox-backed `loki/`, reads/writes manifests, validates layout.
- **Machine registry:** Creates machine IDs, manages machine records, enforces allowed profile/bucket policy, writes heartbeats.
- **Profile resolver:** Resolves common + parent core + buckets into an ordered activation plan.
- **Activation engine:** Applies symlink/copy/render operations, structured merges, snapshots, rollback, and overwrite protection.
- **Watcher/capture engine:** Watches copied fragile configs marked `capture: true`, debounces changes, hash-checks, writes back safely.
- **Skill manager:** Imports, validates, stores, and mirrors skills/prompts/instructions to target tool folders.
- **Sync/conflict manager:** Handles provider conflict-copy detection, current-machine-wins reconciliation, pending/offline status.
- **Infisical integration:** Calls Infisical CLI for required secrets and renders templates without logging values.
- **Verifier/doctor:** Performs dry-run validation, environment checks, stale machine checks, permissions checks, and remediation reporting.
- **Logger:** Structured local logs, redaction, verbose terminal output.

**Store layout:**

```text
loki/
  registry/
    machines.json
    machines/
      <machine_id>.json
  profiles/
    common/
      files/
      skills/
      templates/
      manifest.yaml
    work/
      core/
        files/
        skills/
        templates/
        manifest.yaml
      buckets/
        content-dev/
          files/
          skills/
          templates/
          manifest.yaml
        azure/
          files/
          skills/
          templates/
          manifest.yaml
    dev/
      core/
      buckets/
    writer/
      core/
      buckets/
  conflicts/
  snapshots/
  logs/
```

Local machine state should live outside the synced source of truth, for example under OS-specific app data:

- Windows: `%LOCALAPPDATA%/loki-profile-manager/`
- macOS: `~/Library/Application Support/loki-profile-manager/`

**Core data model:**

- `MachineRecord`: `machine_id`, `display_name`, `os`, `hostname`, `allowed_parent_profiles`, `allowed_buckets`, `last_seen`, `active_profile`, `active_buckets`, `loki_version`.
- `ProfileLayer`: `kind` (`common`, `parent_core`, `bucket`), `name`, `path`, `manifest`.
- `Manifest`: files, skills, templates, merge rules, ignore rules, capture flags, target paths, tool targets.
- `ActivationPlan`: ordered operations, expected hashes, target paths, snapshot plan, rollback plan.
- `Skill`: name, description, source path, target layer, references, validation result.
- `Snapshot`: machine ID, timestamp, previous active profile/buckets, affected target files, hashes.
- `SyncStatus`: provider type/path, online/offline/pending, conflict copies found/resolved, last sync time.

**Interfaces:**

```text
loki tui
loki status [--json] [--verbose]
loki switch <profile> [buckets...] [--dry-run] [--yes] [--verbose]
loki sync [--dry-run] [--verbose]
loki import-skill <path> [--target <target>] [--overwrite] [--verbose]
loki verify [--json] [--verbose]
loki doctor [--json] [--verbose]
```

**Manifest sketch:**

```yaml
version: 1
name: work-core
files:
  - source: files/git/.gitconfig
    target: "~/.gitconfig"
    mode: symlink
  - source: files/vscode/settings.json
    target: "${vscode_user_dir}/settings.json"
    mode: merge
    format: json
    capture: true
  - source: templates/pi-config.json.tmpl
    target: "~/.pi/config.json"
    mode: render
    secrets:
      - INFISICAL_PROJECT_ID
ignore:
  - "**/Cache/**"
  - "**/logs/**"
skills:
  - source: skills/
    targets:
      - pi
      - agents
      - claude_global
merge_rules:
  json: deep_object_bucket_wins
  yaml: deep_mapping_bucket_wins
  toml: table_bucket_wins
```

## 7. Constraints & assumptions

**Constraints:**

- V1 must run locally on Windows and macOS.
- V1 must use a local provider sync folder and must not call provider cloud APIs.
- V1 must not store or print secret values.
- V1 must treat YAML manifests and profile contents as source of truth; SQLite is disposable cache/runtime state.
- V1 must stop before unsafe unmanaged overwrites.
- V1 must use current-machine-wins conflict policy.

**Assumptions:**

- OneDrive/Dropbox desktop clients provide acceptable eventual sync and conflict-copy behavior.
- The user is comfortable approving profile targets and editing YAML when needed.
- Infisical CLI is available or can be installed/configured by the user.
- Tool destination folders may not always exist; Loki should create manageable folders where safe and report missing tools where not.
- Some app config files are unsafe to symlink; manifests decide symlink/copy/render/capture behavior.

## 8. Edge cases & error handling

| Case | Expected behavior |
|------|-------------------|
| No synced provider folder detected | Prompt for manual store path; continue local-only if user confirms. |
| Provider folder offline | Allow local writes; mark sync pending/offline. |
| Invalid YAML manifest | Stop verification/activation and show file path, line/field if available. |
| Unknown manifest version | Stop and advise supported versions. |
| Unmanaged target exists | Stop before overwrite and offer adopt/backup-and-replace/skip/cancel guidance. |
| Symlink unsupported or permission denied | Fall back only if manifest allows copy fallback; otherwise stop with remediation. |
| Structured merge parse failure | Stop before write; report conflicting layers and target path. |
| Activation fails halfway | Roll back using snapshot; keep logs. |
| More than two snapshots | Delete older snapshots after successful snapshot creation/cleanup. |
| Missing Infisical CLI | Verify fails; activation requiring secrets fails. |
| Missing Infisical secret | Activation fails; list secret names only. |
| Skill lacks `SKILL.md` | Reject import; no partial destination. |
| Broken skill relative reference | Reject import; list broken references. |
| Duplicate skill name | Ask before overwrite. |
| Current machine not registered | Enter setup/registration before state-changing commands. |
| Profile not allowed on machine | Refuse switch; show allowed values. |
| OneDrive/Dropbox conflict copy exists | Auto-select current machine version and delete conflict copy during sync. |
| Concurrent local Loki command | Second command exits with operation-in-progress message. |
| Stale machine heartbeat | Doctor reports stale machine and offers removal. |
| Runtime/cache files changed | Ignore according to manifest ignore patterns. |
| Capture cannot pick owner layer | Record pending change and ask user to choose destination. |

## 9. Implementation plan

### Phase 1 — Foundations

- **Deliverable:** Buildable Go repo with CLI skeleton, config path resolution, logging/redaction, SQLite setup, and store path detection stubs.
- **Prerequisites:** None.
- **Scope:** Project scaffolding, command wiring, core types, path utilities, test harness.
- **Validation:** `go test ./...` passes; `loki status` returns a structured not-configured message.

### Phase 2 — Store and machine registry

- **Deliverable:** First-run setup can create/discover `loki/`, register current machine, write/read machine registry and heartbeat JSON.
- **Prerequisites:** Phase 1.
- **Scope:** Store layout, provider folder detection, machine ID creation, allowed profile/bucket policy, JSON registry.
- **Validation:** Register machine, list machine status, delete machine and associated files in tests/temp dirs.

### Phase 3 — Manifest parser and verifier

- **Deliverable:** YAML manifests parse into typed structs; `loki verify` validates layout, manifests, skills, mergeability, target safety, Infisical prerequisites.
- **Prerequisites:** Phase 2.
- **Scope:** YAML schema, validation errors, profile layer resolver, dry-run activation planning.
- **Validation:** Fixture manifests cover valid/invalid versions, malformed YAML, missing files, invalid targets, unsupported merge cases.

### Phase 4 — Activation engine

- **Deliverable:** `loki switch <profile> [buckets...]` activates symlink/copy/render/merge operations with snapshots and rollback.
- **Prerequisites:** Phase 3.
- **Scope:** Activation plan execution, overwrite protection, structured JSON/YAML/TOML merge, Infisical CLI wrapper, snapshot retention.
- **Validation:** Integration tests run against temp home dirs and assert target files, rollback, and unsafe overwrite behavior.

### Phase 5 — Skill import and mirroring

- **Deliverable:** `loki import-skill` supports folder/zip/markdown, strict validation, duplicate prompts, explicit target selection, and mirroring to configured tool destinations.
- **Prerequisites:** Phase 4.
- **Scope:** Skill parser/validator, archive extraction, markdown conversion, tool destination mapping for Pi, agents, Claude, Copilot, VS Code.
- **Validation:** Fixture imports pass/fail correctly; mirrors are created in temp tool directories; duplicate prompt behavior is tested.

### Phase 6 — Sync, conflicts, and watcher capture

- **Deliverable:** `loki sync` handles pending/offline status, conflict-copy deletion with current-machine-wins policy, local lock, and watcher capture for `capture: true` copied configs.
- **Prerequisites:** Phase 5.
- **Scope:** Provider conflict patterns, lock file, status computation, file watcher, debounce, hash guard, pending change queue.
- **Validation:** Simulated conflict-copy files are deleted; concurrent command lock works; watcher captures changed files after debounce.

### Phase 7 — TUI

- **Deliverable:** `loki tui` provides dashboard, switcher, sync view, import-skill workflow, verify/doctor view, and quick actions.
- **Prerequisites:** Phase 6.
- **Scope:** Bubble Tea models, command adapters, keyboard flows, error panels, status rendering.
- **Validation:** TUI model tests verify navigation and action state; manual run shows first paint under 1 second.

### Phase 8 — Packaging, docs, and migration guide

- **Deliverable:** Windows/macOS binaries, README, example manifests, migration notes from `.dotfiles-work`, and release checklist.
- **Prerequisites:** Phase 7.
- **Scope:** Build scripts, basic CI, documentation, examples, troubleshooting.
- **Validation:** Clean clone can build binaries; docs explain setup/switch/import/doctor; examples pass `loki verify`.

## 10. Open questions

None blocking V1 implementation. If implementation exposes unsupported tool-specific path details, add targeted manifests/adapters without changing the V1 product scope.

## 11. Out of scope / future

- Direct OneDrive/Dropbox API integration.
- Team sharing and multi-user access control.
- Linux support.
- App/package installation and workstation bootstrap package management.
- Homebrew and winget installers.
- Web UI or hosted sync service.
- Rich three-way merge UI for conflicts.
- Automatic migration of every existing dotfile/script without user review.
- Plugin system for third-party target adapters.

---

## Notes for the AI agent implementing this spec

- Read this spec, `plan-loki-profile-manager.md`, and `tasks-loki-profile-manager.md` before writing code.
- Implement phases in order. Do not jump to TUI before CLI/core behavior exists.
- Use temp directories heavily in tests. Never run tests against the real home directory or real synced store unless explicitly asked.
- Treat acceptance criteria as tests.
- Keep source-of-truth data in YAML manifests and store files. SQLite is rebuildable local state.
- Do not hardcode secrets. Do not print secret values.
- Unsafe overwrite protection is mandatory, not polish.
- If any profile/tool target behavior is ambiguous, implement a manifest-driven adapter and document the assumption rather than guessing silently.
