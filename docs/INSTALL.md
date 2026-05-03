# Install

Loki Profile Manager currently installs from source. No release binaries or package-manager formulas exist yet.

Requirements for all platforms:

- Git.
- Go 1.23 or later.
- Access to the private GitHub repository.
- Optional: Docker, for validation on machines without Go.

Supported targets:

- Windows amd64.
- Windows arm64.
- macOS amd64.
- macOS arm64.
- Linux amd64.
- Linux arm64.

OneDrive/Dropbox/iCloud/etc. are the sync transport. Loki must read and write a store path inside the synced folder; Loki does not implement cloud sync itself.

## Windows

Use PowerShell.

```powershell
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go mod verify
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

`go test -race ./...` is not supported by Go on Windows ARM64. On Windows ARM64, use normal `go test ./...`, then prove release compatibility with the cross-compile matrix in `docs/DEVELOPMENT.md` and a native smoke test on the VM.

## macOS

Use zsh or bash.

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go mod verify
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

Linux is a supported runtime target.

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go version
go test ./...
go vet ./...
go mod verify
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

## OneDrive Windows VM smoke

Find the synced OneDrive folder first:

```powershell
$Accounts = Get-ChildItem "HKCU:\Software\Microsoft\OneDrive\Accounts" -ErrorAction SilentlyContinue |
  ForEach-Object { Get-ItemProperty $_.PSPath -ErrorAction SilentlyContinue } |
  Where-Object { $_.UserFolder } |
  Select-Object DisplayName, UserEmail, UserFolder

$OneDrive = ($Accounts | Select-Object -First 1).UserFolder
if (-not $OneDrive) {
  $OneDrive = (Get-ChildItem $env:USERPROFILE -Directory |
    Where-Object { $_.Name -like "OneDrive*" } |
    Select-Object -First 1).FullName
}

$Store = Join-Path $OneDrive "LokiProfileManager"
New-Item -ItemType Directory -Force $Store | Out-Null
attrib +P -U "$Store" /S /D
```

Build Loki, create or verify the store layout, then run every command with `--store $Store`. Confirm a file written to `$Store` on the VM appears on the other machine and vice versa before testing `adopt`, `migrate`, or `switch`.

## Next install gap

Release binaries and package-manager installers are not implemented yet. Install from source until a release workflow exists.
