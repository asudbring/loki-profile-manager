# Architecture

Loki Profile Manager is a local CLI. It uses a synced filesystem folder as the durable source of truth and a machine-local SQLite database for runtime state.

## System overview

```mermaid
flowchart TB
    User([User])
    CLI{loki CLI}
    TUI[Bubble Tea TUI]
    App[App service]

    subgraph LocalState[Machine-local state]
      DB[(SQLite state.sqlite)]
      Logs[logs/loki.log]
      Snapshots[activation snapshots]
      MachineID[machine_id]
    end

    subgraph Store[Synced Loki store]
      Registry[registry/machines.json]
      Heartbeats[registry/machines/*.json]
      Profiles[profiles/*/manifest.yaml]
      Files[profile files/templates/skills]
    end

    subgraph Core[Core packages]
      StorePkg[store]
      MachinePkg[machine]
      ManifestPkg[manifest]
      ProfilePkg[profile]
      VerifyPkg[verify]
      DoctorPkg[doctor]
      SyncPkg[storesync]
      StoreMigratePkg[storemigrate]
      ActivationPkg[activation]
      InfisicalPkg[infisical]
    end

    InfisicalCLI[Infisical CLI]

    User -->|invokes| CLI
    CLI --> App
    CLI --> TUI
    TUI --> App
    App --> StorePkg
    App --> MachinePkg
    App --> VerifyPkg
    App --> DoctorPkg
    App --> SyncPkg
    App --> StoreMigratePkg
    App --> ActivationPkg
    StorePkg --> Store
    MachinePkg --> Registry
    MachinePkg --> Heartbeats
    MachinePkg --> MachineID
    VerifyPkg --> ManifestPkg
    VerifyPkg --> ProfilePkg
    ActivationPkg --> ManifestPkg
    SyncPkg --> Store
    ActivationPkg --> ProfilePkg
    ActivationPkg --> DB
    ActivationPkg --> Snapshots
    ActivationPkg --> InfisicalPkg
    InfisicalPkg -->|render secrets| InfisicalCLI
    App --> Logs
```

The CLI is thin. It parses commands and calls the app service. `loki tui` is also thin: it runs a Bubble Tea model over a narrow fakeable `internal/tui.Client` adapter and delegates business logic to app services. The app service owns local path resolution, logging, database bootstrap, operation locks, safety checks, snapshots, restore guards, and orchestration. `doctor` uses a diagnostic path that resolves local paths and opens existing SQLite state read-only instead of bootstrapping it. Store data and manifests remain in a user-selected synced folder. Local runtime state remains outside the synced store.

## Data ownership

```mermaid
flowchart LR
    subgraph SourceOfTruth[Synced store: source of truth]
      Manifests[YAML manifests]
      ProfileFiles[Files, templates, skills]
      MachineRegistry[Machine registry]
    end

    subgraph LocalRuntime[Local runtime state]
      SQLite[(SQLite)]
      ManagedTargets[managed_targets hashes]
      KV[kv_state active profile]
      LocalSnapshots[Snapshots]
      Logs[Logs]
    end

    subgraph Targets[User target paths]
      Dotfiles[Dotfiles]
      AppSettings[App settings]
      Rendered[Rendered templates]
      Links[Symlinks]
    end

    Manifests -->|plan| Targets
    ProfileFiles -->|symlink/copy/merge/render| Targets
    MachineRegistry -->|policy| Targets
    Targets -->|hash tracking| ManagedTargets
    KV -->|previous active state| LocalSnapshots
    Targets -->|before writes| LocalSnapshots
    SQLite --> ManagedTargets
    SQLite --> KV
```

The synced store contains declarative intent. SQLite tracks what this machine last applied and which target hashes are safe to replace. Snapshots are local recovery artifacts, not synced source material.

## Store layout

`internal/store/layout.go` creates and validates this layout:

```text
loki/
├── registry/
│   ├── machines.json
│   └── machines/
├── profiles/
│   ├── common/
│   │   ├── files/
│   │   ├── skills/
│   │   ├── templates/
│   │   └── manifest.yaml
│   ├── work/
│   │   ├── core/
│   │   │   ├── files/
│   │   │   ├── skills/
│   │   │   ├── templates/
│   │   │   └── manifest.yaml
│   │   └── buckets/
│   ├── dev/
│   └── writer/
├── conflicts/
├── snapshots/
└── logs/
```

