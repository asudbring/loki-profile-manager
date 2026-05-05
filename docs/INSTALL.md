# Install

Loki Profile Manager can be installed from GitHub release binaries or built from source. Package-manager formulas/installers do not exist yet.

Requirements for release binaries:

- Access to the private GitHub repository.
- A downloaded release archive for your OS/architecture.
- `checksums.txt` from the same release.

Requirements for source builds:

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

## Release binary install

Release asset names use this pattern:

```text
loki_<version>_<os>_<arch>.tar.gz
loki_<version>_windows_<arch>.zip
checksums.txt
```

Supported assets:

| OS | Arch | Archive |
|---|---|---|
| Linux | amd64 | `tar.gz` |
| Linux | arm64 | `tar.gz` |
| macOS | amd64 | `tar.gz` |
| macOS | arm64 | `tar.gz` |
| Windows | amd64 | `zip` |
| Windows | arm64 | `zip` |

Verify checksums before running the binary.

Linux/macOS, selected archive only:

```bash
grep " loki_<version>_<os>_<arch>.tar.gz$" checksums.txt | shasum -a 256 -c -
# or, where available:
grep " loki_<version>_<os>_<arch>.tar.gz$" checksums.txt | sha256sum -c -
```

If every archive is downloaded, verify all entries:

```bash
shasum -a 256 -c checksums.txt
```

Windows PowerShell, selected archive only:

```powershell
$Archive = "loki_<version>_windows_arm64.zip"
$Expected = (Get-Content .\checksums.txt | Where-Object { $_ -like "* $Archive" }).Split()[0]
$Actual = (Get-FileHash .\$Archive -Algorithm SHA256).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) { throw "checksum mismatch" }
```

After extraction:

```bash
loki --version
loki doctor
loki tui --help
```

Windows PowerShell:

```powershell
.\loki.exe --version
.\loki.exe doctor
.\loki.exe tui --help
```

## Windows source build

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

## macOS source build

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
go run ./cmd/loki --store /path/to/loki machine register --allow-profile work
go run ./cmd/loki --store /path/to/loki switch work --dry-run
```

## Linux source build

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

The store layout can be created by application service code. For a command-line smoke test, create the minimal layout manually or use an existing fixture store, then register the current machine before switching.

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
go run ./cmd/loki --store "$STORE" machine register --allow-profile work
go run ./cmd/loki --store "$STORE" verify work
go run ./cmd/loki --store "$STORE" switch work --dry-run
go run ./cmd/loki --store "$STORE" tui --help
```

Expected switch output includes:

```text
Loki switch dry-run
Profile: work
Operations: 0
```

No `machine.record_missing` warning should appear after `machine register` succeeds. For an interactive TUI smoke, run `go run ./cmd/loki --store "$STORE" tui` in a real terminal and verify the dashboard opens, `r` refreshes, and `q` exits.

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

Package-manager installers, code signing, and notarization are not implemented yet. Use release archives or source builds until installer workflows exist.
