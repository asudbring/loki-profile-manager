# Usage

This document describes the CLI that exists in the current codebase. It does not document planned commands as if they work.

## Command shape

```text
loki [global flags] <command> [command flags]
```

Global flags:

| Flag | Description |
|---|---|
| `--store <path>` | Override the Loki store root path. |
| `--verbose` | Print verbose diagnostics to stderr. |

Current commands:

| Command | Status |
|---|---|
| `status` | Implemented |
| `store status` | Implemented |
| `store discover` | Implemented |
| `store migrate` | Implemented |
| `store use` | Implemented |
| `store init` | Implemented |
| `store unset` | Implemented |
| `verify` | Implemented |
| `switch` | Implemented |
| `sync` | Implemented |
| `tui` | Implemented Bubble Tea MVP |
| `update` | Implemented npm self-update |
| `import-skill` | Implemented folder and zip-import MVP |
| `secrets configure infisical` | Implemented interactive Infisical setup wizard |
| `secrets login` | Implemented Infisical CLI wrapper |
| `secrets status` | Implemented Infisical readiness check |
| `secrets check` | Implemented named-secret presence check |
| `doctor` | Implemented |
| `snapshots list` | Implemented |
| `snapshots show` | Implemented |
| `snapshots restore --dry-run` | Implemented |
| `snapshots restore --yes` | Implemented |
| `machine register` | Implemented |
| `machine status` | Implemented |
| `adopt` | Implemented |
| `migrate repo` | Implemented |
| `migrate local` | Implemented |

Planned but not implemented:

| Command | Planned purpose |
|---|---|
| `import-skill` markdown import | Convert a standalone markdown skill file into a skill folder. |
| Azure Key Vault/other secret providers | Additional render-secret backends beyond Infisical V1. |

## First-run workflows

For full install and first-run procedures, see [`docs/INSTALL.md`](INSTALL.md). For a blank store with no profiles yet, see [`docs/PROFILES.md`](PROFILES.md).

Blank store from scratch:

```bash
STORE="$HOME/OneDrive/LokiProfileManager"
loki store init "$STORE"
loki adopt ~/.gitconfig --profile personal --dry-run
loki adopt ~/.gitconfig --profile personal --yes
loki machine register --allow-profile personal
loki verify personal
loki switch personal --dry-run
```

Fresh machine or already-migrated store:

```bash
STORE="$HOME/OneDrive/LokiProfileManager"
loki store discover --manual "$STORE"
loki store use "$STORE"        # existing valid store with profiles
# or: loki store init "$STORE" # new empty store; add/migrate/import profiles before verify/switch
loki machine register --allow-profile work --allow-bucket content-dev
loki verify work content-dev
loki switch work content-dev --dry-run
loki switch work content-dev --yes
loki status --verbose
```

Existing machine with local profiles/settings not migrated:

```bash
STORE="$HOME/OneDrive/LokiProfileManager"
loki store use "$STORE"
loki machine register --allow-profile work --allow-bucket content-dev
loki migrate local --profile work --dry-run
loki migrate local --profile work --yes
loki verify work content-dev
loki switch work content-dev --dry-run
loki switch work content-dev --yes
```

Safety rules:

- Run `switch --dry-run` before `switch --yes`.
- `--yes` does not bypass unmanaged overwrite protection by itself.
- Create at least one profile core before `verify <profile>` or `switch <profile>` on a new empty store.
- Use `migrate local`, `migrate repo`, `adopt`, or `switch --backup-unmanaged --yes` before first activation on a machine with existing unmanaged files.
- Use `switch --capture-local --yes` only for safe copy-mode local changes from the currently active Loki-managed profile.
- Render outputs are regenerated from templates and are not captured.
- Merge drift is detected but manual in this MVP.

## `loki update`

Install the newest Loki npm build.

```bash
loki update
```

Behavior:

- Runs `npm install -g @asudbring/loki-profile-manager@latest`.
- Uses `npm view @asudbring/loki-profile-manager version --silent` to discover the latest npm version.
- Requires `npm` on `PATH`; if npm is missing, install Node.js/npm or run the npm install command manually after npm is available.
- Always invokes npm install for `@latest`, even when the installed version already matches npm, so the command can repair or refresh the global package.

Startup update notices:

- Human CLI commands check npm for a newer version at most once every 24 hours.
- The check result is cached in Loki local SQLite `kv_state` and reused until the cache expires.
- When newer npm version exists, Loki prints this notice to stderr: ``Newer Loki version available: <version>. Run `loki update` to install the latest npm build.``
- The startup check is skipped for `loki update`, `loki --version`, help output, `--json` commands, `loki`/`loki tui` TUI startup, CI contexts, noninteractive stderr, development builds, and `LOKI_NO_UPDATE_CHECK=1`.
- Update-check failures never fail the command that triggered the notice check.

## `loki tui`

Launch the interactive terminal UI.

```bash
loki
loki tui
loki --store /path/to/loki tui
```

Behavior:

