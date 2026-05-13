---
operation: install
target: loki-profile-manager CLI plus optional first-run Loki store registration and migration
prerequisites:
  - tool: node
    version: ">=18"
  - tool: npm
    version: any
  - tool: git
    version: any
  - tool: go
    version: ">=1.23 when INSTALL_METHOD=source"
  - account: sync-provider
    permissions:
      - read and write access to ${STORE_PATH} when RUN_FIRST_RUN=true
variables:
  - name: INSTALL_METHOD
    description: install method, one of npm or source
    required: false
    default: npm
    sensitive: false
  - name: REPO_URL
    description: Git URL for source validation or source install
    required: false
    default: https://github.com/asudbring/loki-profile-manager.git
    sensitive: false
  - name: WORKDIR
    description: parent directory for source checkout when INSTALL_METHOD=source
    required: false
    default: null
    sensitive: false
  - name: REPO_DIR
    description: full checkout path when INSTALL_METHOD=source
    required: false
    default: ${WORKDIR}/loki-profile-manager
    sensitive: false
  - name: STORE_PATH
    description: Loki store path inside a synced folder
    required: false
    default: null
    sensitive: false
  - name: PROFILE
    description: profile to register, verify, migrate, and switch
    required: false
    default: work
    sensitive: false
  - name: BUCKETS
    description: space-separated bucket list for first-run commands
    required: false
    default: ""
    sensitive: false
  - name: MACHINE_NAME
    description: machine display name for Loki registry
    required: false
    default: host name
    sensitive: false
  - name: RUN_FIRST_RUN
    description: whether to configure store and register machine after install
    required: false
    default: "false"
    sensitive: false
  - name: LOKI_EXE
    description: resolved Loki executable path produced by Step 4; defaults to loki on PATH for npm installs
    required: false
    default: loki
    sensitive: false
  - name: MACHINE_SCENARIO
    description: first-run scenario, one of fresh or existing
    required: false
    default: fresh
    sensitive: false
  - name: LEGACY_REPO
    description: optional legacy profile repository path for migrate repo
    required: false
    default: null
    sensitive: false
  - name: INIT_EMPTY_PROFILE
    description: create a minimal empty profile manifest when the store has no profile core yet
    required: false
    default: "false"
    sensitive: false
idempotent: true
estimated_duration: 5-30 minutes
side_effects:
  - installs or updates a global npm package when INSTALL_METHOD=npm
  - creates or updates ${REPO_DIR} when INSTALL_METHOD=source
  - downloads npm or Go dependencies
  - creates or updates local Loki state
  - creates or updates ${STORE_PATH} when RUN_FIRST_RUN=true and store init/use/register/migrate commands run
  - can capture local profile files into the store when MACHINE_SCENARIO=existing and migration --yes steps run
requires_network: true
requires_sudo: false
---

# Install Loki Profile Manager

This procedure installs the `loki` CLI and, when requested, performs a safe first-run setup against a synced Loki store. It supports a fresh machine path and an existing-machine migration path. It never prints secret values. Execute the procedure in one shell session so variables exported in earlier steps remain available to later steps.

## Prerequisites

- `node` 18 or later and `npm` available on PATH for `INSTALL_METHOD=npm`.
- `git` and Go 1.23 or later for `INSTALL_METHOD=source`.
- A synced filesystem folder available at `${STORE_PATH}` when `${RUN_FIRST_RUN}` is `true`.
- No secret values in command output, logs, or chat.

## Variables

| Name | Required | Default | Description |
|---|---|---|---|
| `INSTALL_METHOD` | no | `npm` | `npm` or `source`. |
| `REPO_URL` | no | `https://github.com/asudbring/loki-profile-manager.git` | Source repository URL. |
| `WORKDIR` | only for source | `null` | Parent directory for checkout. |
| `REPO_DIR` | only for source | `${WORKDIR}/loki-profile-manager` | Checkout directory. |
| `STORE_PATH` | only for first-run | `null` | Synced Loki store path. |
| `PROFILE` | no | `work` | Profile name. |
| `BUCKETS` | no | empty | Space-separated bucket names. |
| `MACHINE_NAME` | no | host name | Machine registry name. |
| `RUN_FIRST_RUN` | no | `false` | Set `true` to configure store and register machine. |
| `LOKI_EXE` | no | `loki` | Resolved Loki executable path. Step 4 sets this for source installs. |
| `MACHINE_SCENARIO` | no | `fresh` | `fresh` or `existing`. |
| `LEGACY_REPO` | no | `null` | Optional legacy profile repository path. |
| `INIT_EMPTY_PROFILE` | no | `false` | Set `true` to create an empty profile core before registration/verify. |

