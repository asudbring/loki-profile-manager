# Contributing

Loki Profile Manager is a Go CLI with a small npm wrapper and cross-platform installer scripts. Contributions are welcome, but safety matters because Loki reads and writes local configuration files.

## Development setup

Requirements:

- Go 1.23 or later.
- Node.js 18 or later.
- npm.
- PowerShell (`pwsh`) for script parser checks.
- GitHub CLI (`gh`) only for release or repository operations.

Clone and validate:

```bash
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go test ./...
go vet ./...
go mod verify
go build -o loki ./cmd/loki
./loki --help
```

On Windows PowerShell:

```powershell
git clone https://github.com/asudbring/loki-profile-manager.git
cd loki-profile-manager
go test ./...
go vet ./...
go mod verify
go build -o loki.exe ./cmd/loki
.\loki.exe --help
```

## Before opening a pull request

Run the local validation gate:

```bash
go test ./...
go vet ./...
go mod verify
go build -trimpath -o /tmp/loki-pr-check ./cmd/loki
bash -n scripts/install.sh scripts/uninstall.sh scripts/package-release.sh scripts/package-npm.sh scripts/release-local.sh scripts/validate-cross.sh scripts/parallels-windows-admin-probe.sh
node --check npm/bin/loki.js
pwsh -NoProfile -Command '
  $ErrorActionPreference = "Stop"
  $allErrors = @()
  foreach ($file in @("scripts/install.ps1", "scripts/uninstall.ps1", "scripts/windows-installer-smoke.ps1", "scripts/validate-local.ps1")) {
    $errors = $null
    $null = [System.Management.Automation.PSParser]::Tokenize((Get-Content -Raw $file), [ref]$errors)
    if ($errors) { $allErrors += $errors }
  }
  if ($allErrors.Count -gt 0) { $allErrors | Format-List *; exit 1 }
'
```

If you change release packaging, also run:

```bash
./scripts/release-local.sh v0.0.0-pr-test --skip-validation --allow-dirty --out-dir dist/pr-test
rm -rf dist/pr-test
```

## Safety rules

- Do not test against real dotfiles unless the test plan explicitly calls for a low-risk target and uses dry-run first.
- Use temporary stores for integration tests.
- Do not commit generated packages, binaries, SQLite files, local stores, logs, or `.pi/` state.
- Do not commit secrets, tokens, private keys, real secret values, or synced-store contents.
- Use placeholder values in docs and tests.
- Keep unsafe overwrite protection, snapshot restore guards, and path validation conservative.

## Pull request expectations

A good PR includes:

- A focused change.
- Tests for new behavior or bug fixes.
- Documentation updates for user-visible commands, flags, installer behavior, release behavior, or safety rules.
- A short explanation of safety impact when the change touches activation, restore, imports, installers, or secrets.

Avoid unrelated formatting churn. Keep generated files out of the diff.

## Security issues

Do not open public issues that include real credentials, exploit details for active abuse, or private synced-store content. Follow [`SECURITY.md`](SECURITY.md).
