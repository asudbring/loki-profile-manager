# Development

This repo is a Go 1.23 CLI project. The code is organized around a thin Cobra CLI, an app service layer, and internal packages for store layout, manifests, machine registry, verification, activation, and Infisical rendering.

## Package map

| Path | Responsibility |
|---|---|
| `cmd/loki` | Binary entry point. |
| `internal/cli` | Cobra commands and terminal output. |
| `internal/app` | Service orchestration, local paths, database, logging, and command workflows. |
| `internal/config` | OS-specific local path resolution and injectable test paths. |
| `internal/db` | SQLite bootstrap, migrations, and key-value helpers. |
| `internal/log` | Structured logging and redaction. |
| `internal/store` | Store discovery, layout creation, and validation. |
| `internal/machine` | Machine ID, registry, heartbeat, and policy validation. |
| `internal/manifest` | YAML schema, parsing, validation, path expansion, and merge dry-run. |
| `internal/profile` | Profile layer resolution. |
| `internal/skills` | Skill folder validation. |
| `internal/verify` | Store, manifest, skill, and policy verification reports. |
| `internal/activation` | Activation planning, safety, symlink/copy/merge/render, snapshots, rollback, and managed target state. |
| `internal/infisical` | Infisical CLI wrapper and template renderer. |

## Test commands

Native Go:

```bash
go test ./...
go vet ./...
```

Docker:

```bash
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go test ./...
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go vet ./...
```

Git Bash for Windows:

```bash
MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/allensu/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go test ./...

MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/allensu/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go vet ./...
```

## Build commands

Linux/macOS:

```bash
go build -o loki ./cmd/loki
./loki --help
```

Windows PowerShell:

```powershell
go build -o loki.exe ./cmd/loki
.\loki.exe --help
```

## Testing rules

- Do not test against the real user home directory.
- Use `config.PathResolver` with temp directories for OS-specific path tests.
- Use temp Loki stores for integration tests.
- Do not put secrets in fixtures, logs, or expected output.
- Do not call real Infisical in unit tests; use an injectable fake runner or fake secret provider.
- Use Docker when the host does not have Go installed.
- Test Windows path expansion and symlink behavior on a Windows VM before dogfooding.
- Test macOS symlink and local-state paths on a macOS host before dogfooding.

## Local state during tests

`app.NewService` opens a SQLite database under the resolved local app state path. Tests should pass an injected resolver such as:

```go
config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()}
```

or Windows-style:

```go
config.PathResolver{
    GOOS: "windows",
    HomeDir: t.TempDir(),
    LocalAppData: filepath.Join(t.TempDir(), "AppData", "Local"),
}
```

## Store fixtures

Use `store.EnsureLayout(tempDir)` to create a valid store. Base manifests contain no files or skills. Tests that need activation targets must write source files and update the relevant `manifest.yaml`.

Minimum activation layers for profile `work`:

```text
profiles/common/manifest.yaml
profiles/work/core/manifest.yaml
```

A bucket such as `azure` also requires:

```text
profiles/work/buckets/azure/manifest.yaml
```

## Secret handling

Secrets must not appear in code, fixtures, docs, logs, or terminal output.

For Infisical-related tests:

- Use `internal/infisical.Runner` fakes.
- Use `activation.SecretProvider` fakes.
- Assert missing-secret errors contain secret names only.
- Use dummy values that are never real credentials.

## Current implementation phase

Phases 1-4 are implemented. Phase 4.5 is next and must add migration/adoption before real machine dogfooding.

Relevant handoff:

```text
docs/handoffs/multi-os-phase-4.5-handoff.md
```

## Before committing

Run:

```bash
go test ./...
go vet ./...
```

If Go is unavailable on the host, run the Docker equivalents.
