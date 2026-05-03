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

Planned but not implemented:

| Command | Planned purpose |
|---|---|
| `migrate` | Import existing repo or local config into a Loki store. |
| `adopt` | Mark an existing local target as Loki-managed. |
| `sync` | Handle captures, conflict copies, pending state, and sync checks. |
| `import-skill` | Import skills from folder, zip, or markdown. |
| `doctor` | Inspect environment and report remediation steps. |
| `tui` | Bubble Tea interactive UI. |

## `loki status`

Show local Loki status.

```bash
loki status [--json]
```

Flags:

| Flag | Description |
|---|---|
| `--json` | Emit machine-readable JSON. |

Behavior:

- Opens the local SQLite state database.
- Reports the local state directory and database path.
- Uses `--store` first when provided.
- Otherwise reads the configured store path from local key-value state.
- Validates the store layout if a store path is configured.

Examples:

```bash
loki status
loki status --json
loki --store /path/to/loki status
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
- Enforces machine policy if the machine is registered in the store registry.
- Allows the switch with a warning if the machine is not registered. This keeps fixture and bootstrap stores usable before setup exists.
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

The Phase 4.5 `adopt` command is planned to make existing local targets manageable without data loss. Until then, real machines with existing dotfiles or settings will often block activation correctly.

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
- `switch` returns nonzero for invalid profile/bucket selection, machine policy violations, unsafe targets, merge failures, render failures, write failures, or rollback failures.