For profile naming, bucket layout, skill organization, and manual manifest examples, see `docs/PROFILES.md`.

## Procedure

## Step 1 — Validate install method variables

**Goal**: reject unsupported install or first-run modes before making changes.

**Command** (Linux/macOS):
```bash
: "${INSTALL_METHOD:=npm}"
: "${RUN_FIRST_RUN:=false}"
: "${MACHINE_SCENARIO:=fresh}"
case "$INSTALL_METHOD" in npm|source) ;; *) echo "unsupported INSTALL_METHOD=$INSTALL_METHOD" >&2; exit 2 ;; esac
case "$RUN_FIRST_RUN" in true|false) ;; *) echo "RUN_FIRST_RUN must be true or false" >&2; exit 2 ;; esac
case "$MACHINE_SCENARIO" in fresh|existing) ;; *) echo "MACHINE_SCENARIO must be fresh or existing" >&2; exit 2 ;; esac
if [ "$INSTALL_METHOD" = source ] && [ -z "${WORKDIR:-}" ]; then echo "WORKDIR required for source install" >&2; exit 2; fi
if [ "$RUN_FIRST_RUN" = true ] && [ -z "${STORE_PATH:-}" ]; then echo "STORE_PATH required when RUN_FIRST_RUN=true" >&2; exit 2; fi
: "${INIT_EMPTY_PROFILE:=false}"
case "$INIT_EMPTY_PROFILE" in true|false) ;; *) echo "INIT_EMPTY_PROFILE must be true or false" >&2; exit 2 ;; esac
```

**Command** (Windows / PowerShell):
```powershell
if (-not $env:INSTALL_METHOD) { $env:INSTALL_METHOD = 'npm' }
if (-not $env:RUN_FIRST_RUN) { $env:RUN_FIRST_RUN = 'false' }
if (-not $env:MACHINE_SCENARIO) { $env:MACHINE_SCENARIO = 'fresh' }
if ($env:INSTALL_METHOD -notin @('npm','source')) { throw "unsupported INSTALL_METHOD=$env:INSTALL_METHOD" }
if ($env:RUN_FIRST_RUN -notin @('true','false')) { throw 'RUN_FIRST_RUN must be true or false' }
if ($env:MACHINE_SCENARIO -notin @('fresh','existing')) { throw 'MACHINE_SCENARIO must be fresh or existing' }
if ($env:INSTALL_METHOD -eq 'source' -and -not $env:WORKDIR) { throw 'WORKDIR required for source install' }
if ($env:RUN_FIRST_RUN -eq 'true' -and -not $env:STORE_PATH) { throw 'STORE_PATH required when RUN_FIRST_RUN=true' }
if (-not $env:INIT_EMPTY_PROFILE) { $env:INIT_EMPTY_PROFILE = 'false' }
if ($env:INIT_EMPTY_PROFILE -notin @('true','false')) { throw 'INIT_EMPTY_PROFILE must be true or false' }
```

**Verify** (Linux/macOS):
```bash
case "$INSTALL_METHOD/$RUN_FIRST_RUN/$MACHINE_SCENARIO/$INIT_EMPTY_PROFILE" in npm/*/*/*|source/*/*/*) test "$RUN_FIRST_RUN" = true -o "$RUN_FIRST_RUN" = false ;; esac
```

**Verify** (Windows / PowerShell):
```powershell
($env:INSTALL_METHOD -in @('npm','source')) -and ($env:RUN_FIRST_RUN -in @('true','false')) -and ($env:MACHINE_SCENARIO -in @('fresh','existing')) -and ($env:INIT_EMPTY_PROFILE -in @('true','false'))
```

**On failure**: set valid variables and rerun this step. Do not continue with defaults guessed from context.

**Idempotent**: true

## Step 2 — Install from npm

