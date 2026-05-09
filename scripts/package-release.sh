#!/usr/bin/env bash
set -euo pipefail

version="${1:-${VERSION:-}}"
out_dir="${2:-dist/packages}"

if [[ -z "$version" ]]; then
  echo "usage: $0 <version> [out-dir]" >&2
  exit 2
fi
if [[ ! "$version" =~ ^[A-Za-z0-9._+-]+$ ]]; then
  echo "invalid version: $version" >&2
  exit 2
fi

repo_root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$repo_root"

module="github.com/asudbring/loki-profile-manager"
ldflags="-s -w -X ${module}/internal/app.Version=${version}"
go_bin="${GO_BIN:-${GO:-go}}"
if ! command -v "$go_bin" >/dev/null 2>&1; then
  echo "go toolchain not found; set GO_BIN=/path/to/go" >&2
  exit 1
fi

installer_assets=(
  scripts/install.ps1
  scripts/uninstall.ps1
  scripts/install.sh
  scripts/uninstall.sh
)
for asset in "${installer_assets[@]}"; do
  if [[ ! -f "$asset" ]]; then
    echo "missing installer asset: $asset" >&2
    exit 1
  fi
done

targets=(
  linux/amd64
  linux/arm64
  darwin/amd64
  darwin/arm64
  windows/amd64
  windows/arm64
)

rm -rf "$out_dir"
mkdir -p "$out_dir"
out_dir=$(cd "$out_dir" && pwd)

work_root=$(mktemp -d)
cleanup() {
  rm -rf "$work_root"
}
trap cleanup EXIT

release_assets=()

for target in "${targets[@]}"; do
  goos=${target%/*}
  goarch=${target#*/}
  ext=""
  archive_ext="tar.gz"
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
    archive_ext="zip"
  fi

  name="loki_${version}_${goos}_${goarch}"
  work_dir="$work_root/$name"
  mkdir -p "$work_dir"

  echo "== build $goos/$goarch =="
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch "$go_bin" build -trimpath -ldflags "$ldflags" -o "$work_dir/loki${ext}" ./cmd/loki
  cp README.md CHANGELOG.md "$work_dir/"

  if [[ "$goos" == "windows" ]]; then
    cp scripts/install.ps1 scripts/uninstall.ps1 "$work_dir/"
    (cd "$work_dir" && zip -q "$out_dir/${name}.zip" "loki${ext}" install.ps1 uninstall.ps1 README.md CHANGELOG.md)
    release_assets+=("${name}.zip")
  else
    cp scripts/install.sh scripts/uninstall.sh "$work_dir/"
    chmod 0755 "$work_dir/install.sh" "$work_dir/uninstall.sh"
    tar -czf "$out_dir/${name}.tar.gz" -C "$work_dir" "loki${ext}" install.sh uninstall.sh README.md CHANGELOG.md
    release_assets+=("${name}.tar.gz")
  fi
done

cp scripts/install.ps1 "$out_dir/install.ps1"
cp scripts/uninstall.ps1 "$out_dir/uninstall.ps1"
cp scripts/install.sh "$out_dir/install.sh"
cp scripts/uninstall.sh "$out_dir/uninstall.sh"
chmod 0755 "$out_dir/install.sh" "$out_dir/uninstall.sh"

commit=$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go_version=$("$go_bin" version)
manifest_assets=("${release_assets[@]}" install.ps1 uninstall.ps1 install.sh uninstall.sh)

python3 - "$out_dir" "$version" "$commit" "$build_date" "$go_version" "${manifest_assets[@]}" <<'PY'
import hashlib
import json
import os
import sys

out_dir, version, commit, build_date, go_version, *names = sys.argv[1:]
assets = []
for name in names:
    path = os.path.join(out_dir, name)
    with open(path, 'rb') as f:
        digest = hashlib.sha256(f.read()).hexdigest()
    assets.append({
        'name': name,
        'size': os.path.getsize(path),
        'sha256': digest,
    })
manifest = {
    'version': version,
    'commit': commit,
    'build_date': build_date,
    'go_version': go_version,
    'assets': assets,
}
with open(os.path.join(out_dir, 'release-manifest.json'), 'w', encoding='utf-8') as f:
    json.dump(manifest, f, indent=2)
    f.write('\n')
PY

checksum_assets=("${manifest_assets[@]}" release-manifest.json)
(
  cd "$out_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${checksum_assets[@]}" > checksums.txt
  else
    shasum -a 256 "${checksum_assets[@]}" > checksums.txt
  fi
)

echo "release packages written to $out_dir"
