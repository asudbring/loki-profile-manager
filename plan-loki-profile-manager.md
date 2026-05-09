<!-- historical-doc -->

> **Historical note:** Historical planning document. Current implementation status is documented in README.md, docs/USAGE.md, docs/ARCHITECTURE.md, and CHANGELOG.md.

# Plan: Loki Profile Manager

**Status:** Ready for implementation  
**Owner:** asudbring  
**Last updated:** 2026-05-03

## 1. Implementation intent

Build Loki Profile Manager as a local-first Go application with two user interfaces over one core domain layer:

- CLI for scriptable operations and deterministic tests.
- Bubble Tea TUI for daily use.

The implementation should favor explicit manifests, typed data models, dry-run planning, and temp-dir integration tests. Avoid side effects until a command has produced and validated an activation plan.

## 2. Architecture overview

```text
                 ┌─────────────────────┐
                 │      Bubble Tea TUI  │
                 └──────────┬──────────┘
                            │
┌─────────────────────┐     │     ┌─────────────────────┐
│      CLI commands   │─────┴────▶│   Application svc   │
└─────────────────────┘           └──────────┬──────────┘
                                             │
        ┌────────────────────────────────────┼────────────────────────────────────┐
        │                                    │                                    │
┌───────▼────────┐  ┌───────────────┐  ┌─────▼──────────┐  ┌─────────────────────▼┐
│ Store manager  │  │ Machine reg   │  │ Profile resolver│ │ Verify/doctor engine │
└───────┬────────┘  └───────┬───────┘  └─────┬──────────┘  └──────────────────────┘
        │                   │                │
        │          ┌────────▼────────────────▼─────────┐
        │          │        Activation planner          │
        │          └────────────────┬───────────────────┘
        │                           │
        │          ┌────────────────▼───────────────────┐
        │          │        Activation executor          │
        │          │ symlink/copy/render/merge/snapshot │
        │          └─────┬───────────┬──────────┬───────┘
        │                │           │          │
┌───────▼─────┐ ┌────────▼────┐ ┌────▼─────┐ ┌─▼─────────────┐
│ YAML store  │ │ Skill mgr   │ │ Sync mgr │ │ Infisical CLI │
└─────────────┘ └─────────────┘ └──────────┘ └───────────────┘
        │                │           │
        │        ┌───────▼──────┐    │
        │        │ Tool adapters│    │
        │        └──────────────┘    │
        │                           │
┌───────▼───────────────────────────▼─────┐
│ Local provider sync folder: loki/        │
│ OneDrive/Dropbox desktop client syncs it│
└─────────────────────────────────────────┘
```

Design principle: all high-risk operations go through plan -> validate -> snapshot -> execute -> verify -> commit state. TUI should call the same application services as CLI commands.

## 3. Repository structure

Recommended Go module layout:

```text
loki-profile-manager/
  README.md
  go.mod
  go.sum
  cmd/
    loki/
      main.go
  internal/
    app/
      service.go
      errors.go
    cli/
      root.go
      status.go
      switch.go
      sync.go
      import_skill.go
      verify.go
      doctor.go
      tui.go
    tui/
      model.go
      dashboard.go
      switcher.go
      import_skill.go
      doctor.go
      styles.go
    config/
      paths.go
      local_state.go
    store/
      discover.go
      layout.go
      manifests.go
      schema.go
      validate.go
    machine/
      id.go
      registry.go
      heartbeat.go
      policy.go
    profile/
      resolver.go
      layers.go
    activation/
      plan.go
      planner.go
      execute.go
      symlink.go
      copy.go
      render.go
      merge_json.go
      merge_yaml.go
      merge_toml.go
      snapshot.go
      rollback.go
      overwrite.go
    skills/
      import.go
      validate.go
      zip.go
      markdown.go
      mirror.go
    targets/
      adapter.go
      pi.go
      agents.go
      claude.go
      copilot.go
      vscode.go
    sync/
      status.go
      conflicts.go
      lock.go
    watcher/
      watcher.go
      capture.go
    infisical/
      cli.go
      render.go
      redact.go
    doctor/
      checks.go
      report.go
    db/
      sqlite.go
      migrations.go
    log/
      logger.go
      redact.go
  testdata/
    stores/
    skills/
    manifests/
  docs/
    manifest-schema.md
    migration-from-dotfiles-work.md
    tool-targets.md
```