**Goal**: install or update the global npm package when `${INSTALL_METHOD}` is `npm`.

**Command**:
```bash
if [ "${INSTALL_METHOD:-npm}" = npm ]; then npm install -g @asudbring/loki-profile-manager; else echo "skip npm install"; fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:INSTALL_METHOD -eq 'npm') { npm install -g @asudbring/loki-profile-manager } else { 'skip npm install' }
```

**Verify**:
```bash
if [ "${INSTALL_METHOD:-npm}" = npm ]; then loki --version && loki --help >/dev/null; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:INSTALL_METHOD -eq 'npm') { loki --version; loki --help | Out-Null } else { $true }
```

**On failure**: confirm Node.js 18+ and npm are on PATH. On Windows, open a new shell and ensure `%APPDATA%\npm` and `C:\Program Files\nodejs` are on PATH, then rerun this step.

**Idempotent**: true

## Step 3 — Install from source

**Goal**: clone/update the repository and build a local `loki` binary when `${INSTALL_METHOD}` is `source`.

**Command** (Linux/macOS):
```bash
if [ "${INSTALL_METHOD:-npm}" = source ]; then
  : "${REPO_URL:=https://github.com/asudbring/loki-profile-manager.git}"
  : "${REPO_DIR:=${WORKDIR}/loki-profile-manager}"
  mkdir -p "$WORKDIR"
  if [ -d "$REPO_DIR/.git" ]; then git -C "$REPO_DIR" pull --ff-only; else git clone "$REPO_URL" "$REPO_DIR"; fi
  cd "$REPO_DIR"
  go test ./...
  go vet ./...
  go mod verify
  go build -trimpath -o loki ./cmd/loki
else
  echo "skip source install"
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:INSTALL_METHOD -eq 'source') {
  if (-not $env:REPO_URL) { $env:REPO_URL = 'https://github.com/asudbring/loki-profile-manager.git' }
  if (-not $env:REPO_DIR) { $env:REPO_DIR = Join-Path $env:WORKDIR 'loki-profile-manager' }
  New-Item -ItemType Directory -Force -Path $env:WORKDIR | Out-Null
  if (Test-Path (Join-Path $env:REPO_DIR '.git')) { git -C $env:REPO_DIR pull --ff-only } else { git clone $env:REPO_URL $env:REPO_DIR }
  Push-Location $env:REPO_DIR
  go test ./...
  go vet ./...
  go mod verify
  go build -trimpath -o loki.exe ./cmd/loki
  Pop-Location
} else { 'skip source install' }
```

**Verify** (Linux/macOS):
```bash
if [ "${INSTALL_METHOD:-npm}" = source ]; then test -x "${REPO_DIR}/loki" && "${REPO_DIR}/loki" --help >/dev/null; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:INSTALL_METHOD -eq 'source') { Test-Path (Join-Path $env:REPO_DIR 'loki.exe'); & (Join-Path $env:REPO_DIR 'loki.exe') --help | Out-Null } else { $true }
```

**On failure**: surface the failing test, vet, module, or build output. Do not proceed to first-run setup with an unvalidated source build.

**Idempotent**: true

## Step 4 — Select Loki executable

**Goal**: set `${LOKI_EXE}` to the binary installed by the selected method.

**Command** (Linux/macOS):
```bash
if [ "${INSTALL_METHOD:-npm}" = source ]; then export LOKI_EXE="${REPO_DIR}/loki"; else export LOKI_EXE="$(command -v loki)"; fi
printf '%s\n' "$LOKI_EXE"
```

**Command** (Windows / PowerShell):
```powershell
if ($env:INSTALL_METHOD -eq 'source') { $env:LOKI_EXE = Join-Path $env:REPO_DIR 'loki.exe' } else { $env:LOKI_EXE = (Get-Command loki).Source }
$env:LOKI_EXE
```

**Verify** (Linux/macOS):
```bash
test -n "$LOKI_EXE" && "$LOKI_EXE" --version
```

**Verify** (Windows / PowerShell):
```powershell
Test-Path $env:LOKI_EXE; & $env:LOKI_EXE --version
```

**On failure**: rerun Step 2 or Step 3. If npm installed successfully but `loki` is missing, fix PATH and reopen the shell.

**Idempotent**: true

