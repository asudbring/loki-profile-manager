# Profiles, buckets, settings, and skills

This guide starts from an empty Loki store. It explains how to choose profile names, initialize profile layers, register machines, add settings, and organize skills.

## Concepts

| Term | Meaning |
|---|---|
| Store | Synced Loki source of truth. Contains `registry/` and `profiles/`. |
| Common layer | `profiles/common`. Applies to every selected profile. Use for files/skills shared by all profiles. |
| Profile core | `profiles/<profile>/core`. Always active when that parent profile is active. |
| Bucket | `profiles/<profile>/buckets/<bucket>`. Optional layer activated with a parent profile. |
| Machine registration | Per-machine policy in `registry/machines.json`. A machine must be allowed to activate a profile/bucket before `verify` or `switch`. |
| Managed target | A local file, directory, or symlink Loki has written or adopted and can safely update when hashes still match. |

Layer order during activation:

1. Common layer.
2. Profile core.
3. Requested buckets in command order.

Later layers win for structured merge targets.

## Naming profiles and buckets

Loki requires profile and bucket names to be simple path components:

- No `/` or `\`.
- No absolute paths.
- No `.` or `..`.
- No Windows drive prefixes such as `C:`.

Recommended convention: lower-kebab-case.

Profile names should describe a broad context:

- `personal`
- `work`
- `writing`
- `dev`
- `consulting`

Bucket names should describe optional capabilities inside a profile:

- `ai`
- `cloud`
- `frontend`
- `backend`
- `security`
- `publishing`
- `client-a`

Example combinations:

```bash
loki switch personal --dry-run
loki switch work cloud --dry-run
loki switch dev backend security --dry-run
```

## Start from an empty store

### 1. Initialize or select the store

```bash
STORE="$HOME/OneDrive/LokiProfileManager"
loki store init "$STORE"
# Future shells can use the persisted store path.
loki store status
```

PowerShell:

```powershell
$Store = Join-Path $env:OneDrive "LokiProfileManager"
loki store init $Store
loki store status
```

An empty store has no activatable profiles. Create at least one profile core before `verify <profile>` or `switch <profile>`.

### 2. Choose your first profile and buckets

Example:

```text
Profile: personal
Buckets: ai, cloud
```

Start with one profile core. Add buckets only when you have settings or skills that should be optional.

### 3. Initialize the profile core

Use one of these paths.

#### Option A: adopt one existing file

This is the easiest way to create a profile and make one local target Loki-managed.

```bash
loki adopt ~/.gitconfig --profile personal --dry-run
loki adopt ~/.gitconfig --profile personal --yes
```

Windows PowerShell:

```powershell
loki adopt "$env:USERPROFILE\.gitconfig" --profile personal --dry-run
loki adopt "$env:USERPROFILE\.gitconfig" --profile personal --yes
```

`adopt` copies the target into `profiles/personal/core/files/`, writes or updates `profiles/personal/core/manifest.yaml`, and records local managed-target state. It does not rewrite the target during adoption.

#### Option B: migrate known local settings

Use this when the current machine already has shell profiles, Git config, app settings, or tool config you want to import.

```bash
loki migrate local --profile personal --dry-run
loki migrate local --profile personal --yes
```

Use a bucket when the imported files belong to an optional context:

```bash
loki migrate local --profile personal --bucket ai --dry-run
loki migrate local --profile personal --bucket ai --yes
```

#### Option C: migrate a legacy repo

Use this when existing dotfiles/settings are in a repo.

```bash
loki migrate repo ~/github/my-dotfiles --profile personal --dry-run
loki migrate repo ~/github/my-dotfiles --profile personal --yes
```

Bucket example:

```bash
loki migrate repo ~/github/my-dotfiles --profile personal --bucket cloud --dry-run
loki migrate repo ~/github/my-dotfiles --profile personal --bucket cloud --yes
```

#### Option D: create an empty profile manually

Use this when you want to define profile structure before adding files.

```bash
PROFILE=personal
ROOT="$STORE/profiles/$PROFILE/core"
mkdir -p "$ROOT/files" "$ROOT/skills" "$ROOT/templates"
cat > "$ROOT/manifest.yaml" <<'YAML'
version: 1
name: personal-core
files: []
skills: []
ignore: []
merge_rules: {}
targets: {}
YAML
```

PowerShell:

```powershell
$Profile = "personal"
$Root = Join-Path $Store "profiles\$Profile\core"
New-Item -ItemType Directory -Force `
  (Join-Path $Root "files"), `
  (Join-Path $Root "skills"), `
  (Join-Path $Root "templates") | Out-Null
@'
version: 1
name: personal-core
files: []
skills: []
ignore: []
merge_rules: {}
targets: {}
'@ | Set-Content -Encoding UTF8 (Join-Path $Root "manifest.yaml")
```

### 4. Register the machine

Registration controls what this machine may activate.

```bash
loki machine register --name "$(hostname)" --allow-profile personal --allow-bucket ai --allow-bucket cloud
loki machine status
```

PowerShell:

```powershell
loki machine register --name $env:COMPUTERNAME --allow-profile personal --allow-bucket ai --allow-bucket cloud
loki machine status
```

You can repeat or comma-separate profile/bucket flags:

```bash
loki machine register --allow-profile personal,work --allow-bucket ai,cloud
```

Re-run `machine register` when this machine should be allowed to activate new profiles or buckets.

### 5. Verify and activate

```bash
loki verify personal
loki switch personal --dry-run
loki switch personal --yes
loki status --verbose
```

With buckets:

```bash
loki verify personal ai cloud
loki switch personal ai cloud --dry-run
loki switch personal ai cloud --yes
```

