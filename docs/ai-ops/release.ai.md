---
operation: deploy
target: loki-profile-manager release asset set
prerequisites:
  - tool: git
    version: any
  - tool: go
    version: ">=1.23"
  - tool: node
    version: ">=18"
  - tool: npm
    version: any
  - tool: pwsh
    version: any
  - tool: gh
    version: any
variables:
  - name: VERSION
    description: Release version tag, for example v0.1.0.
    required: true
    default: null
    sensitive: false
  - name: OUT_DIR
    description: Local package output directory under repo dist/.
    required: false
    default: dist/packages
    sensitive: false
  - name: UPLOAD
    description: Whether to upload assets to GitHub Release with scripts/release-local.sh --upload.
    required: false
    default: "false"
    sensitive: false
idempotent: false
estimated_duration: 10-20 minutes
side_effects:
  - deletes and recreates ${OUT_DIR} when ${OUT_DIR} is under dist/
  - may create or update a local git tag when upload is requested
  - may create or update GitHub Release assets when ${UPLOAD}=true
requires_network: true
requires_sudo: false
---

# Release Loki Profile Manager

This procedure validates the current checkout, builds a local release asset set, verifies checksums, and optionally uploads assets to a GitHub Release. It does not make the repository public and does not publish to npm.

## Prerequisites

- `git`, `go`, `node`, `npm`, `pwsh`, and `gh` are on PATH.
- The current working directory is the repository root.
- `${VERSION}` is a semver tag starting with `v`.
- `${OUT_DIR}` resolves under repository `dist/`.
- If `${UPLOAD}=true`, GitHub CLI is authenticated with permission to create releases and upload assets.

## Variables

| Name | Required | Default | Description |
|---|---|---|---|
| `VERSION` | yes | `null` | Release version tag, for example `v0.1.0`. |
| `OUT_DIR` | no | `dist/packages` | Package output directory under repo `dist/`. |
| `UPLOAD` | no | `false` | Set to `true` to upload assets to GitHub Release. |

Resolve `${VERSION}` before Step 1. Abort if it is empty.

## Procedure

## Step 1 — Verify clean working tree

**Goal**: prevent accidental release from uncommitted files.

**Command**:
```bash
git status --short
```

**Verify**:
```bash
test -z "$(git status --short)"
```

**Expected output**: no output.

**On failure**: commit, stash, or intentionally abandon local changes. Do not use `--allow-dirty` for a public release.

**Idempotent**: true

## Step 2 — Validate release version format

**Goal**: ensure `${VERSION}` is accepted by release packaging.

**Command**:
```bash
case "${VERSION}" in v[0-9]*.[0-9]*.[0-9]*) exit 0 ;; *) echo "invalid VERSION: ${VERSION}" >&2; exit 1 ;; esac
```

**Verify**:
```bash
case "${VERSION}" in v[0-9]*.[0-9]*.[0-9]*) exit 0 ;; *) exit 1 ;; esac
```

**On failure**: choose a semver tag such as `v0.1.0`.

**Idempotent**: true

## Step 3 — Run local validation

**Goal**: prove tests, vet, module verification, build, and script syntax pass before packaging.

**Command**:
```bash
go test ./... && go vet ./... && go mod verify && go build -trimpath -o /tmp/loki-release-check ./cmd/loki && bash -n scripts/install.sh scripts/uninstall.sh scripts/package-release.sh scripts/package-npm.sh scripts/release-local.sh scripts/validate-cross.sh scripts/parallels-windows-admin-probe.sh && node --check npm/bin/loki.js && pwsh -NoProfile -Command '$ErrorActionPreference="Stop"; $allErrors=@(); foreach ($file in @("scripts/install.ps1","scripts/uninstall.ps1","scripts/windows-installer-smoke.ps1","scripts/validate-local.ps1")) { $errors=$null; $null=[System.Management.Automation.PSParser]::Tokenize((Get-Content -Raw $file), [ref]$errors); if ($errors) { $allErrors += $errors } }; if ($allErrors.Count -gt 0) { $allErrors | Format-List *; exit 1 }'
```

**Verify**:
```bash
test -x /tmp/loki-release-check || test -f /tmp/loki-release-check
```

**On failure**: fix the failing test, vet issue, build issue, or syntax error before packaging.

**Idempotent**: true

## Step 4 — Build local release assets

**Goal**: produce release archives, scripts, manifest, checksums, and npm tarball under `${OUT_DIR}`.

**Command**:
```bash
./scripts/release-local.sh "${VERSION}" --skip-validation --out-dir "${OUT_DIR}"
```

**Verify**:
```bash
test -f "${OUT_DIR}/checksums.txt" && test -f "${OUT_DIR}/release-manifest.json" && test -f "${OUT_DIR}/asudbring-loki-profile-manager-${VERSION#v}.tgz"
```

**On failure**: ensure `${OUT_DIR}` is under `dist/`, ensure Go/Node/npm are installed, then rerun Step 4.

**Idempotent**: false

## Step 5 — Verify checksums

**Goal**: prove every generated asset matches `checksums.txt`.

**Command**:
```bash
if command -v sha256sum >/dev/null 2>&1; then (cd "${OUT_DIR}" && sha256sum -c checksums.txt); else (cd "${OUT_DIR}" && shasum -a 256 -c checksums.txt); fi
```

**Verify**:
```bash
if command -v sha256sum >/dev/null 2>&1; then (cd "${OUT_DIR}" && sha256sum -c checksums.txt >/dev/null); else (cd "${OUT_DIR}" && shasum -a 256 -c checksums.txt >/dev/null); fi
```

**On failure**: delete `${OUT_DIR}` and rerun Step 4.

**Idempotent**: true

## Step 6 — Optionally upload GitHub Release assets

**Goal**: create or update the GitHub Release only when requested.

**Command**:
```bash
if [ "${UPLOAD}" = "true" ]; then ./scripts/release-local.sh "${VERSION}" --skip-validation --out-dir "${OUT_DIR}" --upload; else echo "UPLOAD=false; skipping GitHub Release upload"; fi
```

**Verify**:
```bash
if [ "${UPLOAD}" = "true" ]; then gh release view "${VERSION}" --json tagName,assets >/dev/null; else test "${UPLOAD}" = "false"; fi
```

**On failure**: run `gh auth status`. If authentication is valid, check whether the remote tag for `${VERSION}` points at the current commit; `release-local.sh` refuses to clobber assets when tag provenance does not match.

**Idempotent**: false

## Rollback

Local package generation can be removed with:

```bash
rm -rf "${OUT_DIR}"
```

If `${UPLOAD}=true` created a GitHub Release unintentionally, delete it manually after confirming no consumers depend on it:

```bash
gh release delete "${VERSION}" --yes
```

Do not delete a published tag or release if any user has already installed it without first communicating the replacement.

## Verification

Run this end-to-end verification after packaging:

```bash
if command -v sha256sum >/dev/null 2>&1; then (cd "${OUT_DIR}" && sha256sum -c checksums.txt >/dev/null); else (cd "${OUT_DIR}" && shasum -a 256 -c checksums.txt >/dev/null); fi
npm install -g "${OUT_DIR}/asudbring-loki-profile-manager-${VERSION#v}.tgz"
loki --version | grep -F "${VERSION}"
npm uninstall -g @asudbring/loki-profile-manager
```