## Step 5 — Configure store path

**Goal**: persist an existing store or initialize a new empty store when `${RUN_FIRST_RUN}` is `true`.

**Command** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then
  "$LOKI_EXE" store discover --manual "$STORE_PATH"
  if [ -d "$STORE_PATH/registry" ] && [ -d "$STORE_PATH/profiles" ]; then
    "$LOKI_EXE" store use "$STORE_PATH"
  else
    "$LOKI_EXE" store init "$STORE_PATH"
  fi
else
  echo "skip first-run store setup"
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') {
  & $env:LOKI_EXE store discover --manual $env:STORE_PATH
  if ((Test-Path (Join-Path $env:STORE_PATH 'registry')) -and (Test-Path (Join-Path $env:STORE_PATH 'profiles'))) {
    & $env:LOKI_EXE store use $env:STORE_PATH
  } else {
    & $env:LOKI_EXE store init $env:STORE_PATH
  }
} else { 'skip first-run store setup' }
```

**Verify** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then "$LOKI_EXE" store status | grep -E 'valid|configured|Effective'; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') { & $env:LOKI_EXE store status | Select-String -Pattern 'valid|configured|Effective' } else { $true }
```

**On failure**: if the path is non-empty but not a Loki store, choose an empty folder or a valid existing store. Do not delete user files automatically.

**Idempotent**: true

## Step 6 — Initialize an empty profile core

**Goal**: create a minimal profile core manifest when `${INIT_EMPTY_PROFILE}` is `true`.

**Command** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ] && [ "${INIT_EMPTY_PROFILE:-false}" = true ]; then
  : "${PROFILE:=work}"
  root="$STORE_PATH/profiles/$PROFILE/core"
  mkdir -p "$root/files" "$root/skills" "$root/templates"
  if [ ! -f "$root/manifest.yaml" ]; then
    cat > "$root/manifest.yaml" <<YAML
version: 1
name: ${PROFILE}-core
files: []
skills: []
ignore: []
merge_rules: {}
targets: {}
YAML
  fi
else
  echo "skip empty profile initialization"
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true' -and $env:INIT_EMPTY_PROFILE -eq 'true') {
  if (-not $env:PROFILE) { $env:PROFILE = 'work' }
  $root = Join-Path $env:STORE_PATH "profiles\$env:PROFILE\core"
  New-Item -ItemType Directory -Force (Join-Path $root 'files'), (Join-Path $root 'skills'), (Join-Path $root 'templates') | Out-Null
  $manifest = Join-Path $root 'manifest.yaml'
  if (-not (Test-Path $manifest)) {
    @"
version: 1
name: $($env:PROFILE)-core
files: []
skills: []
ignore: []
merge_rules: {}
targets: {}
"@ | Set-Content -Encoding UTF8 $manifest
  }
} else { 'skip empty profile initialization' }
```

**Verify** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ] && [ "${INIT_EMPTY_PROFILE:-false}" = true ]; then test -f "$STORE_PATH/profiles/$PROFILE/core/manifest.yaml"; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true' -and $env:INIT_EMPTY_PROFILE -eq 'true') { Test-Path (Join-Path $env:STORE_PATH "profiles\$env:PROFILE\core\manifest.yaml") } else { $true }
```

**On failure**: create the profile core with `adopt`, `migrate local`, `migrate repo`, or the manual manifest shape documented in `docs/PROFILES.md`.

**Idempotent**: true

## Step 7 — Register machine

**Goal**: create or update the machine registry record when `${RUN_FIRST_RUN}` is `true`.

**Command** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then
  : "${PROFILE:=work}"
  : "${MACHINE_NAME:=$(hostname)}"
  set --
  for bucket in ${BUCKETS:-}; do set -- "$@" --allow-bucket "$bucket"; done
  "$LOKI_EXE" machine register --name "$MACHINE_NAME" --allow-profile "$PROFILE" "$@"