The store directories `conflicts/`, `snapshots/`, and `logs/` exist in the store layout for future sync/provider workflows. Phase 4 activation snapshots are stored in local app state to avoid syncing machine-local recovery data. `loki import-skill` can create profile bucket layer directories (`files/`, `skills/`, `templates/`, and `manifest.yaml`) under an existing parent profile.

Store-root migration is handled by `loki store migrate`. The CLI delegates orchestration to `internal/app`, while `internal/storemigrate` owns dry-run planning, destination safety checks, conflict-copy blocking, cross-platform cloud placeholder detection, explicit hydration, staged filesystem copy, progress events, promotion, cleanup, and validation. The app service copies from the old valid store to a hidden `.DESTINATION.incomplete-*` sibling under the source store operation lock, validates staging, promotes staging to the missing/empty final destination, then rewires local SQLite by updating `kv_state.store_path` and rebasing `managed_targets.source_path` plus managed-target metadata source paths in one transaction. After rewire, Loki retargets active Loki-managed symlinks that still point into the old store root. The old store is never deleted, and provider labels such as `onedrive-business` and `dropbox` are metadata only; OneDrive/Dropbox desktop clients still perform cloud sync.

## Local state

`internal/config/paths.go` resolves local paths by OS.

Windows:

```text
%LOCALAPPDATA%\loki-profile-manager\
```

macOS:

```text
~/Library/Application Support/loki-profile-manager/
```

Linux development fallback:

```text
~/.local/state/loki-profile-manager/
```

Local files include:

```text
state.sqlite
logs/loki.log
snapshots/
locks/
cache/
machine_id
active_profile.txt
```

`active_profile.txt` mirrors the local active profile and buckets in `profile:bucket,bucket` form. Shell prompts and terminal startup files can read this marker without depending on legacy profile repositories or the synced machine registry being immediately available.

## SQLite schema

`internal/db/migrations.go` creates:

| Table | Purpose |
|---|---|
| `kv_state` | Store path and active profile/bucket state. |
| `managed_targets` | Target paths, source paths, modes, hashes, layer metadata, and last apply time. |
| `snapshots` | Snapshot metadata and prior active profile/bucket state. |
| `pending_captures` | Reserved for future capture/sync workflows. |
| `command_history` | Reserved for future command audit/history. |
| `schema_migrations` | Applied local SQLite migrations. |

## Manifest model

Manifests are YAML v1 files. Target expansion is OS-aware through `internal/config` and `internal/manifest`. On Windows, `${DOCUMENTS}`, `${DOCUMENTS_DIR}`, and `${USER_DOCUMENTS}` resolve through the Windows Known Folder API when available so PowerShell profiles land under redirected Documents folders such as Parallels `C:\Mac\Home\Documents` instead of assuming `C:\Users\<user>\Documents`.

Current file modes:

| Mode | Responsibility |
|---|---|
| `symlink` | Create a symlink from target to source. |
| `copy` | Copy source file or directory to target. |
| `merge` | Merge structured JSON/YAML/TOML files targeting the same path. |
| `render` | Render a template using Infisical secret values. |

Profile resolution order:

1. `profiles/common/manifest.yaml`
2. `profiles/<profile>/core/manifest.yaml`
3. `profiles/<profile>/buckets/<bucket>/manifest.yaml` in the order requested by the user

Later layers win during structured merge.

## Switch flow

```mermaid
sequenceDiagram
    actor User
    participant CLI as Cobra CLI
    participant App as App service
    participant Machine as machine package
    participant Planner as activation planner
    participant Safety as overwrite detector
    participant Snapshot as snapshot store
    participant Exec as activation executor
    participant DB as SQLite
    participant Store as Loki store

    User->>CLI: loki switch work [buckets] [--dry-run]
    CLI->>App: Switch(request)
    App->>Machine: Ensure machine_id
    App->>Machine: Load registry record
    Machine-->>App: policy result or blocking registration/policy error
    App->>Planner: BuildPlan(store, profile, buckets)
    Planner->>Store: Read manifests and sources
    Planner-->>App: activation plan
    App->>Safety: Check copied target drift and ValidateSafety(plan, DB)
    Safety->>DB: Read managed_targets
    Safety-->>App: safe plan, capture requirement, or blocking error
    opt capture-local requested for safe copy-mode drift
        App->>Store: Write safe local changes back to store sources
        App->>Planner: RebuildPlan(store, profile, buckets)
        App->>Safety: Recheck plan and target safety
    end
    alt dry-run
        App-->>CLI: plan only
    else activate
        App->>Snapshot: CreateSnapshot(targets)
        Snapshot->>DB: Insert snapshot record
        App->>Exec: Execute operations
        Exec->>DB: Upsert managed target hashes
        Exec->>DB: Set active profile/buckets
        Exec->>App: Write active_profile.txt marker
        App->>DB: Remove obsolete unchanged managed targets
        App->>Machine: Update heartbeat
        App-->>CLI: switch result
    end
```

