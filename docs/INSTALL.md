# Install

Loki Profile Manager currently installs from source. No release binaries or package-manager formulas exist yet.

Requirements for all platforms:

- Git.
- Go 1.23 or later.
- Access to the private GitHub repository.
- Optional: Docker, for validation on machines without Go.

## Windows

Use PowerShell.

```powershell
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go build -o loki.exe ./cmd/loki
.\loki.exe --help
```

Run status:

```powershell
.\loki.exe status
```

Run a dry-run switch against a valid temp store:

```powershell
$store = Join-Path $env:TEMP "loki-smoke"
Remove-Item -Recurse -Force $store -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $store | Out-Null
go run ./cmd/loki --store $store status
```

`status` can run against an empty or invalid store. `verify` and `switch` require a valid Loki store layout.

Windows symlink behavior depends on Developer Mode or elevated permissions. Loki returns a remediation error if symlink creation is denied and does not fall back to copy.

## macOS

Use zsh or bash.

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go build -o loki ./cmd/loki
./loki --help
```

Run status:

```bash
./loki status
```

Run a dry-run switch against a valid temp store once one exists:

```bash
go run ./cmd/loki --store /path/to/loki switch work --dry-run
```

## Linux

Linux is supported for development and CI even though Windows and macOS are the primary target platforms.

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go build -o loki ./cmd/loki
./loki --help
```

Run status:

```bash
./loki status
```

## Docker validation

Use Docker when the host does not have Go installed.

Linux/macOS shell:

```bash
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go test ./...
docker run --rm -v "$PWD:/work" -w /work golang:1.23 go vet ./...
```

Git Bash for Windows from this repository path:

```bash
MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/allensu/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go test ./...

MSYS_NO_PATHCONV=1 docker run --rm \
  -v "C:/Users/allensu/github/loki-profile-manager:/work" \
  -w /work golang:1.23 go vet ./...
```

## Smoke test with a valid store

The store layout can be created by application service code, but no setup CLI exists yet. For a command-line smoke test, create the minimal layout manually or use an existing fixture store.

Linux/macOS shell:

```bash
STORE=$(mktemp -d)/loki
mkdir -p "$STORE"/registry/machines "$STORE"/profiles/common/{files,skills,templates} "$STORE"/profiles/work/core/{files,skills,templates} "$STORE"/profiles/work/buckets "$STORE"/profiles/dev/core/{files,skills,templates} "$STORE"/profiles/dev/buckets "$STORE"/profiles/writer/core/{files,skills,templates} "$STORE"/profiles/writer/buckets "$STORE"/conflicts "$STORE"/snapshots "$STORE"/logs
printf '{"version":1,"machines":[]}' > "$STORE/registry/machines.json"
for path in common work/core dev/core writer/core; do
  name=$(basename "$path")
  cat > "$STORE/profiles/$path/manifest.yaml" <<EOF
version: 1
name: $name
files: []
skills: []
ignore: []
merge_rules: {}
targets: {}
EOF
done
go run ./cmd/loki --store "$STORE" verify work
go run ./cmd/loki --store "$STORE" switch work --dry-run
```

Expected switch output includes:

```text
Loki switch dry-run
Profile: work
Operations: 0
```

An unregistered-machine warning is expected for a manually created smoke store.

## Next install gap

Phase 4.5 must add migration/adoption commands before real dogfood installs are useful on existing machines. Existing `.gitconfig`, VS Code settings, and skill folders are unmanaged files today, so `loki switch` blocks them by design.
