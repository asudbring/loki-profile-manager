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
| `verify` | Implemented |
| `switch` | Implemented |
| `machine register` | Implemented |
| `machine status` | Implemented |
| `adopt` | Implemented |
| `migrate repo` | Implemented |
| `migrate local` | Implemented |

Planned but not implemented:

| Command | Planned purpose |
|---|---|
| `sync` | Handle captures, conflict copies, pending state, and sync checks. |
| `import-skill` | Import skills from folder, zip, or markdown. |
| `doctor` | Inspect environment and report remediation steps. |
| `tui` | Bubble Tea interactive UI. |

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

## `loki switch`

Activate a profile and optional buckets.

```bash
loki switch <profile> [buckets...] [--dry-run] [--yes]
```

Flags:

| Flag | Description |
|---|---|
| `--dry-run` | Build and safety-check the activation plan without writing target files. |
| `--yes` | Reserve confirmation behavior for future prompts. Does not bypass unsafe overwrite protection. |

Behavior:

- Requires a configured store path or `--store`.
- Ensures a local machine ID exists.
- Requires this machine to be registered in the store registry before planning or writing.
- Enforces machine policy from `registry/machines.json`.
- Builds an activation plan from the selected profile layers.
- Classifies target safety before writing.
- Blocks unmanaged files, unmanaged directories, broken symlinks, managed hash mismatches, and targets outside the configured home root.
- Creates a local snapshot before real activation writes.
- Executes symlink, copy, structured merge, and render operations.
- Rolls back target files, managed-target DB rows, and active local state if activation fails after snapshot creation and filesystem rollback succeeds.
- Updates local managed target hashes and machine heartbeat after successful activation.

Examples:

Dry-run:

```bash
loki --store /path/to/loki switch work --dry-run
loki --store /path/to/loki switch work content-dev azure --dry-run
```

Activate:

```bash
loki --store /path/to/loki switch work --yes
loki --store /path/to/loki switch work content-dev azure --yes
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

`switch`, `adopt`, `migrate repo`, and `migrate local` acquire a cooperative store-level lock before planning or writing:

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
| Unmanaged file | Blocked. |
| Unmanaged directory | Blocked. |
| Broken symlink | Blocked. |
| Loki-managed file or directory with changed hash | Blocked. |
| Target outside configured home root | Blocked during manifest validation. |

Use `loki adopt` or `loki migrate local` to capture existing local targets before switching. Real machines with unmanaged dotfiles or settings will block activation until those targets are adopted or moved aside.

## Template rendering

Render mode supports these placeholders:

```text
{{ SECRET_NAME }}
${SECRET_NAME}
```

Secret values come from the Infisical CLI through an injectable provider. Missing secrets fail activation and list only secret names. Secret values must not appear in logs or errors.

## Exit behavior

- Commands return exit code `0` on success.
- `verify` returns nonzero when blocking issues exist.
- `switch` returns nonzero for invalid profile/bucket selection, missing machine registration, machine policy violations, unsafe targets, merge failures, render failures, write failures, or rollback failures.