Safety validation happens before snapshots and before any target writes. `--capture-local` writes only safe copied managed-target drift back to store sources before activation and then rechecks safety. `--backup-unmanaged --yes` is the explicit first-install remediation path when the synced store should win: Loki classifies blockers, moves only unmanaged file/directory blockers to machine-local state under `unmanaged-backups/<timestamp>/`, writes a backup manifest, rechecks safety, then activates. Render outputs are regenerated from templates and are not captured. Rollback runs for failures after snapshot creation; unmanaged pre-switch backups are preserved separately for manual recovery.

## Sync MVP flow

`loki sync` currently resolves provider conflict-copy files only. It does not call OneDrive/Dropbox APIs, run a background watcher, or capture changed app targets.

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant App
    participant Lock as store lock
    participant Scanner as storesync scanner
    participant Store as Loki store
    participant Machine as machine registry

    User->>CLI: loki sync --dry-run or --yes
    CLI->>App: Sync(request)
    App->>Store: Validate layout
    alt dry-run
        App->>Scanner: Scan conflict-copy filenames
        Scanner->>Store: Walk store paths, no file contents
        App-->>CLI: planned deletions + conflict fingerprint
    else yes
        App->>Lock: Acquire cooperative operation lock
        App->>Scanner: Rescan conflict-copy filenames under lock
        Scanner->>Store: Walk store paths, no file contents
        App->>App: Check expected fingerprint when provided
        App->>Machine: Require registered current machine
        App->>Store: Delete file/symlink conflict copies
        App->>Machine: Update heartbeat
        App-->>CLI: deleted/skipped counts
    end
```

The current-machine-wins policy means detected provider conflict-copy files are treated as losing provider artifacts. Loki deletes regular-file and symlink conflict copies only after `--yes` when the filename has a strong provider conflict-copy signal. Broad `case conflict` names, directory conflict copies, and non-regular entries are skipped and reported. Dry-run does not write a lock, machine ID, heartbeat, or conflict deletion. Callers that execute from a prior dry-run can pass an expected conflict fingerprint; app code rescans under the write lock and aborts before deletion if the conflict list changed. No losing-content backup is created.

## TUI MVP flow

`loki` with no arguments and `loki tui` use Bubble Tea under `internal/tui`. The TUI is presentation/orchestration only; app services remain write authority.

```mermaid
sequenceDiagram
    actor User
    participant TUI as Bubble Tea model
    participant Client as tui.Client adapter
    participant App as app.Service

    User->>TUI: open dashboard
    TUI->>Client: Status / StoreStatus / DiscoverStores / Doctor / MachineStatus / SecretsStatus / ProfileCatalog / ListSnapshots
    Client->>App: service calls
    App-->>Client: typed results
    Client-->>TUI: tea.Msg values
    TUI-->>User: dashboard + detail screens

    User->>TUI: guarded action
    TUI->>Client: Dry-run or setup service call
    Client->>App: Switch / Sync / RestoreSnapshotDryRun / UseStore / EnsureStore / ForgetStore / RegisterMachine
    App-->>TUI: plan, blockers, warnings, fingerprint/guard
    alt switch or sync execute
        User->>TUI: exact typed confirmation
        TUI->>Client: dry-run recheck
        TUI->>TUI: compare fingerprint
        TUI->>Client: app-owned Yes:true call
        App-->>TUI: result or safety error
    else snapshot restore
        TUI-->>User: guarded CLI restore command only
    end
