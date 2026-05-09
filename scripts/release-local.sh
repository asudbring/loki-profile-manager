#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: scripts/release-local.sh <version> [options]

Build a dogfood release locally without relying on GitHub Actions.

Options:
  --out-dir <dir>        Package output directory (default: dist/packages)
  --skip-validation      Skip go/script validation before packaging
  --skip-pwsh-syntax     Skip PowerShell syntax validation
  --allow-dirty          Allow a dirty working tree
  --tag                  Create/update-check a local git tag after packaging
  --push-tag             Push the tag to origin (implies --tag)
  --upload               Create/update the GitHub Release and upload assets
  --notes-file <file>    Release notes file for --upload
  -h, --help             Show this help

Default mode validates, packages, verifies checksums, and stops locally.
It does not create tags, push tags, or upload releases unless requested.
EOF
}

version=""
out_dir="dist/packages"
run_validation=1
run_pwsh_syntax=1
allow_dirty=0
create_tag=0
push_tag=0
upload=0
notes_file=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --out-dir)
      if [[ $# -lt 2 ]]; then echo "--out-dir requires a value" >&2; exit 2; fi
      out_dir="$2"
      shift 2
      ;;
    --skip-validation)
      run_validation=0
      shift
      ;;
    --skip-pwsh-syntax)
      run_pwsh_syntax=0
      shift
      ;;
    --allow-dirty)
      allow_dirty=1
      shift
      ;;
    --tag)
      create_tag=1
      shift
      ;;
    --push-tag)
      create_tag=1
      push_tag=1
      shift
      ;;
    --upload)
      upload=1
      create_tag=1
      shift
      ;;
    --notes-file)
      if [[ $# -lt 2 ]]; then echo "--notes-file requires a value" >&2; exit 2; fi
      notes_file="$2"
      shift 2
      ;;
    --*)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
    *)
      if [[ -n "$version" ]]; then
        echo "unexpected argument: $1" >&2
        usage
        exit 2
      fi
      version="$1"
      shift
      ;;
  esac
done

if [[ -z "$version" ]]; then
  usage
  exit 2