If the dry-run blocks on unmanaged files, stop. Use `adopt`, `migrate local`, `migrate repo`, or move the file aside intentionally before activation.

## Add settings to a profile

### Adopt an existing local setting

```bash
loki adopt ~/.config/app/settings.json --profile personal --mode merge --dry-run
loki adopt ~/.config/app/settings.json --profile personal --mode merge --yes
```

Bucket:

```bash
loki adopt ~/.config/cloud/config.json --profile personal --bucket cloud --mode merge --dry-run
loki adopt ~/.config/cloud/config.json --profile personal --bucket cloud --mode merge --yes
```

### Choose the mode

| Mode | Use when | Notes |
|---|---|---|
| `symlink` | App can read/write directly through the store source. | No copy-back needed; Windows requires Developer Mode or elevation. |
| `copy` | App needs a real file/directory at the target. | Local edits are captured later with `switch --capture-local --yes` when safe. |
| `merge` | Multiple layers contribute to one JSON/YAML/TOML settings file. | Later layers win. Merge drift capture is manual in this MVP. |
| `render` | File needs secret values at activation time. | Rendered output is regenerated and never captured. Secret values must live outside the store. |

### Manual file entry example

Place the source file under the layer, then add a manifest entry.

```text
profiles/personal/core/files/gitconfig
profiles/personal/core/manifest.yaml
```

```yaml
version: 1
name: personal-core
files:
  - id: gitconfig
    source: files/gitconfig
    target: ~/.gitconfig
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
```

Structured merge example:

```yaml
files:
  - id: vscode-settings
    source: files/vscode/settings.json
    target: ~/.config/Code/User/settings.json
    mode: merge
    format: json
```

Windows redirected Documents example:

```yaml
files:
  - id: powershell-profile
    source: files/powershell/profile.ps1
    target: ${DOCUMENTS}/PowerShell/Microsoft.PowerShell_profile.ps1
    mode: copy
```

`${DOCUMENTS}`, `${DOCUMENTS_DIR}`, and `${USER_DOCUMENTS}` resolve through the Windows Known Folder API when available. Use them for PowerShell profiles instead of hard-coding `C:\Users\<user>\Documents`.

Render example:

```yaml
files:
  - id: tool-config
    source: templates/tool-config.json.tmpl
    target: ~/.config/tool/config.json
    mode: render
    format: json
    secrets:
      - TOOL_API_KEY
```

The template can reference `{{ TOOL_API_KEY }}` or `${TOOL_API_KEY}`. Run `loki secrets status` and `loki secrets check TOOL_API_KEY` before activation.

## Organize skills

A skill source must be a folder or zip archive containing `SKILL.md`.

Import shared skills into common:

```bash
loki import-skill ~/skills/editorial-pass --common --dry-run
loki import-skill ~/skills/editorial-pass --common --yes
```

Import profile-specific skills into a profile core:

```bash
loki import-skill ~/skills/work-reviewer --profile work --dry-run
loki import-skill ~/skills/work-reviewer --profile work --yes
```

Import optional skills into a bucket:

```bash
# Parent profile core must already exist.
loki import-skill ~/skills/cloud-auditor --profile work --bucket cloud --dry-run
loki import-skill ~/skills/cloud-auditor --profile work --bucket cloud --yes
```

Rename on import:

```bash
loki import-skill ~/Downloads/cloud-auditor.zip --profile work --bucket cloud --name azure-auditor --yes
```

Replace an existing imported skill:

```bash
loki import-skill ~/skills/cloud-auditor --profile work --bucket cloud --name azure-auditor --overwrite --yes
```

Recommended organization:

| Layer | Put here |
|---|---|
| `profiles/common/skills` | Skills every profile can use. |
| `profiles/<profile>/core/skills` | Skills always relevant to that profile. |
| `profiles/<profile>/buckets/<bucket>/skills` | Skills relevant only when that bucket is active. |

`import-skill` copies skill source into the store and updates the layer manifest. It does not by itself install skills into Claude, Pi, Codex, VS Code, or any other runtime. Make runtime availability explicit with managed app settings, shell profiles, or file targets that point the tool at the desired Loki-managed skill directories.

## Organize settings

Use this split when designing a new store:

| Setting type | Suggested layer |
|---|---|
| Shell baseline, prompt theme, common editor defaults | Common |
| Identity-specific Git config, AI-tool persona docs, default app settings | Profile core |
| Cloud provider config, customer/project-specific tools, optional skill bundles | Bucket |
| Secrets or tokens | Never in store. Use render templates plus Infisical or another secret provider. |

Example structure:

```text
profiles/
├── common/
│   ├── files/
│   ├── skills/
│   ├── templates/
│   └── manifest.yaml
└── personal/
    ├── core/
    │   ├── files/
    │   ├── skills/
    │   ├── templates/
    │   └── manifest.yaml
    └── buckets/
        ├── ai/
        │   ├── files/
        │   ├── skills/
        │   ├── templates/
        │   └── manifest.yaml
        └── cloud/
            ├── files/
            ├── skills/
            ├── templates/
            └── manifest.yaml
```

## Promotion workflow

Use this loop for every new profile, bucket, skill, or setting:

1. Add or import into the store.
2. Run `loki verify <profile> [buckets...]`.
3. Run `loki switch <profile> [buckets...] --dry-run`.
4. Review every planned target.
5. Run `loki switch <profile> [buckets...] --yes` only when the plan is expected.
6. Open a fresh shell/app and verify behavior.
7. Run `loki status --verbose` and one more `switch --dry-run`.

If a copied managed target changed locally and you want to write it back before changing profiles:

```bash
loki switch <profile> [buckets...] --capture-local --yes
```

`--capture-local` only handles safe copy-mode drift from already-managed targets. It is not a way to overwrite unmanaged files.
