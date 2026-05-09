# Release

Loki Profile Manager has two release lanes:

1. **Normal public-repo lane** — GitHub Actions validates, packages, smoke-tests installers, and publishes release assets from a `v*` tag.
2. **Local fallback lane** — `scripts/release-local.sh` builds the same asset set locally when Actions is unavailable or when you want to validate packages before pushing a tag.

Release assets include native archives for Linux, macOS, and Windows on amd64/arm64, install/uninstall scripts, a release manifest, checksums, and an npm tarball containing all supported native binaries.

## Normal GitHub Actions release

Use this path after public CI is available.

```bash
git status --short
go test ./...
go vet ./...
go mod verify

git tag v0.1.0
git push origin v0.1.0
```

Tag pushes matching `v*` run `.github/workflows/release.yml`. The workflow:

1. Runs Go tests, vet, module verification, Go build, shell syntax, Node syntax, and PowerShell parser checks.
2. Packages release archives with `scripts/package-release.sh`.
3. Packages the npm tarball with `scripts/package-npm.sh`.
4. Uploads the package set as a workflow artifact.
5. Runs installer smoke tests on Ubuntu, macOS, and Windows.
6. Runs npm global install smoke tests on Ubuntu, macOS, and Windows.
7. Creates or updates the GitHub Release and uploads assets.

Hyphenated versions such as `v0.1.0-beta.1` are prereleases. Plain versions such as `v0.1.0` are stable 0.x releases.

You can also dispatch the workflow manually:

```bash
gh workflow run release.yml -f version=v0.1.0
```

## Local fallback release

Use this lane when GitHub Actions is unavailable or when validating a package set before pushing a tag.

From a clean working tree:

```bash
./scripts/release-local.sh v0.1.0
```

Default behavior:

1. Runs local validation:
   - `go test ./...`
   - `go vet ./...`
   - `go mod verify`
   - `go build -trimpath ./cmd/loki`
   - shell, Node, and PowerShell syntax checks
2. Builds Linux, macOS, and Windows archives for amd64 and arm64.
3. Builds the npm tarball with all native binaries embedded.
4. Verifies `checksums.txt` locally.
5. Stops without creating tags, pushing tags, or uploading anything.

Output goes to `dist/packages` unless `--out-dir <dir>` is set. The output directory is cleaned by the packaging step, so `release-local.sh` only accepts paths under this repo's `dist/` directory, such as `dist/packages` or `dist/manual/v0.1.0`.

## Install local package

The npm tarball is the preferred local installer because it carries every supported native binary.

```bash
npm install -g ./dist/packages/asudbring-loki-profile-manager-0.1.0.tgz
loki --version
```

Expected version:

```text
v0.1.0
```

## Optional GitHub Release upload from local build

If a GitHub Release asset set is useful while Actions is unavailable, rebuild from the current commit and upload:

```bash
./scripts/release-local.sh v0.1.0 --skip-validation --upload
```

`--upload` creates or updates the GitHub Release and uploads `dist/packages/*`. Existing release assets are replaced with `gh release upload --clobber` only if the remote tag for the version points at the current commit. This protects release provenance: assets built from commit A must not replace assets for a tag that points at commit B.

Notes:

- `gh release create` does not require GitHub Actions.
- Creating or pushing a tag can still enqueue tag-triggered workflows.
- Use `--tag` for a local tag only.
- Use `--push-tag` only when you explicitly want the tag on `origin`.

## Manual asset replacement

If the release already exists and you want to replace assets after a validation run from the same commit:

```bash
./scripts/release-local.sh v0.1.0 --skip-validation --upload
```

The script rebuilds packages from the current commit, checks that the remote tag points at that same commit, then uploads with `--clobber`.

## Useful flags

| Flag | Use |
|---|---|
| `--out-dir <dir>` | Write packages somewhere under repo `dist/`; the package step deletes and recreates this directory. |
| `--skip-validation` | Reuse a validation result from the same commit. |
| `--skip-pwsh-syntax` | Skip PowerShell parser checks if `pwsh` is unavailable. |
| `--allow-dirty` | Permit packaging from a dirty tree. Avoid for releases. |
| `--tag` | Create a local tag after successful packaging. |
| `--push-tag` | Push the tag to `origin`; implies `--tag`. |
| `--upload` | Create/update GitHub Release assets. |
| `--notes-file <file>` | Use custom release notes for `--upload`. |

## Dogfood checklist

Run at least these checks before trusting a new dogfood release.

### macOS

```bash
loki --version
loki store status
loki verify dev frontend backend --json
loki verify work content-dev azure --json
loki verify writer drafting beta-reading editing originality publishing --json
```

Each verify should report:

```text
valid=true, blocking=0, warnings=0, info=0
```

### Windows

Use the installed `loki` from a normal user shell, not a SYSTEM automation shell unless that is the scenario under test.

```powershell
loki --version
loki store status
loki verify dev frontend backend --json
loki verify work content-dev azure --json
loki verify writer drafting beta-reading editing originality publishing --json
```

Each verify should report:

```text
valid=true, blocking=0, warnings=0, info=0
```

### Windows-created zip import smoke

Create a skill folder on Windows, zip it with `Compress-Archive`, then import it into a disposable store:

```powershell
$Root = Join-Path $env:TEMP ("loki-zip-smoke-" + [guid]::NewGuid().ToString("N"))
$Store = Join-Path $Root "store"
$Skill = Join-Path $Root "windows-zip-smoke"
$Zip = Join-Path $Root "windows-zip-smoke.zip"
New-Item -ItemType Directory -Force -Path $Skill | Out-Null
@'
---
name: windows-zip-smoke
description: Windows-created zip smoke
---
# windows-zip-smoke

See [notes](notes.md).
'@ | Set-Content -LiteralPath (Join-Path $Skill "SKILL.md") -Encoding utf8
Set-Content -LiteralPath (Join-Path $Skill "notes.md") -Value "WINDOWS_ZIP_IMPORT_OK" -Encoding utf8
Compress-Archive -LiteralPath $Skill -DestinationPath $Zip -Force

loki store init $Store
loki --store $Store machine register --allow-profile work --active-profile work
loki --store $Store import-skill $Zip --common --dry-run --json
loki --store $Store import-skill $Zip --common --yes --json
loki --store $Store verify work --json
Get-Content -Raw (Join-Path $Store "profiles\common\skills\windows-zip-smoke\notes.md")
```

Expected marker:

```text
WINDOWS_ZIP_IMPORT_OK
```

## Stable release gate

Do not promote a stable tag until:

- Public CI is green on Ubuntu, macOS, and Windows.
- Release workflow installer and npm smoke tests pass.
- Current-tree secret and personal-information scans pass.
- At least one dogfood prerelease has passed macOS and Windows final-store verification.