```

TUI write-capable flows are intentionally narrow:

- Store setup requires exact `USE STORE`, `INIT STORE`, or `UNSET STORE` phrase before persisting local config or creating a missing/empty store layout.
- Machine registration requires exact `REGISTER MACHINE` phrase before writing the synced machine registry.
- Switch requires successful dry-run, exact `SWITCH <profile> [bucket...]` phrase, and dry-run fingerprint recheck before `Switch(Yes:true)`.
- Sync requires successful dry-run, exact `DELETE <n> CONFLICTS` phrase, dry-run recheck, and app-side expected conflict fingerprint before `Sync(Yes:true)` deletes conflict copies.
- Snapshot restore writes are not exposed. TUI can record a restore dry-run guard and displays the existing guarded CLI restore command.
- Secrets views render names/status only. Snapshot views render metadata only and do not read or print file contents.

## Skill import MVP flow

`loki import-skill` imports one existing skill folder or `.zip` archive into store source-of-truth only. It does not mirror to runtime skill directories.

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant App
    participant Validator as skills validator
    participant Lock as store lock
    participant Store as Loki store
    participant Manifest as manifest writer

    User->>CLI: loki import-skill <source> --profile work --yes
    CLI->>App: ImportSkill(request)
    App->>Store: Validate layout and destination layer
    App->>App: Prepare folder or safely extract zip to temp staging
    App->>Validator: Validate SKILL.md and local references
    App->>App: Reject source symlinks
    App->>Lock: Acquire cooperative operation lock
    App->>Store: Check skills/<name> conflict
    alt dry-run
        App-->>CLI: planned copy and manifest update
    else yes
        App->>Store: Copy folder to selected layer skills/<name>
        App->>Manifest: Add/update skills source entry
        App-->>CLI: changed count
    end
```

Existing destinations require `--overwrite`. Source symlinks are rejected for the MVP so imported skill content is regular directories/files only. Zip archives must contain `SKILL.md` at archive root or exactly one top-level skill folder, and extraction normalizes Windows path separators while rejecting traversal, absolute paths, Windows drive paths, symlink entries, non-regular entries, oversized archives, and ambiguous roots.

## Unsafe overwrite protection

The overwrite detector uses `os.Lstat`, symlink inspection, target hashes, and `managed_targets` records.

| Target state | Result |
|---|---|
| Missing | Safe. |
| Loki-managed symlink | Safe only when a symlink-mode `managed_targets` record matches the link target. |
| Loki-managed file/directory hash match | Safe. |
| Loki-managed render target | Safe for render operations; regenerated from the template. |
| Obsolete Loki-managed target hash match | Removed after successful switch and deleted from local state. |
| Obsolete Loki-managed target hash mismatch | Blocks before activation; capture, remove, or adopt manually. |
| Unmanaged file | Blocked. |
| Unmanaged directory | Blocked. |
| Broken symlink | Blocked. |
| Managed hash mismatch | Blocked. |
| Target outside configured home root | Blocked during manifest validation. |

This is why migration/adoption or explicit unmanaged backup remediation is required before using Loki on a machine with existing config files. `--backup-unmanaged --yes` handles only unmanaged file/directory blockers; broken symlinks, managed hash mismatches, obsolete changed targets, and other unsafe states still require manual repair, capture, migration, or adoption.

## Rollback and snapshot reporting

Activation rollback restores target files from the local snapshot. Targets that did not exist before activation are removed only if they still match the Loki-created hash and mode. Prior `managed_targets` rows and active profile/bucket key-value state are restored from snapshot metadata after filesystem rollback succeeds. If filesystem rollback fails, DB state is left unchanged and the snapshot path is reported for manual recovery.

After successful activation, Loki compares local `managed_targets` against the new activation plan. Records absent from the new plan are obsolete. Missing obsolete targets only lose their stale state record. Existing obsolete targets are removed only when their hash still matches the last Loki-applied hash. Render targets are generated from templates, so obsolete rendered files are removed even if their local hash drifted. Changed obsolete targets in other modes block before activation so old profile skills and app config cannot silently leak into the newly active profile.

Snapshot reporting is read-only. `loki snapshots list` combines SQLite rows with local `metadata.json` files and marks stale/missing directories as degraded instead of panicking. `loki snapshots show` reads metadata only: target paths, kinds, hashes, modes, and snapshot entry paths. It never reads or prints snapshot entry contents.

`loki snapshots restore <id> --dry-run` previews restore actions without writing target files or mutating active state. It hashes only current non-sensitive targets for conflict context, redacts sensitive-looking paths, blocks sensitive targets by default, and records a short-lived restore guard when the plan is restorable.

`loki snapshots restore <id> --yes` requires the guard fingerprint from a matching prior dry-run. Before any restore write, it creates a pre-restore snapshot of current target files, managed-target rows, and active profile/buckets with retention disabled. It then restores target files/symlinks from the selected snapshot, restores managed-target rows and active state, and clears the guard. If a restore write fails, Loki rolls back with the pre-restore snapshot and preserves that snapshot for emergency recovery. `--target <path>` scopes both guard and restore to one exact snapshot target; targeted restore updates only that target and its managed-target row, not global active profile/buckets.

## Secrets integration