else
  echo "skip machine register"
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') {
  if (-not $env:PROFILE) { $env:PROFILE = 'work' }
  if (-not $env:MACHINE_NAME) { $env:MACHINE_NAME = $env:COMPUTERNAME }
  $args = @('machine','register','--name',$env:MACHINE_NAME,'--allow-profile',$env:PROFILE)
  if ($env:BUCKETS) { $env:BUCKETS -split ' ' | Where-Object { $_ } | ForEach-Object { $args += @('--allow-bucket', $_) } }
  & $env:LOKI_EXE @args
} else { 'skip machine register' }
```

**Verify** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then "$LOKI_EXE" machine status | grep -E 'registered|Machine'; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') { & $env:LOKI_EXE machine status | Select-String -Pattern 'registered|Machine' } else { $true }
```

**On failure**: wait for sync-provider file locks to clear and rerun the same register command. Do not edit `registry/machines.json` manually unless debugging with explicit approval.

**Idempotent**: true

## Step 8 — Migrate existing local profiles

**Goal**: when `${MACHINE_SCENARIO}` is `existing`, capture known local settings into the store before switching.

**Command** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ] && [ "${MACHINE_SCENARIO:-fresh}" = existing ]; then
  buckets=${BUCKETS:-}
  first_bucket=${buckets%% *}
  if [ -n "$first_bucket" ]; then
    "$LOKI_EXE" migrate local --profile "$PROFILE" --bucket "$first_bucket" --dry-run
    "$LOKI_EXE" migrate local --profile "$PROFILE" --bucket "$first_bucket" --yes
  else
    "$LOKI_EXE" migrate local --profile "$PROFILE" --dry-run
    "$LOKI_EXE" migrate local --profile "$PROFILE" --yes
  fi
  if [ -n "${LEGACY_REPO:-}" ]; then
    "$LOKI_EXE" migrate repo "$LEGACY_REPO" --profile "$PROFILE" --dry-run
    "$LOKI_EXE" migrate repo "$LEGACY_REPO" --profile "$PROFILE" --yes
  fi
else
  echo "skip existing-machine migration"
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true' -and $env:MACHINE_SCENARIO -eq 'existing') {
  $bucket = ($env:BUCKETS -split ' ' | Where-Object { $_ } | Select-Object -First 1)
  if ($bucket) {
    & $env:LOKI_EXE migrate local --profile $env:PROFILE --bucket $bucket --dry-run
    & $env:LOKI_EXE migrate local --profile $env:PROFILE --bucket $bucket --yes
  } else {
    & $env:LOKI_EXE migrate local --profile $env:PROFILE --dry-run
    & $env:LOKI_EXE migrate local --profile $env:PROFILE --yes
  }
  if ($env:LEGACY_REPO) {
    & $env:LOKI_EXE migrate repo $env:LEGACY_REPO --profile $env:PROFILE --dry-run
    & $env:LOKI_EXE migrate repo $env:LEGACY_REPO --profile $env:PROFILE --yes
  }
} else { 'skip existing-machine migration' }
```

**Verify** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then "$LOKI_EXE" verify "$PROFILE" ${BUCKETS:-}; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') { $args=@('verify',$env:PROFILE); if($env:BUCKETS){$args += ($env:BUCKETS -split ' ' | Where-Object { $_ })}; & $env:LOKI_EXE @args } else { $true }
```

**On failure**: inspect the migration dry-run output and generated manifest. Use `loki adopt <target> --profile <profile> --dry-run` for missed targets. Do not proceed to real switch until `verify` succeeds.

**Idempotent**: false (real migration writes store files and local managed-target records; reruns update the same layer but may reflect current local file changes)

## Step 9 — Verify and dry-run switch

**Goal**: prove the selected profile can activate safely without writing target files.

**Command** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then
  "$LOKI_EXE" doctor
  "$LOKI_EXE" verify "$PROFILE" ${BUCKETS:-}
  "$LOKI_EXE" switch "$PROFILE" ${BUCKETS:-} --dry-run
else
  "$LOKI_EXE" doctor
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') {
  & $env:LOKI_EXE doctor
  $verifyArgs=@('verify',$env:PROFILE); if($env:BUCKETS){$verifyArgs += ($env:BUCKETS -split ' ' | Where-Object { $_ })}; & $env:LOKI_EXE @verifyArgs
  $switchArgs=@('switch',$env:PROFILE); if($env:BUCKETS){$switchArgs += ($env:BUCKETS -split ' ' | Where-Object { $_ })}; $switchArgs += '--dry-run'; & $env:LOKI_EXE @switchArgs
} else { & $env:LOKI_EXE doctor }
```

**Verify** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then "$LOKI_EXE" switch "$PROFILE" ${BUCKETS:-} --dry-run | grep -F 'Loki switch dry-run'; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') { $args=@('switch',$env:PROFILE); if($env:BUCKETS){$args += ($env:BUCKETS -split ' ' | Where-Object { $_ })}; $args += '--dry-run'; & $env:LOKI_EXE @args | Select-String -SimpleMatch 'Loki switch dry-run' } else { $true }
```