Keep package boundaries strict. Packages under `internal` should not import `cli` or `tui`; interfaces face inward from app service.

## 4. Store layout and source of truth

Cloud/provider-synced store:

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

Rules:

- YAML manifests and stored files are source of truth.
- Registry JSON and heartbeat files are shared generated state.
- `conflicts/`, `snapshots/`, and `logs/` exist in layout, but V1 local activation snapshots and detailed logs should live in local app state by default to avoid unnecessary cloud churn. Only concise machine heartbeat/error summary goes into shared registry.
- SQLite is local cache/runtime state. It can be deleted and rebuilt.
- Never put secret values in `loki/` source files.

Local app state:

```text
Windows: %LOCALAPPDATA%/loki-profile-manager/
macOS:   ~/Library/Application Support/loki-profile-manager/

state.sqlite
logs/loki.log
snapshots/<timestamp>/
locks/loki.lock
cache/
```

## 5. Manifest design

Use YAML for human-authored manifests.

### 5.1 Manifest schema v1

```yaml
version: 1
name: work-core

files:
  - id: gitconfig
    source: files/git/.gitconfig
    target: "~/.gitconfig"
    mode: symlink
    capture: false

  - id: vscode-settings
    source: files/vscode/settings.json
    target: "${vscode_user_dir}/settings.json"
    mode: merge
    format: json
    capture: true

  - id: pi-config
    source: templates/pi-config.json.tmpl
    target: "~/.pi/config.json"
    mode: render
    format: json
    secrets:
      - INFISICAL_PROJECT_ID
      - INFISICAL_ENV

skills:
  - source: skills/
    targets:
      - pi
      - agents
      - claude_global
      - claude_repo
      - copilot
      - vscode

ignore:
  - "**/Cache/**"
  - "**/cache/**"
  - "**/logs/**"
  - "**/Session Storage/**"

merge_rules:
  json: deep_object_bucket_wins
  yaml: deep_mapping_bucket_wins
  toml: table_bucket_wins

targets:
  vscode_user_dir:
    windows: "${APPDATA}/Code/User"
    darwin: "${HOME}/Library/Application Support/Code/User"
```

### 5.2 Activation modes

- `symlink`: create symlink to source file/directory.
- `copy`: copy source to target. Can be watched if `capture: true`.
- `merge`: merge structured config from multiple layers into one target.
- `render`: render template with Infisical values.

### 5.3 Layer order

For active profile `work` with buckets `content-dev,azure`:

1. `profiles/common/manifest.yaml`
2. `profiles/work/core/manifest.yaml`
3. `profiles/work/buckets/content-dev/manifest.yaml`
4. `profiles/work/buckets/azure/manifest.yaml`

Bucket order should be the order supplied by user/recorded active state. Later bucket wins on key conflicts.

### 5.4 Merge semantics

- JSON: parse both as objects; recursively merge object keys; arrays replace by later layer unless future schema declares append/union.
- YAML: parse mappings; recursively merge mappings; sequences replace by later layer unless future schema declares append/union.
- TOML: parse tables; recursively merge tables; arrays replace by later layer unless future schema declares append/union.
- Type conflict: fail activation before writing.
- Unsupported duplicate target: fail unless all operations form a compatible structured merge group.

## 6. Machine registry

### 6.1 Shared machine registry

`registry/machines.json`:

```json
{
  "version": 1,
  "machines": [
    {
      "machine_id": "uuid-or-stable-id",
      "display_name": "Allen Work Laptop",
      "os": "windows",
      "hostname": "HOSTNAME",
      "allowed_parent_profiles": ["work"],
      "allowed_buckets": ["content-dev", "azure"],
      "last_seen": "2026-05-03T03:00:00Z",
      "active_profile": "work",
      "active_buckets": ["content-dev", "azure"],
      "loki_version": "0.1.0"
    }
  ]
}
```

`registry/machines/<machine_id>.json` duplicates current state as per-machine heartbeat for conflict isolation and quick dashboard display.

### 6.2 Machine ID

Use a generated UUID stored in local app state. Do not derive only from hostname because hostnames can change and collide.

### 6.3 Delete machine

Machine deletion removes:

- Entry from `registry/machines.json`.
- `registry/machines/<machine_id>.json`.

It does not alter files on the removed machine because that machine may be offline.

## 7. CLI design

### 7.1 Common flags

- `--store <path>` override store root.
- `--json` for status/verify/doctor machine-readable output.
- `--verbose` for extra terminal diagnostics.
- `--dry-run` for switch/sync/verify-like commands where applicable.
- `--yes` to confirm safe prompts in non-interactive contexts. Must not bypass unmanaged overwrite safety unless explicit adopt/replace flag is added later.

### 7.2 Commands

```text
loki tui
```
Launches TUI. Default command may also launch TUI if no args.

```text
loki status [--json] [--verbose]
```
Prints active profile/buckets, machine record, store path, sync/offline/pending state, pending capture changes, skill counts, recent errors.

```text
loki switch <profile> [buckets...] [--dry-run] [--yes]
```
Validates machine policy, resolves layers, validates activation plan, snapshots, executes, rolls back on failure.

```text
loki sync [--dry-run]
```
Handles provider conflict copies, writes heartbeat, reconciles state current-machine-wins, reports pending/offline.

```text
loki import-skill <path> [--target <target>] [--overwrite]
```
Imports folder/zip/markdown. If `--target` absent, asks explicitly. If duplicate and `--overwrite` absent, asks.

```text
loki verify [--json]
```
No side effects. Validates manifests, registry, machine policy, targets, mergeability, Infisical CLI availability for required templates, skill references, path permissions where possible.

```text
loki doctor [--json]
```
Environment and repair diagnostics. Includes provider folder state, conflict copies, stale machines, missing target tools/folders, permission problems, lock files, SQLite integrity.

## 8. TUI design

Use Bubble Tea. Keep all business logic in app services; TUI only orchestrates and renders.

### 8.1 Dashboard

Must show:

- Active profile and buckets.
- Current machine display name, OS, hostname, machine ID short form.
- Store path and provider guess.
- Sync status: synced, pending/offline, conflicts resolved, errors.
- Pending local captured changes.
- Skill counts by target/layer.
- Recent conflicts/errors.
- Quick actions: switch, sync, import skill, verify, doctor, quit.

### 8.2 Switcher

Flow:

1. Choose parent profile from allowed profiles.
2. Choose zero or more buckets from allowed buckets belonging to parent.
3. Show dry-run activation summary.
4. Confirm.
5. Execute and show success/rollback/error.

### 8.3 Import skill

Flow:

1. Enter path or browse via simple prompt.
2. Validate source type.
3. Choose explicit destination: common, parent core, or bucket.
4. If duplicate, ask overwrite.
5. Import and show mirror status.

### 8.4 Verify/doctor views

Grouped issues:

- Blocking.
- Warning.
- Info.

Each issue includes path, reason, and suggested fix.

## 9. Activation engine design

### 9.1 Plan generation

Input:

- Store root.
- Machine record.
- Target parent profile and buckets.
- Current active state.
- Layer manifests.

Output:

- Ordered operations.
- Snapshot plan.
- Target safety decisions.
- Merge/render plans.
- Tool mirror plans.
- Expected final hashes.

No writes during planning.

### 9.2 Target safety

Classify existing target:

- Missing: safe.
- Loki-managed symlink: safe if points to expected store or previous Loki target.
- Loki-managed copied/rendered file: safe if hash matches previous Loki state or manifest allows replace.
- Unmanaged real file/dir: block.
- Broken symlink: block unless points to previous Loki path and user confirms repair.

### 9.3 Snapshot

Before writes, copy affected existing managed targets to local snapshot dir and record metadata:

```json
{
  "snapshot_id": "timestamp-id",
  "machine_id": "...",
  "created_at": "...",
  "previous_active_profile": "work",
  "previous_active_buckets": ["content-dev"],
  "targets": [
    {
      "path": "...",
      "kind": "file",
      "hash": "sha256",
      "snapshot_path": "..."
    }
  ]
}
```

Keep last two snapshots.

### 9.4 Execution order

1. Acquire local lock.
2. Validate current machine registered and policy allowed.
3. Generate plan.
4. Validate target safety.
5. Create snapshot.
6. Execute file operations into temporary files where possible.
7. Atomic rename temporary files into target paths where possible.
8. Mirror skills/prompts.
9. Update local SQLite state.
10. Update machine heartbeat.
11. Release lock.

If any step after snapshot fails, attempt rollback.

### 9.5 Rollback

Rollback restores files from snapshot and active state from previous record. If rollback fails, log detailed emergency recovery instructions and leave snapshot untouched.

## 10. Sync and conflict design

### 10.1 Local provider model

Loki never authenticates to OneDrive/Dropbox. It only reads/writes local files under `loki/`.

Provider status is inferred from:

- Store path exists/writable.
- Conflict-copy patterns exist.
- Recent heartbeat write succeeded.
- Optional provider-specific path heuristics.

### 10.2 Current machine wins

When conflict appears:

- Prefer current machine's active manifest/state.
- Delete provider conflict-copy file.
- Log path and decision.
- Do not save losing content into extra Loki backup.

Conflict copy pattern support should include common OneDrive/Dropbox names, e.g.:

- `*conflicted copy*`
- `*'s conflicted copy*`
- `*ComputerName*conflict*`

Keep patterns configurable if easy.

### 10.3 Locking

Use local lock file in local app state. Second command exits with operation-in-progress. Stale lock detection can be part of doctor; avoid unsafe automatic lock deletion unless process is clearly gone.

## 11. Watcher/capture design

Use a cross-platform watcher library such as `fsnotify`.

Rules:

- Only watch copied targets with `capture: true`.
- Ignore paths matching manifest ignore rules.
- Debounce approximately 2 seconds.
- Hash file before write-back.
- If unchanged from last Loki hash, ignore.
- If changed and owner layer is unambiguous, copy back to source in active layer and update local state.
- If owner layer ambiguous due merged output, record pending change and ask user to choose target/diff later.

V1 can run watcher during TUI session. A persistent background daemon is not required unless implementation chooses to support it locally.

## 12. Skill import and mirroring design

### 12.1 Import sources

- Folder containing `SKILL.md`.
- Zip archive containing a skill folder.
- Single markdown file converted to folder with `SKILL.md`.

### 12.2 Validation

Reject if:

- Missing `SKILL.md`.
- Missing `name` frontmatter.
- Missing `description` frontmatter.
- Broken relative references from `SKILL.md`.
- Unsafe archive paths (`..`, absolute paths, drive roots).

No partial destination remains after validation failure.

### 12.3 Target selection

User must explicitly choose one:

- `common`
- parent core: `work`, `dev`, `writer`
- bucket: `work/content-dev`, `work/azure`, etc.

No silent default to active profile.

### 12.4 Duplicate behavior

If same skill name exists in selected target, prompt before overwrite. CLI `--overwrite` can skip prompt.

### 12.5 Mirror targets

V1 target adapters:

- Pi: `~/.pi/agent/skills`.
- Generic agents: `~/.agents/skills`.
- Claude global: `~/.claude/skills`.
- Claude repo-local: `.claude/skills` when repo target configured.
- Copilot:
  - global Copilot instructions if present/configured.
  - repo `.github/copilot-instructions.md`.
  - repo `.github/instructions/*.instructions.md`.
  - repo `.github/prompts/*.prompt.md`.
  - VS Code Copilot settings in `settings.json`.
- VS Code:
  - user `settings.json`.
  - user `mcp.json`.

Adapters should be manifest-driven because exact paths differ by OS/tool version.

## 13. Infisical design

Use CLI only.

Principles:

- Do not call Infisical API directly in V1.
- Do not store Infisical secret values.
- Do not log command output containing secret values.
- Missing secret names can be shown; values cannot.
- Prefer running `infisical run` or `infisical secrets get` in a way that avoids printing secrets to terminal. Implementation must inspect actual CLI behavior before choosing final command.

Template rendering can use Go templates or a simple placeholder renderer. If using Go templates, document delimiters. Missing values fail activation.

## 14. Verification and doctor checks

### 14.1 Verify checks

No side effects:

- Store layout valid.
- Machine registered.
- Machine policy permits requested/active profile.
- YAML manifests parse.
- Manifest sources exist.
- Target paths expand for current OS.
- Duplicate targets merge safely.
- Unmanaged target conflicts are detected.
- Skill metadata and references valid.
- Infisical CLI exists if templates require secrets.
- Tool target directories are resolvable or creatable.

### 14.2 Doctor checks

May inspect broader environment:

- OneDrive/Dropbox folder existence/writability.
- Conflict-copy files.
- Stale machine heartbeats.
- Stale local lock.
- SQLite integrity.
- Snapshot retention health.
- Missing target apps/folders.
- Missing Infisical CLI/auth hints.
- Permissions for symlink creation on Windows.

Doctor should report actions, not silently repair unless a future `--fix` flag is added.

## 15. Testing strategy

### 15.1 Unit tests

- Path expansion.
- YAML parsing/validation.
- Machine policy.
- Layer ordering.
- Merge algorithms.
- Skill validation.
- Conflict-copy detection.
- Redaction.

### 15.2 Integration tests

Use temp dirs as fake home and fake store.

Scenarios:

- First-run setup creates store and machine.
- Switch creates symlink/copy/render outputs.
- Unsafe unmanaged file blocks switch.
- JSON/YAML/TOML merges produce expected output.
- Failed activation rolls back.
- Snapshot retention keeps last two.
- Skill folder/zip/markdown imports.
- Duplicate skill prompt behavior via injectable prompt interface.
- Sync deletes conflict-copy files.
- Lock prevents concurrent command.

### 15.3 TUI model tests

Test Bubble Tea update functions without terminal rendering:

- Dashboard loads status.
- Switcher validates choices.
- Import flow asks target and handles duplicate.
- Doctor view groups issues.

### 15.4 Manual smoke tests

- Windows: build binary, run setup against temp OneDrive-like folder, switch profile, run TUI.
- macOS: same.
- Infisical installed: template render with dummy test secret.
- Infisical missing: verify/activation fail correctly.

## 16. Packaging and docs

V1 packaging:

- `go build` binaries for Windows and macOS.
- Manual binary copy install instructions.
- Homebrew/winget deferred.

Docs required before build handoff complete:

- README with problem, setup, commands, safety model.
- Manifest schema doc.
- Tool targets doc.
- Migration notes from existing `.dotfiles-work` pattern.
- Troubleshooting section for symlinks, Infisical, conflict copies, unsafe overwrite.

## 17. Key implementation cautions

- Never test against real user home by default. Inject home/store paths.
- Treat Windows symlink permission quirks as first-class.
- Keep secret redaction central and covered by tests.
- Keep prompts abstract behind an interface so CLI/TUI/tests can share workflows.
- Avoid hidden magic defaults for skill target selection.
- Do not make provider sync assumptions stronger than local file presence/writability.
- Make dry-run output useful early; it becomes the basis for verify and TUI previews.