Render mode uses an injectable secret provider. The V1 provider is Infisical through the Infisical CLI. Secret values are written only to the intended rendered target file. Missing secrets report names only. Loki never stores Infisical tokens or secret values in the synced store or local SQLite.

`loki secrets configure infisical` is the interactive setup path. The CLI prompts for project ID, environment, client ID, masked client secret/key, and optional host URL; the TUI Secrets screen exposes the same app service behind `c`. Both paths validate Universal Auth credentials with Infisical before writing, leave any existing machine-local env file unchanged on validation failure, write only the machine-local `~/.config/infisical/.env` file with restricted local permissions where supported, store Universal Auth config only, and do not run profile activation. The wizard does not persist minted `INFISICAL_TOKEN` values; readiness verification happens through `loki secrets status`.

`loki secrets --infisical` remains the noninteractive automation path: it creates or updates local `~/.config/infisical/.env` from existing `INFISICAL_*` environment variables and local `.infisical.json` project config, then runs readiness checks while reporting key names only. `loki secrets login` delegates to `infisical login` with inherited terminal I/O, so Loki does not capture login tokens or passwords. `loki secrets status` checks CLI/readiness without printing values and reports invalid local machine-auth config separately from missing interactive CLI login/project setup. `loki secrets check <NAME...>` fetches only named secrets and reports available/missing names only.

The provider seam is intentionally small (`GetSecrets(ctx, names)` plus status/login helpers) so Azure Key Vault and other backends can be added later without changing activation planning or render operations.

Supported placeholders:

```text
{{ SECRET_NAME }}
${SECRET_NAME}
```

Current provider strategy:

- Secret reads call the Infisical CLI internally and return values only to render operations.
- Universal Auth token minting uses the Infisical API so the client secret is not passed in a child-process argument list.
- Interactive user login still delegates to `infisical login` with inherited terminal I/O.

Doctor reports Infisical readiness as a warning when missing or not ready, not a blocking issue unless a render operation later needs secrets.

## Release packaging

Release packaging is script-driven so the same asset set can be produced by GitHub Actions or locally.

```mermaid
flowchart TB
    Commit[Git commit]
    Tag[v* tag]
    ReleaseWorkflow[.github/workflows/release.yml]
    LocalScript[scripts/release-local.sh]
    Validate[tests, vet, mod verify, syntax checks]
    PackageRelease[scripts/package-release.sh]
    PackageNpm[scripts/package-npm.sh]
    Assets[dist/packages assets]
    Checksums[checksums.txt + release-manifest.json]
    Smoke[installer + npm smoke tests]
    GitHubRelease[GitHub Release]
    NpmPublish[.github/workflows/npm-publish.yml]
    NpmRegistry[npm registry]

    Commit --> Tag --> ReleaseWorkflow
    Commit --> LocalScript
    ReleaseWorkflow --> Validate
    LocalScript --> Validate
    Validate --> PackageRelease --> PackageNpm --> Assets
    Assets --> Checksums
    ReleaseWorkflow --> Smoke --> GitHubRelease
    GitHubRelease --> NpmPublish --> NpmRegistry
    LocalScript -->|optional --upload| GitHubRelease
```

`package-release.sh` cross-compiles native binaries for Linux, macOS, and Windows on amd64 and arm64, then writes release archives, installer scripts, `release-manifest.json`, and `checksums.txt`. `package-npm.sh` embeds the native binaries into the npm tarball and rewrites the manifest/checksums to include the tarball. `release-local.sh` wraps validation and packaging for the no-Actions path, refuses destructive output directories outside repo `dist/`, and refuses to clobber an existing GitHub Release unless the remote tag points at the current commit. The npm publish workflows use the same packaging scripts and publish through npm trusted publishing (GitHub Actions OIDC), without a long-lived npm token.

## Current limitations

- No setup CLI.
- Manual snapshot restore exists; sensitive-looking paths are blocked/redacted by default, and per-target restore is available with `--target`.
- Skill folder and zip import exist, but markdown conversion, runtime mirroring, and target-adapter sync are not implemented.
- Secrets V1 supports Infisical only; Azure Key Vault and other providers are deferred.
- Sync is conflict-copy cleanup only; watcher capture and full provider-state reconciliation are not implemented.
- TUI MVP exists, including store setup and machine registration, but no manifest editor, `adopt`/`migrate`/`import-skill` execution forms, daemon control, or inline snapshot restore execution.
- `OperationMirror` is currently a no-op.
- Verify does not reuse activation safety classification because it has no SQLite dependency in its current shape.
