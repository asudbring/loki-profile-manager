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

module="github.com/allensu/loki-profile-manager"
ldflags="-s -w -X ${module}/internal/app.Version=${version}"

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
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags "$ldflags" -o "$work_dir/loki${ext}" ./cmd/loki
  cp README.md CHANGELOG.md "$work_dir/"

  if [[ "$archive_ext" == "zip" ]]; then
    (cd "$work_dir" && zip -q "$out_dir/${name}.zip" "loki${ext}" README.md CHANGELOG.md)
  else
    tar -czf "$out_dir/${name}.tar.gz" -C "$work_dir" "loki${ext}" README.md CHANGELOG.md
  fi
done

(
  cd "$out_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum loki_* > checksums.txt
  else
    shasum -a 256 loki_* > checksums.txt
  fi
)

echo "release packages written to $out_dir"
