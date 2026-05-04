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
go test ./internal/activation ./internal/app ./internal/cli
go test ./...
go vet ./...
go mod verify
go build ./cmd/loki
```

Race tests where supported:

```bash
go test -race ./...
```

`go test -race` is not supported by Go on every target. In particular, Windows ARM64 currently must use normal `go test ./...`; prove Windows ARM64 with native tests plus the cross-compile matrix below.

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

## Cross-platform matrix

Supported targets:

| OS | Architectures | Validation |
|---|---|---|
| Windows | amd64, arm64 | Native `go test`; cross-compile both; skip `-race` on arm64. |
| macOS | amd64, arm64 | Native `go test`; cross-compile both. |
| Linux | amd64, arm64 | Native `go test`; cross-compile both. |

Cross-compile all release targets from any host with Go installed:

```bash
for target in windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
  GOOS=${target%/*}
  GOARCH=${target#*/}
  ext=""
  if [ "$GOOS" = "windows" ]; then ext=".exe"; fi
  CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -o "dist/loki-$GOOS-$GOARCH$ext" ./cmd/loki
done
```

Compile package tests for all targets:

```bash
for target in windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
  GOOS=${target%/*}
  GOARCH=${target#*/}
  ext=""
  if [ "$GOOS" = "windows" ]; then ext=".exe"; fi
  for pkg in $(go list ./...); do
    name=$(printf '%s' "$pkg" | tr '/.' '__')
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go test -c -o ".testbin/$GOOS-$GOARCH-$name$ext" "$pkg"
  done
done
```

GitHub Actions runs native tests and cross-compilation in `.github/workflows/ci.yml`.

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
- For rollback hardening, validate `loki snapshots list`, `loki snapshots show <id>`, and `loki snapshots restore <id> --dry-run` before adding any command that writes restored files.

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

Phases 1-4.5 are implemented. Current hardening focuses on real low-risk dotfile dogfood, machine registration, status audit, read-only snapshot reporting, and restore dry-run preview. Real manual snapshot restore remains deferred until read-only UX is proven on macOS and Windows.

Relevant historical handoff:

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