- `loki` with no arguments launches the TUI. Any command or flag uses normal CLI mode.
- Requires an interactive terminal. Non-TTY execution returns a clear error.
- Loads status, store, doctor, machine, secrets, profile catalog, and snapshot summary data through `internal/app` service APIs.
- Dashboard quick actions: `g` store, `w` switch, `y` sync conflicts, `n` snapshots, `d` doctor, `m` machine, `s` secrets, `p` profiles.
- Store screen can discover candidates, manually inspect a path, persist an existing store, initialize a missing/empty store, or unset local store config with typed confirmation.
- Machine screen can register/update this machine with allowed profiles/buckets and active profile/bucket metadata with typed confirmation.
- Switch screen runs `Switch(DryRun:true)`, requires exact `SWITCH <profile> [bucket...]` confirmation, rechecks the dry-run fingerprint, then calls app-owned `Switch(Yes:true)`.
- Sync screen runs `Sync(DryRun:true)`, requires exact `DELETE <n> CONFLICTS` confirmation, rechecks the conflict fingerprint, then calls app-owned `Sync(Yes:true)`.
- Snapshot screen lists and shows metadata only, runs restore dry-runs, and displays the guarded CLI restore command. It does not execute restore writes in the TUI MVP.
- Secrets screen renders provider/readiness/check names and status only. Press `c` to configure Infisical through a masked local-only wizard. It never renders secret values.
- Snapshot views do not read or print snapshot entry file contents; sensitive paths are redacted where app APIs mark them redacted.

Keys:

| Key | Behavior |
|---|---|
| `q`, `ctrl+c` | Quit. |
| `esc` | Back/cancel. |
| `r` | Refresh dashboard. |
| `enter` | Open selected dashboard item, show selected snapshot, or choose selected store candidate. |
| `g` | Open Store screen from dashboard. |
| `d` | Dry-run switch/sync/snapshot restore on action screens; rediscover store candidates on Store screen. |
| `x` | Switch execute confirmation or sync confirmation reset. Snapshot restore has no execute key. |
| `c` | Open Infisical configuration wizard on the Secrets screen. |
| Arrow keys / `hjkl` | Navigate lists, profile/bucket selection, and snapshot targets. |

Example smoke:

```bash
loki tui --help
loki tui
```

## `loki status`

Show local Loki status.

