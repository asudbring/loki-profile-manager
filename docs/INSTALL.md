# Install

Loki Profile Manager can be installed from a release npm tarball, GitHub release binaries, bundled script installers, or source builds. Homebrew/Scoop/winget formulas do not exist yet.

Requirements for the release npm tarball:

- Node.js 18 or later with npm.
- Access to the private GitHub release asset.
- The downloaded `asudbring-loki-profile-manager-<npm-version>.tgz` file.

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

## npm tarball install

The release npm tarball is the preferred cross-platform installer for dogfood releases. It bundles all supported native Loki binaries and installs a `loki` wrapper on PATH.

Download the `.tgz` asset from the GitHub release, then install it. `<npm-version>` is the release version without the leading `v` (for example, tag `v0.0.0-dogfood.3` produces `asudbring-loki-profile-manager-0.0.0-dogfood.3.tgz`). This package is currently distributed as a GitHub release asset, not through the public npm registry.

```bash
npm install -g ./asudbring-loki-profile-manager-<npm-version>.tgz
loki --version
loki doctor
```

PowerShell uses the same npm command:

```powershell
npm install -g .\asudbring-loki-profile-manager-<npm-version>.tgz
loki --version
loki doctor
```

If Windows cannot find `loki` after npm install, open a new shell and confirm the npm global bin directory is on `PATH`:

```powershell
npm config get prefix
$env:PATH
```

The user npm global bin is usually `%APPDATA%\npm` (for example, `C:\Users\allen\AppData\Roaming\npm`). Node/npm also require `C:\Program Files\nodejs` on `PATH`. The bundled native binary remains under the npm package's `vendor\win32-<arch>` directory, but the normal entry point is the npm shim.

Uninstall removes only the npm wrapper and bundled binary. Loki local state, synced stores, and managed targets are not part of the npm package and are preserved:

```bash
npm uninstall -g @asudbring/loki-profile-manager
```

## Release binary install

Release asset names use this pattern:

```text
asudbring-loki-profile-manager-<npm-version>.tgz
loki_<version>_<os>_<arch>.tar.gz
loki_<version>_windows_<arch>.zip
install.sh
uninstall.sh
install.ps1
uninstall.ps1
release-manifest.json
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

Use the script installers for normal installs. Install path and Loki store path are separate: installers copy the binary and optionally configure a store only when `--store-path` / `-StorePath` is provided.

Linux/macOS script install from downloaded assets:

```bash
chmod +x install.sh uninstall.sh
./install.sh --version <version> \
  --archive ./loki_<version>_<os>_<arch>.tar.gz \
  --checksums ./checksums.txt \
  --install-dir "$HOME/.local/bin"

# Optional store setup; creates a missing/empty store or accepts an existing valid one.
./install.sh --version <version> \
  --archive ./loki_<version>_<os>_<arch>.tar.gz \
  --checksums ./checksums.txt \
  --store-path "$HOME/OneDrive/LokiProfileManager" \
  --force
```

Windows PowerShell script install from downloaded assets:

```powershell
.\install.ps1 `
  -Version <version> `
  -ArchivePath .\loki_<version>_windows_arm64.zip `
  -ChecksumsPath .\checksums.txt `
  -InstallDir "$env:LOCALAPPDATA\Programs\Loki" `
  -AddToPath

# Optional store setup; creates a missing/empty store or accepts an existing valid one.
.\install.ps1 `
  -Version <version> `
  -ArchivePath .\loki_<version>_windows_arm64.zip `
  -ChecksumsPath .\checksums.txt `
  -StorePath "$env:USERPROFILE\OneDrive\LokiProfileManager" `
  -Force
```

Default install paths:

| OS | Default install path | Local state preserved by uninstall |
|---|---|---|
| Windows | `%LOCALAPPDATA%\Programs\Loki` | `%LOCALAPPDATA%\loki-profile-manager` |
| macOS | `$HOME/.local/bin` | `~/Library/Application Support/loki-profile-manager` |
| Linux | `$HOME/.local/bin` | `~/.local/state/loki-profile-manager` |

Uninstall preserves local state, synced stores, managed targets, and Windows Developer Mode by default:

```bash
./uninstall.sh --install-dir "$HOME/.local/bin"
```

```powershell
.\uninstall.ps1 -InstallDir "$env:LOCALAPPDATA\Programs\Loki"
```

Manual archive verification still works.

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

## Troubleshooting

### Windows: `loki` not found after npm install

Npm global installs create a platform shim in the npm global bin directory. If `npm install -g .\asudbring-loki-profile-manager-<npm-version>.tgz` succeeds but `loki --version` fails:

1. Open a new PowerShell or Command Prompt window.
2. Confirm the npm prefix:

   ```powershell
   npm config get prefix
   ```

3. Confirm the prefix directory is on `PATH`. For the default per-user prefix, add `%APPDATA%\npm` (for example, `C:\Users\allen\AppData\Roaming\npm`).
4. Confirm Node.js is on `PATH` (`C:\Program Files\nodejs` for the standard Windows installer).
5. Retry `loki --version` in PowerShell and Command Prompt.

### Windows/OneDrive: `Access is denied` replacing `registry\machines.json`

OneDrive can transiently lock or hydrate store files while Loki updates machine registration. If `loki machine register` fails while replacing `registry\machines.json` with an `Access is denied` error:

1. Wait for OneDrive sync/hydration to settle.
2. Confirm the store folder is available locally.
3. Retry the same `loki machine register` command.
4. If it persists, inspect file attributes and ACLs:

   ```powershell
   $Store = "$env:USERPROFILE\OneDrive\LokiProfileManager"
   attrib "$Store\registry\machines.json"
   Get-Acl "$Store\registry\machines.json"
   ```

A retry after sync settles should preserve existing profile/bucket allowances and update the machine registry cleanly.

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

The store layout can be created by application service code. For a command-line smoke test, initialize a disposable store, register the current machine, then switch.

Linux/macOS shell:

```bash
STORE=$(mktemp -d)/loki
go run ./cmd/loki store init "$STORE"
go run ./cmd/loki machine register --allow-profile work
go run ./cmd/loki verify work
go run ./cmd/loki switch work --dry-run
go run ./cmd/loki --help
go run ./cmd/loki tui --help
```

Expected switch output includes:

```text
Loki switch dry-run
Profile: work
Operations: 0
```

No `machine.record_missing` warning should appear after `machine register` succeeds. For an interactive TUI smoke, run `go run ./cmd/loki` in a real terminal and verify the dashboard opens, Store (`g`) and Machine (`m`) screens open, `r` refreshes, and `q` exits.

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
.\bin\loki.exe store init $Store
.\bin\loki.exe machine register --allow-profile work --allow-bucket content-dev,azure
attrib +P -U "$Store" /S /D
```

Build Loki, initialize or use the store with `loki store init`/`loki store use`, then commands can run without `--store`. Confirm a file written to `$Store` on the VM appears on the other machine and vice versa before testing `adopt`, `migrate`, or `switch`.

## Next install gap

Package-manager installers, code signing, MSI/MSIX, macOS notarization, Homebrew, winget, Scoop, deb, and rpm are not implemented yet. Use release archives plus script installers or source builds.
