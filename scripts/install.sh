#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: install.sh [options]

Options:
  --version VERSION       Expected Loki version, e.g. v0.1.0
  --archive PATH         Release tar.gz archive to install
  --archive-path PATH    Alias for --archive
  --checksums PATH       checksums.txt from same release
  --checksums-path PATH  Alias for --checksums
  --install-dir DIR      Install directory (default: $HOME/.local/bin)
  --store-path DIR       Initialize/use Loki store after install
  --force                Overwrite existing loki binary
  --require-symlink      Fail if symlink probe fails
  -h, --help             Show help
USAGE
}

die() {
  echo "install.sh: $*" >&2
  exit 1
}

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
version=""
archive=""
checksums=""
install_dir="${LOKI_INSTALL_DIR:-$HOME/.local/bin}"
store_path=""
force=0
require_symlink=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || die "--version requires a value"
      version="$2"
      shift 2
      ;;
    --archive|--archive-path)
      [[ $# -ge 2 ]] || die "$1 requires a value"
      archive="$2"
      shift 2
      ;;
    --checksums|--checksums-path)
      [[ $# -ge 2 ]] || die "$1 requires a value"
      checksums="$2"
      shift 2
      ;;
    --install-dir)
      [[ $# -ge 2 ]] || die "--install-dir requires a value"
      install_dir="$2"
      shift 2
      ;;
    --store-path)
      [[ $# -ge 2 ]] || die "--store-path requires a value"
      store_path="$2"
      shift 2
      ;;
    --force)
      force=1
      shift
      ;;
    --require-symlink)
      require_symlink=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

resolve_path() {
  local path="$1"
  [[ -n "$path" ]] || return 0
  if [[ "$path" = /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s\n' "$(pwd)/$path"
  fi
}

detect_os_arch() {
  local os arch
  case "$(uname -s)" in
    Darwin) os="darwin" ;;
    Linux) os="linux" ;;
    *) die "unsupported OS: $(uname -s)" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
  printf '%s %s\n' "$os" "$arch"
}

find_archive() {
  local requested="$1"
  [[ -n "$requested" ]] && { resolve_path "$requested"; return; }
  read -r os arch < <(detect_os_arch)
  local patterns=()
  if [[ -n "$version" ]]; then
    patterns+=("loki_${version}_${os}_${arch}.tar.gz")
  fi
  patterns+=("loki_*_${os}_${arch}.tar.gz")
  local root pattern candidate
  for root in "$(pwd)" "$script_dir"; do
    for pattern in "${patterns[@]}"; do
      for candidate in "$root"/$pattern; do
        if [[ -f "$candidate" ]]; then
          printf '%s\n' "$candidate"
          return
        fi
      done
    done
  done
  die "release archive not found; pass --archive"
}

find_checksums() {
  local requested="$1" archive_path="$2" archive_dir
  [[ -n "$requested" ]] && { resolve_path "$requested"; return; }
  archive_dir=$(CDPATH= cd -- "$(dirname -- "$archive_path")" && pwd)
  for candidate in "$archive_dir/checksums.txt" "$script_dir/checksums.txt" "$(pwd)/checksums.txt"; do
    if [[ -f "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return
    fi
  done
  die "checksums.txt not found; pass --checksums"
}

sha256_file() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print tolower($1)}'
  else
    shasum -a 256 "$path" | awk '{print tolower($1)}'
  fi
}

expected_sha256() {
  local checksums_path="$1" asset="$2" base
  base=$(basename -- "$asset")
  awk -v file="$base" '
    /^[[:space:]]*[0-9A-Fa-f]{64}[[:space:]]+/ {
      hash=tolower($1); name=$2; sub(/^\*/, "", name); n=split(name, parts, "/");
      if (parts[n] == file) { print hash; found=1; exit }
    }
    END { if (!found) exit 1 }
  ' "$checksums_path"
}

verify_checksum() {
  local asset="$1" checksums_path="$2" expected actual
  expected=$(expected_sha256 "$checksums_path" "$asset") || die "checksum entry not found for $(basename -- "$asset")"
  actual=$(sha256_file "$asset")
  [[ "$actual" == "$expected" ]] || die "checksum mismatch for $(basename -- "$asset"): got $actual want $expected"
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

symlink_probe() {
  local root target link
  root=$(mktemp -d 2>/dev/null || mktemp -d -t loki-symlink-probe)
  target="$root/target.txt"
  link="$root/link.txt"
  printf 'loki symlink probe\n' > "$target"
  if ln -s "$target" "$link" 2>/dev/null; then
    rm -rf "$root"
    return 0
  fi
  rm -rf "$root"
  return 1
}

archive=$(find_archive "$archive")
checksums=$(find_checksums "$checksums" "$archive")
install_dir=$(resolve_path "$install_dir")
[[ -z "$store_path" ]] || store_path=$(resolve_path "$store_path")

echo "== verify archive =="
[[ -f "$archive" ]] || die "archive not found: $archive"
[[ -f "$checksums" ]] || die "checksums not found: $checksums"
verify_checksum "$archive" "$checksums"

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t loki-install)
cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT

echo "== extract archive =="
tar -xzf "$archive" -C "$tmp"
extracted=$(find "$tmp" -type f -name loki -print -quit)
[[ -n "$extracted" ]] || die "loki binary missing from archive"

installed_version=$("$extracted" --version | head -n 1 | tr -d '\r')
if [[ -n "$version" && "$installed_version" != "$version" ]]; then
  die "version mismatch: got $installed_version want $version"
fi

echo "== install binary =="
mkdir -p "$install_dir"
installed="$install_dir/loki"
if [[ -e "$installed" && "$force" -ne 1 ]]; then
  die "loki already exists at $installed; pass --force to overwrite"
fi
install -m 0755 "$extracted" "$installed"

metadata="$install_dir/.loki-install.json"
cat > "$metadata" <<EOF
{
  "version": "$(json_escape "$installed_version")",
  "installed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "install_dir": "$(json_escape "$install_dir")",
  "archive": "$(json_escape "$archive")",
  "store_path": "$(json_escape "$store_path")"
}
EOF

echo "== smoke =="
"$installed" --version >/dev/null
"$installed" doctor --json >/dev/null
"$installed" tui --help >/dev/null

echo "== symlink probe =="
if ! symlink_probe; then
  if [[ "$require_symlink" -eq 1 ]]; then
    die "symlink probe failed"
  fi
  echo "warning: symlink probe failed; symlink activations may not work" >&2
fi

if [[ -n "$store_path" ]]; then
  echo "== configure store =="
  "$installed" store init "$store_path" >/dev/null
fi

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "warning: $install_dir is not on PATH" >&2 ;;
esac

echo "Loki installed"
echo "Version: $installed_version"
echo "Install dir: $install_dir"
echo "Next: $installed doctor"
