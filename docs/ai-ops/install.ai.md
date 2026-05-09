---
operation: install
target: loki-profile-manager development checkout and validation
prerequisites:
  - tool: git
    version: any
  - tool: go
    version: ">=1.23"
  - account: null
    permissions: []
variables:
  - name: REPO_URL
    description: Git URL for the repository.
    required: false
    default: https://github.com/asudbring/loki-profile-manager.git
    sensitive: false
  - name: WORKDIR
    description: Parent directory where the repository checkout will be created.
    required: true
    default: null
    sensitive: false
  - name: REPO_DIR
    description: Full path to the repository checkout.
    required: false
    default: ${WORKDIR}/loki-profile-manager
    sensitive: false
idempotent: true
estimated_duration: 5-10 minutes
side_effects:
  - creates or updates ${REPO_DIR}
  - downloads Go modules into the Go module cache
  - writes a local build artifact named loki or loki.exe inside ${REPO_DIR}
requires_network: true
requires_sudo: false
---

# Install Loki Profile Manager

This procedure creates a development checkout of `loki-profile-manager`, validates it, builds the CLI binary, and runs a smoke command without touching real user profile targets.

## Prerequisites

- `git` available on PATH.
- `go` 1.23 or later available on PATH.
- Network access to `github.com`.
- Docker only if native Go validation cannot run.

## Variables

| Name | Required | Default | Description |
|---|---|---|---|
| `REPO_URL` | no | `https://github.com/asudbring/loki-profile-manager.git` | Repository URL. |
| `WORKDIR` | yes | `null` | Parent directory for the checkout. |
| `REPO_DIR` | no | `${WORKDIR}/loki-profile-manager` | Checkout directory. |

Resolve `${WORKDIR}` before Step 1. If `${REPO_DIR}` is not provided, set it to `${WORKDIR}/loki-profile-manager`.

## Procedure

## Step 1 — Verify Git access

**Goal**: confirm Git can reach the repository.

**Command**:
```bash
git ls-remote "${REPO_URL}" HEAD
```

**Verify**:
```bash
git ls-remote "${REPO_URL}" HEAD | grep -E '^[0-9a-f]{40}[[:space:]]+HEAD$'
```

**Expected output**: matches `^[0-9a-f]{40}[[:space:]]+HEAD$`

**On failure**: verify network access to `github.com` and confirm `${REPO_URL}` is correct. If using a private fork, authenticate with `gh auth login` or configure Git credentials, then rerun this step. Do not print tokens.

**Idempotent**: true

## Step 2 — Create work directory

**Goal**: ensure `${WORKDIR}` exists.

**Command** (Linux/macOS):
```bash
mkdir -p "${WORKDIR}"
```

**Command** (Windows / PowerShell):
```powershell
New-Item -ItemType Directory -Force -Path $env:WORKDIR | Out-Null
```

**Verify** (Linux/macOS):
```bash
test -d "${WORKDIR}"
```

**Verify** (Windows / PowerShell):
```powershell
Test-Path -Path $env:WORKDIR -PathType Container
```

**On failure**: choose a writable `${WORKDIR}` and rerun this step.

**Idempotent**: true

## Step 3 — Clone or update repository

**Goal**: ensure `${REPO_DIR}` contains the latest `main` checkout.

**Command** (Linux/macOS):
```bash
if [ -d "${REPO_DIR}/.git" ]; then git -C "${REPO_DIR}" pull --ff-only; else git clone "${REPO_URL}" "${REPO_DIR}"; fi
```

**Command** (Windows / PowerShell):
```powershell
if (Test-Path (Join-Path $env:REPO_DIR ".git")) { git -C $env:REPO_DIR pull --ff-only } else { git clone $env:REPO_URL $env:REPO_DIR }
```

**Verify** (Linux/macOS):
```bash
test -f "${REPO_DIR}/go.mod" && test -f "${REPO_DIR}/cmd/loki/main.go"
```

**Verify** (Windows / PowerShell):
```powershell
(Test-Path (Join-Path $env:REPO_DIR "go.mod")) -and (Test-Path (Join-Path $env:REPO_DIR "cmd/loki/main.go"))
```

**On failure**: if the directory exists but is not this repository, move it aside and rerun the clone command.

