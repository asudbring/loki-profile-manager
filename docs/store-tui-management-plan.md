<!-- historical-doc -->

> **Historical note:** Historical store/TUI management plan. Current behavior is documented in README.md, docs/USAGE.md, and docs/ARCHITECTURE.md.

# Store + TUI Management Plan

## Goal

Make Loki usable without remembering `--store`, and make the TUI capable of the same core setup tasks as the CLI:

- Persistently configure local store path.
- Discover/init/use/unset store from CLI and TUI.
- Register/update current machine from CLI and TUI.
- Keep destructive or persistent writes explicit and confirmed.

## Current state

- Global `--store` works, but only for current command.
- `app.Service.EnsureStore` already creates/validates layout and persists `kv_state["store_path"]` when valid.
- No CLI command exposes persistent store configuration directly.
- TUI is mostly read-only for machine/store setup; machine screen cannot register.
- TUI screens call app service through `internal/tui.Client`.

## CLI UX

Add new command group:

```text
loki store <command>
```

### `loki store status`

Show local store configuration.

```bash
loki store status
loki store status --json
loki --store /path/to/store store status
```

Output must distinguish:

- persisted store path from local SQLite
- current `--store` override
- effective path
- effective source: `override`, `persisted`, or `none`
- layout validity and missing paths

### `loki store discover`

List likely store paths.

```bash
loki store discover
loki store discover --manual /path/to/store
loki store discover --json
```

Candidate fields:

- provider: `onedrive`, `dropbox`, `manual`
- provider root path
- candidate store path
- discovery source
- provider root exists
- store path exists
- store layout valid
- missing layout paths, if invalid

No writes.

### `loki store use <path>`

Persist an existing valid store path.

```bash
loki store use "$env:OneDrive\LokiProfileManager"
```

Rules:

- Validates existing layout.
- Does not create directories/files.
- Persists only if valid.
- Fails on missing path or invalid non-empty directory.

### `loki store init <path>`

Create or validate a store, then persist it.

```bash
loki store init "$env:OneDrive\LokiProfileManager"
```

Rules:

- Creates layout when path is missing or empty.
- Accepts existing valid layout.
- Refuses non-empty invalid directory.
- Persists only after layout is valid.

### `loki store unset`

Clear persisted local store path.

```bash
loki store unset
```

Rules:

- Deletes only local SQLite `store_path` key.
- Does not delete synced store files.
- Warns if `--store` override is active.

## App service design

Add typed store-management APIs to `internal/app/service.go`. CLI and TUI must not touch SQLite directly.

```go
type StoreStatusRequest struct{}

type StoreStatusResult struct {
    StoreOverride      string   `json:"store_override,omitempty"`
    PersistedStorePath string   `json:"persisted_store_path,omitempty"`
    EffectiveStorePath string   `json:"effective_store_path,omitempty"`
    EffectiveSource    string   `json:"effective_source"` // override|persisted|none
    LocalStatePath      string   `json:"local_state_path"`
    DatabasePath        string   `json:"database_path"`
    Valid               bool     `json:"valid"`
    Missing             []string `json:"missing,omitempty"`
    Message             string   `json:"message"`
}

type StoreCandidate struct {
    Provider       store.ProviderType `json:"provider"`
    ProviderPath   string             `json:"provider_path"`
    StorePath      string             `json:"store_path"`
    Source         string             `json:"source"`
    ProviderExists bool               `json:"provider_exists"`
    StoreExists    bool               `json:"store_exists"`
    StoreValid     bool               `json:"store_valid"`
    Missing        []string           `json:"missing,omitempty"`
}

type DiscoverStoresResult struct {
    Candidates []StoreCandidate `json:"candidates"`
}

type UseStoreRequest struct {
    StorePath string
}

type ForgetStoreRequest struct{}
```

Methods:

- `StoreStatus(ctx, StoreStatusRequest) (StoreStatusResult, error)`
- `DiscoverStores(ctx, DiscoverStoresRequest) (DiscoverStoresResult, error)`
- `UseStore(ctx, UseStoreRequest) (EnsureStoreResult, error)`
- `EnsureStore(ctx, EnsureStoreRequest) (EnsureStoreResult, error)` existing, used by `store init`
- `ForgetStore(ctx, ForgetStoreRequest) (StoreStatusResult, error)`

Add non-writing inspection to `internal/store/layout.go`:

```go
type InspectionResult struct {
    Exists  bool
    Empty   bool
    IsDir   bool
    Valid   bool
    Missing []string
}

func InspectLayout(root string) (InspectionResult, error)
```

Use this for `store status`, `store discover`, and `store use`.

## TUI UX

Add `ScreenStore` and a new file:

```text
internal/tui/store.go
```

### Dashboard

Add Store action:

```text
[g] Store    configure persistent store
```

Behavior:

- If store is unconfigured, Store action appears first and is selected by default.
- Dashboard help adds `g store`.
- Store summary shows effective path and validity.

### Store screen

Modes:

1. Browse discovered candidates.
2. Manual path entry.
3. Confirm action.
4. Result/error view.

Keys in browse mode:

| Key | Action |
|---|---|
| `↑/↓`, `k/j` | move candidate |
| `d` | rediscover |
| `m` | manual path input |
| `enter` | use valid selected store, or init missing/empty selected store |
| `u` | unset persisted store |
| `r` | reload dashboard |
| `esc` | back |
| `q` | quit |