```bash
loki status [--json] [--verbose]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |
| `--verbose` | Also list locally managed targets in human output. |

Behavior:

- Opens the local SQLite state database.
- Reports the local state directory and database path.
- Uses `--store` first when provided.
- Otherwise reads the configured store path from local key-value state.
- Validates the store layout if a store path is configured.
- Reports the active profile and buckets from local state, falling back to the synced machine registry.
- Reports locally managed target count; `--verbose` lists target paths, modes, layers, and source paths.
- Reports current machine registration when the store layout is valid, without creating a machine ID.

Examples:

```bash
loki status
loki status --json
loki --store /path/to/loki status
loki --store /path/to/loki --verbose status
```

## `loki store`

Manage persistent local store configuration. Persistent configuration is stored in local SQLite state and is machine-local. `--store <path>` still overrides the persisted path for one command.

### `loki store status`

```bash
loki store status [--json]
loki --store /path/to/loki store status [--json]
```

Shows persisted path, override path, effective path/source, local state/database paths, and layout validity.

### `loki store discover`

```bash
loki store discover [--manual <path>] [--json]
```

Lists OneDrive, Dropbox, and optional manual candidates with provider root, candidate store path, existence, validity, and missing layout paths.

### `loki store use`

```bash
loki store use <path> [--json]
```

Persists an existing valid Loki store path. It validates only and does not create files.

### `loki store init`

```bash
loki store init <path> [--json]
```

Creates a missing/empty Loki store layout or accepts an existing valid layout, then persists the path. It refuses non-empty invalid directories.

### `loki store migrate`

```bash
loki store migrate --to <path> (--dry-run|--yes) [--from <path>] [--provider <provider>] [--copy-only] [--capture-local] [--hydrate] [--file-timeout <duration>] [--progress-interval <duration>] [--json]
loki store migrate --to <path> --cleanup [--json]
```

Copies a valid Loki store root to a hidden staging sibling, validates the staged layout, then promotes it to a missing/empty destination. By default, `--yes` also rewires this machine's persisted store path, rebases `managed_targets.source_path` rows and metadata source paths, and retargets active Loki-managed symlinks from the old store root to the new store root. `--copy-only` validates and promotes the copied store without changing local SQLite state. The old store is never deleted.

Flags:

| Flag | Description |
|---|---|
| `--to <path>` | Destination Loki store path. Required. Must be missing or empty. |
| `--from <path>` | Source Loki store path. Defaults to the current effective store. |
| `--provider <provider>` | Optional label: `onedrive-business`, `onedrive-personal`, `onedrive`, `dropbox`, or `manual`. |
| `--dry-run` | Validate and estimate the migration without copying or rewiring. |
| `--yes` | Copy the store and, unless `--copy-only` is set, rewire local state. |
| `--copy-only` | Copy and validate only; do not persist the new store path or rebase managed targets. |
| `--capture-local` | Before copying, write safe copy-mode local changes back to the source store. |
| `--hydrate` | Explicitly materialize cloud-only source files before copying. Without this flag, migration fails fast when cloud placeholders are detected. |
| `--file-timeout <duration>` | Maximum time to spend hydrating or copying one source file before failing. Defaults to `2m`. |
| `--progress-interval <duration>` | Minimum interval between same-phase progress messages during `--yes`. Defaults to `2s`. |
| `--cleanup` | Remove interrupted staging directories for the destination and exit. Does not require `--dry-run` or `--yes`. |
| `--json` | Emit machine-readable JSON. |

Safety rules:

- Requires exactly one of `--dry-run` or `--yes`, except `--cleanup` which exits after staging cleanup.
- Refuses non-empty destinations and nested source/destination paths.
- Refuses source stores containing provider conflict-copy files; run `loki sync --dry-run` and resolve them first.
- Refuses cloud-only source files unless `--hydrate` is provided. macOS detects File Provider dataless files; Windows uses conservative Cloud Files attributes when available; Linux/other platforms still get staged copy, progress, timeout, cleanup, rewire, and symlink retarget behavior.
- Refuses local copy-mode drift on `--yes` unless `--capture-local` is provided.
- Copies to a hidden `.DESTINATION.incomplete-*` sibling first, validates staging, then promotes to the final path. Use `--cleanup` if an interrupted run leaves staging behind.
- Does not call provider APIs or wait for OneDrive/Dropbox upload completion; wait for your sync client after the local copy finishes.

Examples:

```bash
loki store migrate --to "$HOME/Dropbox/LokiProfileManager" --provider dropbox --dry-run
loki store migrate --to "$HOME/Dropbox/LokiProfileManager" --provider dropbox --yes
loki store migrate --to "$HOME/Library/CloudStorage/OneDrive-Contoso/LokiProfileManager" --provider onedrive-business --dry-run
loki store migrate --to "$HOME/Library/CloudStorage/OneDrive-Contoso/LokiProfileManager" --provider onedrive-business --yes --hydrate
loki store migrate --to "$HOME/Library/CloudStorage/OneDrive-Contoso/LokiProfileManager" --provider onedrive-business --yes --copy-only
loki store migrate --to "$HOME/Library/CloudStorage/OneDrive-Contoso/LokiProfileManager" --cleanup
```

### `loki store unset`

```bash
loki store unset [--json]
```

Clears the local persisted store path. It does not delete synced store files.

Examples:

```bash
loki store discover
loki store init ~/OneDrive/loki
loki store status
loki store use ~/OneDrive/loki
loki store migrate --to ~/Dropbox/loki --provider dropbox --dry-run
loki store migrate --to ~/Dropbox/loki --provider dropbox --yes
loki store unset
```

## Profiles, buckets, settings, and skills

Profiles and buckets are store layers, not separate command objects.

- A profile core lives at `profiles/<profile>/core`.
- A bucket lives at `profiles/<profile>/buckets/<bucket>`.
- The first `adopt`, `migrate local`, `migrate repo`, or profile-level `import-skill` into a profile can initialize that layer and write its manifest.
- `import-skill --profile <profile> --bucket <bucket>` requires the parent profile core manifest to exist first.
- An empty profile can also be created manually with `files/`, `skills/`, `templates/`, and `manifest.yaml` containing `files: []` and `skills: []`.
- Machine registration controls allowed activation: `loki machine register --allow-profile <profile> --allow-bucket <bucket>`.
- Skills imported into a layer are validated store assets. Runtime tools see them only when managed settings/files point those tools at the relevant Loki-managed skill directories.

See [`docs/PROFILES.md`](PROFILES.md) for complete from-scratch examples and organization guidance.

## `loki machine register`

Register or update this device in the synced machine registry.

```bash
loki machine register --allow-profile <profile> [--allow-bucket <bucket> ...] [--name <name>] [--active-profile <profile>] [--active-bucket <bucket> ...] [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--name <name>` | Human-readable machine name. Defaults to hostname. |
| `--allow-profile <profile>` | Parent profile this machine may activate. Repeat or comma-separate. Required for a new registration. |
| `--allow-bucket <bucket>` | Bucket this machine may activate. Repeat or comma-separate. |
| `--active-profile <profile>` | Active parent profile to record in the registry. |
| `--active-bucket <bucket>` | Active bucket to record in the registry. Repeat or comma-separate. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Ensures a local machine ID exists.
- Writes or updates `registry/machines.json` for this machine.
- Writes the machine heartbeat under `registry/machines/<machine_id>.json`.
- Preserves existing policy fields when re-running without policy flags.

Examples:

```bash
loki --store /path/to/loki machine register --name "Allen Mac" --allow-profile work
loki --store /path/to/loki machine register --allow-profile work --allow-bucket content-dev --allow-bucket azure
loki --store /path/to/loki machine register --allow-profile work,dev --json
```

## `loki machine status`

Show this device's local machine ID and registry status.

```bash
loki machine status [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Reads the local machine ID if present.
- Reads `registry/machines.json` and reports whether this machine is registered.
- Does not create a machine ID.

Examples:

```bash
loki --store /path/to/loki machine status
loki --store /path/to/loki machine status --json
```

## `loki verify`

Verify a Loki store, manifests, skills, mergeability, and machine policy.

```bash
loki verify [profile] [buckets...] [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Validates required store directories and files.
- If a profile is provided, resolves layers in this order:
  1. `profiles/common/manifest.yaml`
  2. `profiles/<profile>/core/manifest.yaml`
  3. `profiles/<profile>/buckets/<bucket>/manifest.yaml` for each requested bucket.
- If no profile is provided, attempts to use the current machine's active profile from the registry.
- Validates manifest schema, source paths, target expansion, modes, formats, ignore patterns, skill folders, and structured mergeability.
- Enforces machine policy when a machine record is available.
- Warns with `machine.record_missing` and a `loki machine register ...` remediation when a local machine ID exists but no registry record exists.
- Returns a nonzero exit code when blocking issues exist.

Examples:

```bash
loki --store /path/to/loki verify
loki --store /path/to/loki verify work
loki --store /path/to/loki verify work content-dev azure
loki --store /path/to/loki verify work --json
```

## `loki sync`

Resolve local provider conflict-copy files in the Loki store using current-machine-wins semantics.

```bash
loki sync --dry-run [--json]
loki sync --yes [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--dry-run` | Scan and report conflict-copy files without deleting them. |
| `--yes` | Delete detected conflict-copy files and update the machine heartbeat. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Requires exactly one of `--dry-run` or `--yes`.
- Scans only filenames under the Loki store for known OneDrive/Dropbox conflict-copy patterns.
- `--dry-run` validates the store, scans conflict-copy names, reports planned deletions/skips, emits a stable conflict fingerprint, and does not acquire a lock, create a machine ID, update heartbeat, or delete files.
- `--yes` acquires the cooperative store operation lock, rescans conflict-copy names, optionally checks an expected conflict fingerprint for callers such as TUI, ensures a local machine ID, requires that machine to be registered, deletes conflict-copy files, and updates heartbeat.
- Regular files and symlinks with strong provider conflict-copy names are deletable. Broad `case conflict` names, conflict-copy directories, and non-regular filesystem entries are skipped and reported for manual review.
- No provider APIs are called; OneDrive/Dropbox desktop clients still perform actual cloud replication.
- This MVP does not implement watcher capture, pending captured changes, or full bidirectional sync.
- No losing-content backup is created for deleted provider conflict copies.

Examples:

```bash
loki --store /path/to/loki sync --dry-run
loki --store /path/to/loki sync --dry-run --json
loki --store /path/to/loki sync --yes
```

## `loki import-skill`

Import one valid skill folder or `.zip` archive into a Loki store layer.

```bash
loki import-skill <source> (--common | --profile <profile> [--bucket <bucket>]) [--name <store-name>] (--dry-run | --yes) [--overwrite] [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--common` | Import into `profiles/common`. |
| `--profile <profile>` | Import into `profiles/<profile>/core`, unless `--bucket` is also set. |
| `--bucket <bucket>` | Import into `profiles/<profile>/buckets/<bucket>`. The parent profile core manifest must already exist. |
| `--name <store-name>` | Folder name under the layer `skills/` directory. Defaults to the source folder name or zip-derived skill name. |
| `--dry-run` | Validate and show the planned import without writing files. Creates and removes a transient operation lock. |
| `--yes` | Confirm store writes. Required for real import. |
| `--overwrite` | Replace an existing `skills/<store-name>` folder. Without this flag, an existing destination is a blocking conflict. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Requires exactly one of `--dry-run` or `--yes`.
- Requires exactly one destination family: `--common` or `--profile <profile>` with optional `--bucket <bucket>`.
- Validates the source with Loki skill validation (`SKILL.md` frontmatter and local references).
- Accepts an existing skill folder or a `.zip` archive containing `SKILL.md` at archive root or exactly one top-level skill folder.
- Extracts zip archives into a temporary staging directory, normalizes Windows path separators, and rejects path traversal, absolute paths, Windows drive paths, symlink entries, non-regular entries, oversized archives, and ambiguous multi-root archives.
- Rejects symlinks anywhere inside the staged source skill folder.
- Acquires the store operation lock before checking destination conflicts or writing.
- Copies the folder to the selected layer at `skills/<store-name>`.
- Adds or updates the selected layer manifest with `skills: [{source: skills/<store-name>}]`.
- Creates bucket layer folders and manifest when importing into a new bucket under an existing parent profile.
- Does not mirror the skill into Pi, Claude, or other runtime skill directories.
- Does not convert standalone markdown files in this MVP.

Examples:

```bash
loki --store /path/to/loki import-skill ~/skills/my-skill --common --dry-run
loki --store /path/to/loki import-skill ~/Downloads/my-skill.zip --profile work --yes
loki --store /path/to/loki import-skill ~/skills/my-skill --profile work --bucket azure --name cloud-skill --yes
loki --store /path/to/loki import-skill ~/Downloads/my-skill.zip --common --overwrite --yes --json
```

## `loki secrets`

Manage Infisical-backed secret readiness for render templates.

```bash
loki secrets configure infisical
loki secrets --infisical
loki secrets login [--domain <url>]
loki secrets status [--json]
loki secrets check <SECRET_NAME...> [--json]
```

Flags:

| Command | Flag | Description |
|---|---|---|
| `configure infisical` | none | Interactive local-only Universal Auth setup wizard; prompts for project ID, environment, client ID, client secret/key, and optional host URL. |
| `secrets` | `--infisical` | Noninteractive create/update local Infisical env config from existing safe local inputs, then check readiness. |
| `login` | `--domain <url>` | Optional Infisical domain URL for EU Cloud or self-hosted instances. |
| `status` | `--json` | Emit machine-readable JSON. |
| `check` | `--json` | Emit machine-readable JSON. |

Behavior:

- V1 supports Infisical only. Azure Key Vault and other providers are deferred.
- `configure infisical` prompts for Infisical Universal Auth values, validates them with Infisical before writing, and writes `~/.config/infisical/.env` with local-only permissions where supported. If validation fails, Loki leaves any existing env file unchanged. It stores Universal Auth config only, does not persist minted `INFISICAL_TOKEN` values, and then tells you to run `loki secrets status` for readiness verification.
- `--infisical` creates or updates `~/.config/infisical/.env` from existing `INFISICAL_*` environment variables and local `.infisical.json` project config, then runs readiness checks. It remains noninteractive for automation, reports key names only, and never prints values.
- `login` delegates to `infisical login` using inherited terminal I/O. Loki does not capture tokens, passwords, or login output.
- `status` checks that the Infisical CLI is installed and ready for render templates. It does not list or print secret values. If a local machine-auth env file is invalid, `status` reports `machine identity invalid` and tells you to rerun `loki secrets configure infisical` or remove the local env file to fall back to interactive CLI login.
- `check` fetches only the named secrets and reports available or missing names. It never prints values.
- Loki does not store Infisical tokens or secret values in the synced store or local SQLite. The only wizard-written secret-bearing file is the machine-local Infisical env file.
- Render mode reads secret values only during real `switch` execution for files that use `render` mode.
- Machine identity auth is supported through environment variables or `~/.config/infisical/.env`. If `INFISICAL_TOKEN` is set, Loki passes it only to Infisical child processes. If `INFISICAL_AUTH_METHOD=universal-auth` plus `INFISICAL_CLIENT_ID` and `INFISICAL_CLIENT_SECRET` are set, Loki mints a short-lived token through the Infisical Universal Auth API and uses it only for the current operation.
- When machine auth is active and `INFISICAL_PROJECT_ID` is set, Loki passes `--projectId <id>` to Infisical secret reads and `infisical run` calls. This avoids relying on ambient project detection for machine identities.
- Set `INFISICAL_ENV` or `INFISICAL_ENVIRONMENT` to choose a non-`dev` environment. Loki passes `--env <name>` to Infisical secret reads and `infisical run` calls.
- For non-default Infisical hosts, set `INFISICAL_API_URL`, `INFISICAL_HOST`, or legacy `INFISICAL_HOST_URL`. Loki maps `INFISICAL_HOST_URL` to the CLI-supported `INFISICAL_HOST` environment variable for child processes.

Examples:

```bash
loki secrets configure infisical
loki secrets --infisical
loki secrets login
loki secrets login --domain https://eu.infisical.com
loki secrets status
loki secrets status --json
loki secrets check OPENAI_API_KEY GITHUB_TOKEN

# Machine identity shell setup; keep client secret out of committed files.
$env:INFISICAL_AUTH_METHOD = "universal-auth"
$env:INFISICAL_CLIENT_ID = "<client-id>"
$env:INFISICAL_CLIENT_SECRET = "<value>"
$env:INFISICAL_PROJECT_ID = "<project-id>"
$env:INFISICAL_ENV = "dev"
$env:INFISICAL_HOST_URL = "https://app.infisical.com" # optional legacy host alias
loki secrets --infisical
loki secrets status

# Or place the same key-value pairs in ~/.config/infisical/.env for automatic local loading.
```

## `loki doctor`

Inspect local environment, store layout, machine registry, snapshots, operation locks, provider conflict-copy filenames, SQLite state, and Infisical CLI readiness.

```bash
loki doctor [--json]
loki --store /path/to/loki doctor [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |

Behavior:

- Uses `--store` first when provided; otherwise reads the configured store path from local key-value state.
- Does not create local state, a store, machine ID, registry record, snapshot, or target file.
- Opens existing local SQLite state read-only; a missing database is reported as a warning.
- Reports warning-only diagnostics with exit code 0.
- Returns a nonzero exit code when blocking issues exist, such as an invalid configured store layout or SQLite integrity failure.
- Checks local state paths, SQLite integrity and tables, provider discovery, store layout, operation locks, machine registration and stale heartbeats, snapshot metadata, conflict-copy filenames, and Infisical CLI readiness.
- Does not fetch or print secret values and does not read target or snapshot file contents.

Examples:

```bash
loki doctor
loki doctor --json
loki --store /path/to/loki doctor
loki --store /path/to/loki doctor --json
```

## `loki switch`

Activate a profile and optional buckets.

```bash
loki switch <profile> [buckets...] [--dry-run] [--yes] [--capture-local] [--backup-unmanaged]
```

Flags:

| Flag | Description |
|---|---|
| `--dry-run` | Build and safety-check the activation plan without writing target files. |
| `--yes` | Reserve confirmation behavior for future prompts. Does not bypass unsafe overwrite protection by itself. |
| `--capture-local` | Write safe local changes from copied managed targets back to the Loki store before switching. |
| `--backup-unmanaged` | Move unmanaged blocker targets to a machine-local backup directory before switching. Requires `--yes`; does not write those files into the synced store. |

Behavior:

- Requires a configured store path or `--store`.
- Ensures a local machine ID exists.
- Requires this machine to be registered in the store registry before planning or writing.
- Enforces machine policy from `registry/machines.json`.
- Builds an activation plan from the selected profile layers.
- Scans the currently active managed targets for local drift before switching.
- If copied targets changed locally and the store source is unchanged, reports a capture plan. Real switches block until rerun with `--capture-local`; `--capture-local` writes those safe local changes back to the store first.
- Symlink targets need no capture because app writes already land in the store. Render targets are never captured because rendered output can contain secrets; Loki regenerates them from templates during activation. Merge target capture is detected but manual in this MVP.
- If both local target and store source changed since the last switch/adoption, capture blocks as a conflict and requires manual resolution.
- Classifies target safety before writing.
- Blocks unmanaged files, unmanaged directories, broken symlinks, managed hash mismatches, and targets outside the configured home root.
- When unmanaged files/directories are the only blockers and the Loki store should become source of truth, rerun with `--backup-unmanaged --yes`. Loki moves each unmanaged blocker to local state under `unmanaged-backups/<timestamp>/`, writes a `manifest.json`, then switches. Use `loki adopt` instead when the local file should become the store source of truth. Do not adopt rendered secret-bearing files; back them up and let Loki render fresh.
- Creates a local snapshot before real activation writes.
- Executes symlink, copy, structured merge, and render operations. Existing render targets are regenerated even when their local hash drifted, because rendered output is generated and not capturable.
- Removes obsolete managed targets that are no longer part of the active profile after a successful switch. Cleanup only removes missing or unchanged Loki-managed targets; changed obsolete targets block the switch and require manual capture, removal, or adoption.
- Rolls back target files, managed-target DB rows, and active local state if activation fails after snapshot creation and filesystem rollback succeeds.
- Updates local managed target hashes, active profile/buckets, Loki active-profile marker, and machine heartbeat after successful activation.

Examples:

Dry-run:

```bash
loki --store /path/to/loki switch work --dry-run
loki --store /path/to/loki switch work content-dev azure --dry-run
loki --store /path/to/loki switch work content-dev azure --capture-local --dry-run
```

Activate:

```bash
loki --store /path/to/loki switch work --yes
loki --store /path/to/loki switch work content-dev azure --yes
loki --store /path/to/loki switch work content-dev azure --capture-local --yes
loki --store /path/to/loki switch work content-dev azure --backup-unmanaged --yes
```

## `loki snapshots list`

List retained local activation snapshots.

```bash
loki snapshots list [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |

Behavior:

- Reads the machine-local snapshot directory and SQLite snapshot metadata.
- Falls back to `metadata.json` files under the snapshot directory when SQLite has no row.
- Reports snapshot ID, creation time, previous active profile/buckets, target count, target kinds, existence, and local path.
- Does not read or print snapshot entry contents.
- Snapshots are local recovery artifacts; they are not synced into the store.
- Retention currently keeps the latest 2 local snapshot directories.

Examples:

```bash
loki snapshots list
loki snapshots list --json
```

## `loki snapshots show`

Show metadata for one local activation snapshot.

```bash
loki snapshots show <snapshot-id> [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |

Behavior:

- Reads snapshot metadata only.
- Shows previous active profile/buckets and target metadata: path, kind, hashes, expected mode, snapshot entry path, and symlink target when present.
- Shortens hashes in human output.
- Warns when target paths look sensitive, such as `.ssh`, `.env`, token, credential, private, or key paths.
- Does not print file contents from targets or snapshot entries.
- Does not restore files. Use `snapshots restore <id> --dry-run` to preview restore actions before `--yes`.

Examples:

```bash
loki snapshots show 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f
loki snapshots show 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f --json
```

## `loki snapshots restore`

Preview or execute a guarded restore for one local activation snapshot.

```bash
loki snapshots restore <snapshot-id> --dry-run [--json]
loki snapshots restore <snapshot-id> --yes [--json]
loki snapshots restore <snapshot-id> --target <path> --dry-run [--json]
loki snapshots restore <snapshot-id> --target <path> --yes [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--dry-run` | Preview restore actions without writing target files and record a short-lived restore guard. |
| `--yes` | Execute restore after a matching prior dry-run. Mutually exclusive with `--dry-run`. |
| `--target <path>` | Restore only the exact matching snapshot target path. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires exactly one of `--dry-run` or `--yes`.
- `--dry-run` reads snapshot metadata and current target metadata only.
- `--dry-run` computes current hashes for non-sensitive target paths to show whether created targets still match expected hashes.
- `--dry-run` records a 15-minute restore guard only when the plan has no blockers.
- `--yes` requires the guard fingerprint to match the current snapshot, restore scope, target list, hashes, modes, and blockers from the prior dry-run.
- Full `--yes` without `--target` also requires an interactive confirmation phrase: `RESTORE <snapshot-id>`. The prompt is written to stderr, including when `--json` is requested.
- `--yes` creates a new pre-restore snapshot of current target files and local state before any writes.
- Full `--yes` restores target files, symlinks, managed-target DB rows, and active profile/buckets from the selected snapshot.
- `--target` restores only the selected snapshot target and its managed-target row. It does not restore global active profile/buckets and does not prompt for full active-state restore consent.
- `--target` must exactly match a snapshot target path; restoring a child inside a directory snapshot is not supported unless that child was captured as its own target.
- If restore fails after writes begin, Loki rolls back using the pre-restore snapshot and reports that snapshot ID/path for emergency recovery.
- Sensitive-looking target paths such as `.ssh`, `.env`, token, credential, private, `.pem`, or `.key` paths are blocked and redacted by default.
- Never prints target file contents or snapshot entry contents.

Examples:

```bash
loki snapshots restore 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f --dry-run
loki snapshots restore 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f --yes
# Type the exact prompt phrase: RESTORE 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f
loki snapshots restore 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f --target ~/.config/git/ignore --dry-run
loki snapshots restore 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f --target ~/.config/git/ignore --yes
loki snapshots restore 20260504T024936Z-d0f298bf-860a-4c24-91df-91f26681c03f --dry-run --json
```

## `loki adopt`

Capture one existing local target into a profile or bucket and mark it as Loki-managed.

```bash
loki adopt <target> --profile <profile> [--bucket <bucket>] [--mode <mode>] [--source-name <relative-path>] [--dry-run] [--yes] [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--profile <profile>` | Required profile to adopt into. |
| `--bucket <bucket>` | Optional bucket to adopt into instead of profile core. |
| `--mode <mode>` | Adoption mode: `copy`, `symlink`, `merge`, or `render`. When omitted, Loki classifies the target. |
| `--source-name <relative-path>` | Relative source path to use inside the store. When omitted, Loki derives one from the target path. |
| `--dry-run` | Show the adoption plan without changing store files, manifests, or local state. Creates and removes a transient operation lock. |
| `--yes` | Confirm store and local-state writes. Required for real adoption. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Ensures a local machine ID exists for operation-lock metadata.
- Acquires the store operation lock before planning or writing.
- Requires the target to exist under the configured home root.
- Supports valid symlink targets by importing the resolved file or directory while recording the current symlink hash for switch safety.
- Rejects broken symlinks, unsafe source-name paths, unsupported mode/format combinations, and symlink targets with `merge` or `render` mode.
- Copies the current target into the selected store layer.
- Adds or updates the selected layer manifest.
- Writes a local managed-target record so future `switch` can safely update the target when hashes still match.
- Does not rewrite the adopted local target.

Examples:

```bash
loki --store /path/to/loki adopt ~/.gitconfig --profile work --dry-run
loki --store /path/to/loki adopt ~/.gitconfig --profile work --yes
loki --store /path/to/loki adopt ~/.config/app/settings.json --profile work --bucket azure --mode merge --yes
loki --store /path/to/loki adopt ~/Documents/PowerShell/Microsoft.PowerShell_profile.ps1 --profile work --source-name powershell/profile.ps1 --yes
```

## `loki migrate repo`

Import a legacy dotfiles/settings repository into a profile or bucket.

```bash
loki migrate repo <path> --profile <profile> [--bucket <bucket>] [--dry-run] [--yes] [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--profile <profile>` | Required profile to migrate into. |
| `--bucket <bucket>` | Optional bucket to migrate into instead of profile core. |
| `--dry-run` | Show the migration plan without changing store files, manifests, or local state. Creates and removes a transient operation lock. |
| `--yes` | Confirm store and local-state writes. Required for real migration. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Ensures a local machine ID exists for operation-lock metadata.
- Acquires the store operation lock before planning or writing.
- Walks the repo path and skips generated or sensitive paths such as `.git` and private SSH keys.
- Copies supported files, templates, and skills into the selected store layer.
- Adds or updates the selected layer manifest.
- Writes a managed-target record when the imported source matches an existing local target.
- Does not rewrite local targets.

Examples:

```bash
loki --store /path/to/loki migrate repo ~/dotfiles --profile work --dry-run
loki --store /path/to/loki migrate repo ~/dotfiles --profile work --yes
loki --store /path/to/loki migrate repo ~/dotfiles --profile work --bucket azure --yes
```

## `loki migrate local`

Import known settings from the current machine into a profile or bucket.

```bash
loki migrate local --profile <profile> [--bucket <bucket>] [--dry-run] [--yes] [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--profile <profile>` | Required profile to migrate into. |
| `--bucket <bucket>` | Optional bucket to migrate into instead of profile core. |
| `--dry-run` | Show the migration plan without changing store files, manifests, or local state. Creates and removes a transient operation lock. |
| `--yes` | Confirm store and local-state writes. Required for real migration. |
| `--json` | Emit machine-readable JSON. |

Behavior:

- Requires a configured store path or `--store`.
- Ensures a local machine ID exists for operation-lock metadata.
- Acquires the store operation lock before scanning or writing.
- Scans known local targets such as shell profiles, Git config, VS Code settings, PowerShell profiles, SSH config, and local skill folders.
- Supports valid symlink targets by importing the resolved file or directory while recording the current symlink hash for switch safety.
- Copies discovered files and skills into the selected store layer.
- Adds or updates the selected layer manifest.
- Writes managed-target records for imported local targets.
- Does not rewrite local targets.

Examples:

```bash
loki --store /path/to/loki migrate local --profile work --dry-run
loki --store /path/to/loki migrate local --profile work --yes
loki --store /path/to/loki migrate local --profile work --bucket dev-tools --yes
```

## Store operation lock

`switch`, `sync --yes`, `import-skill`, `adopt`, `migrate repo`, and `migrate local` acquire a cooperative store-level lock before writes or write-authoritative planning. `sync --dry-run` intentionally scans without writing a lock:

```text
<store>/.loki-operation.lock
```

Behavior:

- Prevents two local Loki processes from mutating the same store path at the same time.
- Stores operation, machine ID, hostname, PID, acquisition time, expiry time, and a token in JSON.
- Waits up to 10 seconds for an active lock.
- Reports a lock as stale after 30 minutes but does not remove it automatically.
- Removes the lock only when the token still matches, so a late unlock cannot delete a newer lock.
- Works on Windows, macOS, and Linux with `O_CREATE|O_EXCL` file creation.

Cloud-sync note:

- OneDrive, Dropbox, and similar providers are eventual sync transports, not distributed lock services.
- The lock is best-effort across machines and strong only for processes that see the same local store path.
- If a command times out on a lock, remove `<store>/.loki-operation.lock` only after confirming no Loki process is active on any synced machine.

## Activation modes

Manifest file entries support these modes:

| Mode | Behavior |
|---|---|
| `symlink` | Create or replace a symlink from target to source. Does not fall back to copy. |
| `copy` | Copy a source file or directory to the target. |
| `merge` | Deep-merge JSON, YAML, or TOML sources targeting the same file. Later layers win. |
| `render` | Render a template using Infisical-sourced secret values. |

Supported structured formats:

- `json`
- `yaml`
- `toml`
- `text` for non-merge files

## Unsafe overwrite rules

`loki switch` classifies every target before writing:

| Existing target | Result |
|---|---|
| Missing | Safe. |
| Loki-managed symlink | Safe only when a symlink-mode `managed_targets` record matches the link target. |
| Loki-managed file or directory with matching hash | Safe. |
| Loki-managed render target | Safe for render operations; regenerated from the template. |
| Obsolete Loki-managed target with matching hash | Removed during post-switch cleanup and deleted from local state. |
| Obsolete Loki-managed target with changed hash | Blocked; capture, remove, or adopt manually. |
| Unmanaged file | Blocked. |
| Unmanaged directory | Blocked. |
| Broken symlink | Blocked. |
| Loki-managed file or directory with changed hash | Blocked. |
| Target outside configured home root | Blocked during manifest validation. |

Use `loki adopt`, `loki migrate local`, or `loki migrate repo` to capture existing local targets before switching. If the synced store should win and local unmanaged targets are only stale/default install files, use `loki switch <profile> [buckets...] --backup-unmanaged --yes` to move blockers to a local backup and switch in one guarded operation. Real machines with unmanaged dotfiles or settings will block activation until those targets are adopted, migrated, or moved aside/backed up. See [`docs/INSTALL.md#first-run-path-existing-machine-with-profiles-not-migrated`](INSTALL.md#first-run-path-existing-machine-with-profiles-not-migrated) for the full safe migration sequence.

## Template rendering

Render mode supports these placeholders:

```text
{{ SECRET_NAME }}
${SECRET_NAME}
```

Secret values come from the Infisical CLI through an injectable provider. Missing secrets fail activation and list only secret names. Secret values must not appear in logs or errors. Use `loki secrets login`, `loki secrets status`, and `loki secrets check <NAME...>` to prepare and validate Infisical readiness without printing values.

## Exit behavior

- Commands return exit code `0` on success.
- `verify` returns nonzero when blocking issues exist.
- `switch` returns nonzero for invalid profile/bucket selection, missing machine registration, machine policy violations, unsafe targets, merge failures, render failures, write failures, or rollback failures.