**Idempotent**: true

## Step 4 — Verify Go toolchain

**Goal**: confirm Go is installed and visible.

**Command**:
```bash
go version
```

**Verify** (Linux/macOS):
```bash
go version | grep -E 'go1\.(23|[3-9][0-9])'
```

**Verify** (Windows / PowerShell):
```powershell
go version | Select-String -Pattern 'go1\.(23|[3-9][0-9])'
```

**Expected output**: contains Go `1.23` or later.

**On failure**: install Go 1.23 or later, reopen the shell, and rerun this step. If native Go cannot be installed, use the Docker validation fallback in Step 8 after confirming Docker is installed.

**Idempotent**: true

## Step 5 — Run unit and integration tests

**Goal**: validate all Go packages.

**Command**:
```bash
go test ./...
```

**Verify**:
```bash
go test ./...
```

**Expected output**: all packages report `ok` or `[no test files]`.

**On failure**: stop. Capture package name and failing test output. Do not proceed to build.

**Idempotent**: true

## Step 6 — Run vet

**Goal**: run Go static checks.

**Command**:
```bash
go vet ./...
```

**Verify**:
```bash
go vet ./...
```

**Expected output**: no output and exit code 0.

**On failure**: stop. Capture vet output.

**Idempotent**: true

## Step 7 — Build binary

**Goal**: create the local Loki CLI binary.

**Command** (Linux/macOS):
```bash
go build -o loki ./cmd/loki
```

**Command** (Windows / PowerShell):
```powershell
go build -o loki.exe ./cmd/loki
```

**Verify** (Linux/macOS):
```bash
./loki --help | grep -F "Manage local profiles"
```

**Verify** (Windows / PowerShell):
```powershell
.\loki.exe --help | Select-String -SimpleMatch "Manage local profiles"
```

**On failure**: rerun Step 5 and Step 6. If they pass, surface the build error.

**Idempotent**: true

## Step 8 — Docker validation fallback

**Goal**: validate the repository when native Go is unavailable or suspect.

**Command** (Linux/macOS):
```bash
docker run --rm -v "${REPO_DIR}:/work" -w /work golang:1.23 go test ./...
```

**Command** (Windows / PowerShell):
```powershell
docker run --rm -v "${env:REPO_DIR}:/work" -w /work golang:1.23 go test ./...
```

**Verify** (Linux/macOS):
```bash
docker run --rm -v "${REPO_DIR}:/work" -w /work golang:1.23 go vet ./...
```

**Verify** (Windows / PowerShell):
```powershell
docker run --rm -v "${env:REPO_DIR}:/work" -w /work golang:1.23 go vet ./...
```

**Expected output**: tests report `ok` and vet exits 0.

**On failure**: if running from Git Bash on Windows, set `MSYS_NO_PATHCONV=1` and rerun from Git Bash. Otherwise surface Docker output.

**Idempotent**: true

## Step 9 — Run status smoke test

**Goal**: confirm the binary can initialize local app state and print status.

**Command** (Linux/macOS):
```bash
./loki status
```

**Command** (Windows / PowerShell):
```powershell
.\loki.exe status
```

**Verify** (Linux/macOS):
```bash
./loki status | grep -F "Loki Profile Manager"
```

**Verify** (Windows / PowerShell):
```powershell
.\loki.exe status | Select-String -SimpleMatch "Loki Profile Manager"
```

**On failure**: inspect local state path permissions. Do not delete real user files.

**Idempotent**: true

## Rollback

Remove the checkout and local build artifact only. Do not remove Go module cache or user-local Loki state unless explicitly requested.

Linux/macOS:

```bash
rm -rf "${REPO_DIR}"
```

Windows PowerShell:

```powershell
Remove-Item -Recurse -Force $env:REPO_DIR
```

## Verification

Linux/macOS:

```bash
test -f "${REPO_DIR}/go.mod" && (cd "${REPO_DIR}" && go test ./... && go vet ./...)
```

Windows PowerShell:

```powershell
Test-Path (Join-Path $env:REPO_DIR "go.mod"); Push-Location $env:REPO_DIR; go test ./...; go vet ./...; Pop-Location
```

A successful install has a valid checkout, passing tests, passing vet, and a working `loki status` command.