Manual path mode:

| Key | Action |
|---|---|
| runes | type path |
| backspace | edit |
| enter | inspect path, choose use/init confirmation |
| esc | cancel |

Confirm phrases:

- `USE STORE`
- `INIT STORE`
- `UNSET STORE`

After success:

- Reload dashboard.
- Show success line.
- If process was launched with `--store`, warn that override remains active for this session.

## TUI machine registration

Enhance `ScreenMachine` from read-only to editable.

Add client method:

```go
RegisterMachine(context.Context, app.RegisterMachineRequest) (machine.Record, error)
```

Machine screen modes:

1. Read-only status.
2. Edit registration fields.
3. Confirm registration.
4. Result/error view.

Default values:

- name: current registry display name or hostname/default from app service
- allowed profiles: existing record values, or discovered profiles from profile catalog
- allowed buckets: existing record values, or buckets from catalog
- active profile/buckets: current local status values

Keys:

| Key | Action |
|---|---|
| `e` | edit machine registration |
| `r` | refresh |
| `x` | confirm/register when edit complete |
| `esc` | cancel/back |

Confirmation phrase:

```text
REGISTER MACHINE
```

Write rules:

- Requires configured store.
- Requires at least one allowed profile.
- Reuses current CLI semantics: if field not changed, preserve existing record values.

## Code changes

### CLI

- `internal/cli/root.go`
  - Add `newStoreCommand(...)`.
- `internal/cli/store.go`
  - New command group and human/json printers.
- `internal/cli/store_test.go`
  - New command tests.

### App/store

- `internal/app/service.go`
  - Add store status/use/unset APIs.
  - Enrich discovery result with layout inspection.
- `internal/app/service_test.go`
  - Add app-level store config tests.
- `internal/store/layout.go`
  - Add `InspectLayout`.
- `internal/store/layout_test.go`
  - Add layout inspection tests.

### TUI

- `internal/tui/client.go`
  - Add store + register methods to interface and service adapter.
- `internal/tui/model.go`
  - Add `ScreenStore`, store state, machine edit state, key routing.
- `internal/tui/messages.go`
  - Add store/machine command messages.
- `internal/tui/dashboard.go`
  - Add Store quick action and summary.
- `internal/tui/store.go`
  - New store screen view/update helpers.
- `internal/tui/machine.go`
  - Add edit/register UI.
- `internal/tui/model_test.go`
  - Extend fake client and add store/machine flow tests.

### Docs

- `README.md`
- `docs/USAGE.md`
- `docs/INSTALL.md`
- `docs/TUI_PLAN.md`
- `docs/ARCHITECTURE.md`

## Test plan

### Store package

- Missing root inspection.
- Empty directory inspection.
- Valid layout inspection.
- Non-directory root error.
- Non-empty invalid directory inspection.

### App service

- `StoreStatus` reports no persisted path.
- `EnsureStore` creates layout and persists path.
- `UseStore` persists existing valid layout without creating.
- `UseStore` rejects missing path.
- `UseStore` rejects invalid non-empty path and does not persist.
- `ForgetStore` clears persisted path.
- `StoreStatus` distinguishes persisted path from `--store` override.
- `DiscoverStores` reports candidate validity.

### CLI

- `store status --json` returns valid JSON.
- `store discover --json` includes manual candidate.
- `store init <path>` creates + persists.
- `store use <valid-path>` persists.
- `store use <missing-path>` fails.
- `store unset` clears persisted config.
- root help lists `store`.

### TUI

- Dashboard renders Store action.
- Unconfigured dashboard defaults to Store.
- `g` opens Store screen.
- Store screen renders candidates.
- Candidate navigation wraps.
- Valid candidate confirm calls `UseStore`.
- Missing/empty candidate confirm calls `EnsureStore`.
- Manual input handles runes/backspace/esc.
- Wrong confirmation blocks writes.
- Duplicate enter while busy does nothing.
- Store API errors render without crash.
- Unset confirm calls `ForgetStore`.
- Machine edit confirm calls `RegisterMachine`.
- Machine wrong confirmation blocks registration.

### Manual smoke

```bash
go test ./...
go run ./cmd/loki store status
go run ./cmd/loki store discover
go run ./cmd/loki store init /tmp/loki-smoke
go run ./cmd/loki status
go run ./cmd/loki machine register --allow-profile work
go run ./cmd/loki verify work
go run ./cmd/loki store unset
go run ./cmd/loki tui
```

## Implementation order

1. Add `store.InspectLayout`.
2. Add app store status/use/unset APIs and enrich discovery.
3. Add CLI `loki store` commands.
4. Add docs for CLI path.
5. Add TUI Store screen.
6. Add TUI machine registration flow.
7. Add docs for TUI setup flow.
8. Run full tests and Windows VM smoke.

## Risks

| Risk | Mitigation |
|---|---|
| `--store` override hides persisted config | Store status shows persisted, override, effective source separately. |
| User initializes wrong cloud folder | Separate `use` vs `init`; confirm phrase required in TUI. |
| Non-empty invalid directory mutated | Keep `EnsureLayout` refusal behavior. |
| TUI reload stale after config change | Reload dashboard after use/init/unset/register. |
| Discovery slow on cloud folders | Run discovery only in Store screen/command, not every dashboard load unless cached. |
| Machine registration forms get too complex | Start with text fields/comma-separated profile and bucket lists; later improve multi-select. |
