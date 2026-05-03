# Loki Profile Manager multi-OS handoff — Phase 4 complete, Phase 4.5 needed

Created: 2026-05-03
Repo cwd when written: `C:\Users\allensu\github\loki-profile-manager`

## Purpose

Use this repo-local handoff after cloning the repository on the dev machine with macOS and a Windows VM. This avoids dependence on machine-local `~/.ai-handoff` files.

## Current status

Phase 4 activation engine is implemented and passes Docker validation on the current Windows host.

Implemented areas:

- Activation plan data model and planner.
- Unsafe overwrite detector.
- Symlink operation with Windows remediation text.
- Copy operation for files and directories.
- JSON/YAML/TOML merge writers.
- Infisical CLI wrapper with injectable runner.
- Template renderer for `{{ SECRET_NAME }}` and `${SECRET_NAME}` placeholders.
- Snapshots and retention of last two snapshots.
- Rollback on activation failure.
- App service `Switch(ctx, SwitchRequest)`.
- CLI command `loki switch <profile> [buckets...] [--dry-run] [--yes]`.
- Unit/integration tests for activation, Infisical wrapper, app switch, and CLI switch.

Validation already run with Docker:

```bash
MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/allensu/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go test ./...

MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/allensu/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go vet ./...
```

Docker smoke passed with an empty valid store:

```bash
go run ./cmd/loki --store /tmp/loki-smoke switch work --dry-run
```

Expected output shape:

```text
Loki switch dry-run
Profile: work
Operations: 0
Warning: machine <id> is not registered; policy and heartbeat update skipped
```

## Important realization

Phase 4 can activate an already-populated Loki store, but no bootstrap path exists for real user data.

Without migration/adoption, platform tests are mostly empty-store tests. Existing user settings and skills will be unmanaged local files, so `loki switch` correctly blocks them as unsafe overwrites.

Therefore add **Phase 4.5 — Migration and adoption bootstrap** before serious macOS/Windows dogfood testing.

`tasks-loki-profile-manager.md` has been updated with Phase 4.5.

## Phase 4.5 scope

### Commands to add

```text
loki migrate repo <path> --profile <profile> [--bucket <bucket>] [--dry-run] [--yes]
loki migrate local --profile <profile> [--bucket <bucket>] [--dry-run] [--yes]
loki adopt <target> --profile <profile> [--bucket <bucket>] [--mode copy|symlink|merge|render] [--source-name <name>] [--dry-run] [--yes]
```

### Packages/files likely needed

```text
internal/migration/plan.go
internal/migration/repo.go
internal/migration/local.go
internal/migration/adopt.go
internal/migration/manifest_writer.go
internal/migration/classify.go
internal/migration/skills.go
internal/app/migrate.go
internal/app/adopt.go
internal/cli/migrate.go
internal/cli/adopt.go
```

### Migration behavior

`migrate repo`:

- Scan an existing dotfiles/settings repo.
- Copy selected files into Loki store profile layer.
- Generate or append `manifest.yaml` entries.
- Preserve relative structure under `files/`.
- Default modes by heuristic:
  - stable dotfiles: `symlink`
  - app-mutated settings: `copy`
  - JSON/YAML/TOML settings with duplicate targets: `merge`
  - templates with secret placeholders: `render`

`migrate local`:

- Scan current/injected home for known paths:
  - `~/.pi/agent/skills`
  - `~/.agents/skills`
  - `~/.claude/skills`
  - VS Code `settings.json`, `mcp.json`
  - Copilot `.github/` instructions/prompts
  - `.gitconfig`, shell rc files, PowerShell profile
- Build a plan first.
- Never print secrets.
- Let user confirm before store writes.

`adopt`:

- Copy existing unmanaged target into store.
- Add manifest entry.
- Write `managed_targets` SQLite record with current hash.
- Make subsequent `loki switch` safe if local file hash still matches.
- If local file changes later, switch should block until capture/re-adopt.

## Why adoption matters

Phase 4 overwrite protection is mandatory:

- Missing target: safe.
- Managed hash match: safe.
- Loki-managed symlink: safe.
- Unmanaged file/dir: blocked.
- Broken symlink: blocked.
- Managed hash mismatch: blocked.

Most real machines already have `settings.json`, `.gitconfig`, skill folders, etc. Adoption is the safe bridge from existing local state into Loki ownership.

## Multi-OS validation plan after Phase 4.5

### macOS native

```bash
go test ./...
go vet ./...
go run ./cmd/loki --help
go run ./cmd/loki switch work --dry-run --store <temp-valid-store>
```

Then with migrated fixture:

```bash
go run ./cmd/loki migrate repo <fixture-repo> --store <temp-store> --profile work --dry-run
go run ./cmd/loki migrate repo <fixture-repo> --store <temp-store> --profile work --yes
go run ./cmd/loki switch work --store <temp-store> --dry-run
go run ./cmd/loki switch work --store <temp-store> --yes
```

Check:

- copy target written
- symlink target written or clear permission error
- JSON/YAML/TOML merge output correct
- render works with fake/injected provider in tests
- snapshot created
- rollback test still passes
- no real home modified

### Windows VM native

PowerShell equivalents:

```powershell
go test ./...
go vet ./...
go run ./cmd/loki --help
go run ./cmd/loki switch work --dry-run --store <temp-valid-store>
```

Then with migrated fixture:

```powershell
go run ./cmd/loki migrate repo <fixture-repo> --store <temp-store> --profile work --dry-run
go run ./cmd/loki migrate repo <fixture-repo> --store <temp-store> --profile work --yes
go run ./cmd/loki switch work --store <temp-store> --dry-run
go run ./cmd/loki switch work --store <temp-store> --yes
```

Check:

- `%VAR%`, `${VAR}`, `~`, and target variables expand correctly.
- Symlink succeeds if Developer Mode/admin available.
- Symlink failure message explains Developer Mode/admin and does not fall back to copy.
- Copy/merge/render use temp home only.
- Unsafe overwrite blocks real existing temp-home files until adopted.

## Live Infisical smoke, optional

Only use dummy test secret. Do not print secret values.

Checks:

- Missing CLI gives clear remediation.
- Missing secret lists names only.
- Successful render writes intended target only.
- No logs contain secret values.

## Known implementation caveats from Phase 4

- `verify` does not yet reuse activation safety classifier because current verify path has no SQLite database dependency.
- Rollback restores files and active KV state, but does not restore prior `managed_targets` rows if DB state update fails after file writes. Current executor upserts DB after file operations; future improvement.
- `--yes` currently reserves CLI contract. No prompts exist yet and it does not bypass unsafe overwrite protection.
- Infisical command is currently `infisical run -- printenv NAME` via injectable runner. Verify real CLI behavior on dev machine before relying on live secrets.

## Files changed for Phase 4

```text
internal/activation/copy.go
internal/activation/execute.go
internal/activation/merge.go
internal/activation/merge_json.go
internal/activation/merge_toml.go
internal/activation/merge_yaml.go
internal/activation/operations_test.go
internal/activation/overwrite.go
internal/activation/overwrite_test.go
internal/activation/plan.go
internal/activation/planner.go
internal/activation/planner_test.go
internal/activation/render.go
internal/activation/rollback.go
internal/activation/snapshot.go
internal/activation/snapshot_test.go
internal/activation/symlink.go
internal/activation/write.go
internal/app/service.go
internal/app/switch.go
internal/app/switch_test.go
internal/cli/root.go
internal/cli/switch.go
internal/cli/switch_test.go
internal/infisical/cli.go
internal/infisical/cli_test.go
internal/infisical/render.go
tasks-loki-profile-manager.md
docs/handoffs/multi-os-phase-4.5-handoff.md
```

## Start-here checklist on new machine

1. Clone repo.
2. Open this file.
3. Run native tests.
4. Implement Phase 4.5 migration/adoption.
5. Run migration fixture tests.
6. Run native macOS + Windows VM smoke tests.
7. Only then dogfood against real local config paths.