fi
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?(\+[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "invalid release version: $version" >&2
  echo "expected semver tag like v0.0.0-dogfood.10" >&2
  exit 2
fi

repo_root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$repo_root"

if [[ $allow_dirty -ne 1 ]] && [[ -n "$(git status --porcelain)" ]]; then
  echo "working tree is dirty; commit/stash changes or pass --allow-dirty" >&2
  git status --short >&2
  exit 1
fi

head_commit=$(git rev-parse HEAD)
head_short=$(git rev-parse --short=12 HEAD)

tmp_root=$(mktemp -d)
cleanup() {
  rm -rf "$tmp_root"
}
trap cleanup EXIT

run() {
  echo "+ $*"
  "$@"
}

normalize_output_dir() {
  local requested="$1"
  local abs
  abs=$(python3 - "$requested" <<'PY'
import os
import sys
print(os.path.abspath(sys.argv[1]))
PY
)
  local repo_abs
  repo_abs=$(pwd -P)
  case "$abs" in
    "$repo_abs"/dist/*) ;;
    *)
      echo "refusing destructive output directory outside repo dist/: $requested" >&2
      echo "choose a path like dist/packages or dist/manual/<version>" >&2
      exit 1
      ;;
  esac
  if [[ "$abs" == "$repo_abs" || "$abs" == "$HOME" || "$abs" == "/" ]]; then
    echo "refusing unsafe output directory: $requested" >&2
    exit 1
  fi
  if [[ -L "$abs" ]]; then
    echo "refusing symlink output directory: $requested" >&2
    exit 1
  fi
  if [[ -e "$abs" && ! -d "$abs" ]]; then
    echo "output path exists and is not a directory: $requested" >&2
    exit 1
  fi
  printf '%s\n' "$abs"
}

remote_tag_commit() {
  local lines deref direct
  lines=$(git ls-remote --tags origin "refs/tags/$version" "refs/tags/$version^{}" 2>/dev/null || true)
  deref=$(awk '$2 ~ /\^\{\}$/ {print $1; exit}' <<<"$lines")
  direct=$(awk '$2 !~ /\^\{\}$/ {print $1; exit}' <<<"$lines")
  printf '%s\n' "${deref:-$direct}"
}

verify_remote_tag_matches_head() {
  local remote_commit
  remote_commit=$(remote_tag_commit)
  if [[ -z "$remote_commit" ]]; then
    return 1
  fi
  if [[ "$remote_commit" != "$head_commit" ]]; then
    echo "remote tag $version points at $remote_commit, not HEAD $head_commit" >&2
    exit 1
  fi
  return 0
}

validate() {
  echo "== local validation =="
  run go test ./...
  run go vet ./...
  run go mod verify
  run go build -trimpath -o "$tmp_root/loki" ./cmd/loki
  run bash -n \
    scripts/install.sh \
    scripts/uninstall.sh \
    scripts/package-release.sh \
    scripts/package-npm.sh \
    scripts/release-local.sh \
    scripts/validate-cross.sh \
    scripts/parallels-windows-admin-probe.sh
  run node --check npm/bin/loki.js
  if [[ $run_pwsh_syntax -eq 1 ]]; then
    if ! command -v pwsh >/dev/null 2>&1; then
      echo "pwsh not found; install PowerShell or pass --skip-pwsh-syntax" >&2
      exit 1
    fi
    run pwsh -NoProfile -Command '
      $ErrorActionPreference = "Stop"
      $allErrors = @()
      foreach ($file in @("scripts/install.ps1", "scripts/uninstall.ps1", "scripts/windows-installer-smoke.ps1", "scripts/validate-local.ps1")) {
        $errors = $null
        $null = [System.Management.Automation.PSParser]::Tokenize((Get-Content -Raw $file), [ref]$errors)
        if ($errors) { $allErrors += $errors }
      }
      if ($allErrors.Count -gt 0) { $allErrors | Format-List *; exit 1 }
    '
  fi
}

verify_checksums() {
  echo "== checksum verification =="
  if [[ ! -f "$out_dir/checksums.txt" ]]; then
    echo "missing checksums.txt in $out_dir" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$out_dir" && sha256sum -c checksums.txt)
  else
    (cd "$out_dir" && shasum -a 256 -c checksums.txt)
  fi
}

ensure_local_tag() {
  echo "== local tag =="
  if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
    tagged_commit=$(git rev-parse "refs/tags/$version^{commit}")
    if [[ "$tagged_commit" != "$head_commit" ]]; then
      echo "tag $version points at $tagged_commit, not HEAD $head_commit" >&2
      exit 1
    fi
    echo "tag $version already points at HEAD"
  else
    run git tag "$version"
  fi
}

push_local_tag() {
  echo "== push tag =="
  if verify_remote_tag_matches_head; then
    echo "remote tag $version already points at HEAD"
  else
    run git push origin "refs/tags/$version"
  fi
}

write_default_notes() {
  local path="$1"
  local npm_version="${version#v}"
  cat > "$path" <<NOTES
## Loki Profile Manager $version

Prerelease build.

Built locally from commit \`$head_short\`. Use this lane when GitHub Actions is unavailable or when validating release assets before pushing a tag.

### Install

npm tarball install:

\`\`\`bash
npm install -g ./asudbring-loki-profile-manager-$npm_version.tgz
loki --version
\`\`\`

Windows script install:

\`\`\`powershell
.\\install.ps1 -Version $version -ArchivePath .\\loki_${version}_windows_arm64.zip -ChecksumsPath .\\checksums.txt -AddToPath
\`\`\`

macOS/Linux script install:

\`\`\`bash
./install.sh --version $version --archive ./loki_${version}_<os>_<arch>.tar.gz --checksums ./checksums.txt
\`\`\`

### Notes

- Verify \`checksums.txt\` before install.
- npm install/uninstall affects only the npm wrapper and bundled binary; Loki local state and synced stores are preserved.
- Script uninstall preserves local Loki state, synced stores, and managed targets by default.
NOTES
}

upload_release() {
  echo "== GitHub release upload =="
  if ! command -v gh >/dev/null 2>&1; then
    echo "gh not found; install GitHub CLI or omit --upload" >&2
    exit 1
  fi
  assets=()
  while IFS= read -r asset; do
    assets+=("$asset")
  done < <(find "$out_dir" -maxdepth 1 -type f | sort)
  if [[ ${#assets[@]} -eq 0 ]]; then
    echo "no release assets found in $out_dir" >&2
    exit 1
  fi

  local effective_notes="$notes_file"
  if [[ -z "$effective_notes" ]]; then
    effective_notes="$tmp_root/release-notes.md"
    write_default_notes "$effective_notes"
  elif [[ ! -f "$effective_notes" ]]; then
    echo "release notes file not found: $effective_notes" >&2
    exit 1
  fi

  prerelease=()
  if [[ "$version" == *-* ]]; then
    prerelease=(--prerelease)
  fi

  if gh release view "$version" >/dev/null 2>&1; then
    verify_remote_tag_matches_head >/dev/null
    echo "release $version exists and tag points at HEAD; uploading assets with --clobber"
    run gh release upload "$version" "${assets[@]}" --clobber
  else
    echo "creating release $version"
    run gh release create "$version" "${assets[@]}" --title "$version" --notes-file "$effective_notes" --target "$head_commit" "${prerelease[@]}"
  fi
}

out_dir=$(normalize_output_dir "$out_dir")

if [[ $run_validation -eq 1 ]]; then
  validate
else
  echo "== local validation skipped =="
fi

echo "== package release assets =="
run ./scripts/package-release.sh "$version" "$out_dir"
run ./scripts/package-npm.sh "$version" "$out_dir"
verify_checksums

if [[ $create_tag -eq 1 ]]; then
  ensure_local_tag
fi
if [[ $push_tag -eq 1 ]]; then
  push_local_tag
fi
if [[ $upload -eq 1 ]]; then
  upload_release
fi

asset_count=$(find "$out_dir" -maxdepth 1 -type f | wc -l | tr -d ' ')
cat <<SUMMARY

Local release ready.
Version: $version
Commit: $head_short
Assets: $asset_count files in $out_dir
Upload: $([[ $upload -eq 1 ]] && echo yes || echo no)

Next dogfood:
  npm install -g "$out_dir/asudbring-loki-profile-manager-${version#v}.tgz"
  loki --version
SUMMARY