**On failure**: resolve every blocker. Adopt/migrate unmanaged targets, reconcile managed hash drift, or remove conflicting manifest entries. Do not run `switch --yes` until dry-run blockers are gone.

**Idempotent**: true

## Step 10 — Activate profile

**Goal**: activate the selected profile only after a successful dry-run.

**Command** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then
  "$LOKI_EXE" switch "$PROFILE" ${BUCKETS:-} --yes
else
  echo "skip activation"
fi
```

**Command** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') {
  $args=@('switch',$env:PROFILE); if($env:BUCKETS){$args += ($env:BUCKETS -split ' ' | Where-Object { $_ })}; $args += '--yes'; & $env:LOKI_EXE @args
} else { 'skip activation' }
```

**Verify** (Linux/macOS):
```bash
if [ "${RUN_FIRST_RUN:-false}" = true ]; then "$LOKI_EXE" status --verbose | grep -E "Active profile: ${PROFILE}"; else exit 0; fi
```

**Verify** (Windows / PowerShell):
```powershell
if ($env:RUN_FIRST_RUN -eq 'true') { & $env:LOKI_EXE status --verbose | Select-String -Pattern "Active profile: $env:PROFILE" } else { $true }
```

**On failure**: if output says capture is required and the change is safe copy-mode drift, rerun with `switch <profile> [buckets...] --capture-local --yes`. If output reports unmanaged overwrite protection, return to migration/adoption. Do not delete local files automatically.

**Idempotent**: true when targets remain unchanged between runs

## Step 11 — Final smoke

**Goal**: confirm Loki can report state and dry-run the active profile after activation.

**Command** (Linux/macOS):
```bash
"$LOKI_EXE" status --verbose
if [ "${RUN_FIRST_RUN:-false}" = true ]; then "$LOKI_EXE" switch "$PROFILE" ${BUCKETS:-} --dry-run; fi
```

**Command** (Windows / PowerShell):
```powershell
& $env:LOKI_EXE status --verbose
if ($env:RUN_FIRST_RUN -eq 'true') { $args=@('switch',$env:PROFILE); if($env:BUCKETS){$args += ($env:BUCKETS -split ' ' | Where-Object { $_ })}; $args += '--dry-run'; & $env:LOKI_EXE @args }
```

**Verify** (Linux/macOS):
```bash
"$LOKI_EXE" --help >/dev/null && "$LOKI_EXE" status >/dev/null
```

**Verify** (Windows / PowerShell):
```powershell
& $env:LOKI_EXE --help | Out-Null; & $env:LOKI_EXE status | Out-Null
```

**On failure**: surface the command and exit code. Do not inspect or print secret-rendered file contents.

**Idempotent**: true

## Rollback

Npm install rollback:

```bash
npm uninstall -g @asudbring/loki-profile-manager
```

Source install rollback, Linux/macOS:

```bash
rm -rf "${REPO_DIR}"
```

Source install rollback, Windows PowerShell:

```powershell
Remove-Item -Recurse -Force $env:REPO_DIR
```

Profile activation rollback must use Loki snapshots, not package uninstall:

```bash
loki snapshots list
loki snapshots show <snapshot-id>
loki snapshots restore <snapshot-id> --dry-run
```

Run real restore only after the required dry-run guard and consent described in `docs/USAGE.md`.

## Verification

An install is successful when:

```bash
loki --version
loki --help
loki doctor
```

A first-run setup is successful when:

```bash
loki store status
loki machine status
loki verify ${PROFILE} ${BUCKETS}
loki switch ${PROFILE} ${BUCKETS} --dry-run
loki status --verbose
```

Expected result: commands exit 0, `status --verbose` reports the intended active profile/buckets after activation, and the final dry-run has no unexpected blockers.
