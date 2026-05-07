#!/usr/bin/env bash
set -euo pipefail

version="${1:-${VERSION:-}}"
release_dir="${2:-dist/packages}"

if [[ -z "$version" ]]; then
  echo "usage: $0 <version> [release-dir]" >&2
  exit 2
fi
if [[ ! "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?(\+[A-Za-z0-9._-]+)?$ ]]; then
  echo "invalid npm-compatible release version: $version" >&2
  exit 2
fi

repo_root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$repo_root"

if ! command -v node >/dev/null 2>&1; then
  echo "node not found; install Node.js 18+" >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found; install npm" >&2
  exit 1
fi

release_dir=$(mkdir -p "$release_dir" && cd "$release_dir" && pwd)
manifest="$release_dir/release-manifest.json"
if [[ ! -f "$manifest" ]]; then
  echo "release manifest missing: $manifest" >&2
  echo "run scripts/package-release.sh first" >&2
  exit 1
fi

for path in npm/package.json npm/bin/loki.js README.md CHANGELOG.md; do
  if [[ ! -f "$path" ]]; then
    echo "missing npm package input: $path" >&2
    exit 1
  fi
done

work_root=$(mktemp -d)
cleanup() {
  rm -rf "$work_root"
}
trap cleanup EXIT

package_dir="$work_root/package"
mkdir -p "$package_dir/bin" "$package_dir/vendor"
cp npm/package.json "$package_dir/package.json"
cp npm/bin/loki.js "$package_dir/bin/loki.js"
chmod 0755 "$package_dir/bin/loki.js"
cp README.md CHANGELOG.md "$package_dir/"

python3 - "$version" "$release_dir" "$package_dir" <<'PY'
import json
import os
import re
import stat
import sys
import tarfile
import zipfile
from pathlib import Path

release_version, release_dir, package_dir = sys.argv[1:]
package_version = release_version[1:] if release_version.startswith('v') else release_version
if not re.match(r'^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$', package_version):
    raise SystemExit(f'invalid npm package version after stripping leading v: {package_version}')

pkg_path = Path(package_dir) / 'package.json'
pkg = json.loads(pkg_path.read_text(encoding='utf-8'))
pkg['version'] = package_version
pkg['lokiRelease'] = {'version': release_version}
pkg_path.write_text(json.dumps(pkg, indent=2) + '\n', encoding='utf-8')

matrix = [
    ('linux', 'amd64', 'linux', 'x64', 'loki', 'tar.gz'),
    ('linux', 'arm64', 'linux', 'arm64', 'loki', 'tar.gz'),
    ('darwin', 'amd64', 'darwin', 'x64', 'loki', 'tar.gz'),
    ('darwin', 'arm64', 'darwin', 'arm64', 'loki', 'tar.gz'),
    ('windows', 'amd64', 'win32', 'x64', 'loki.exe', 'zip'),
    ('windows', 'arm64', 'win32', 'arm64', 'loki.exe', 'zip'),
]

release_root = Path(release_dir)
package_root = Path(package_dir)
for release_os, release_arch, npm_os, npm_arch, binary, archive_type in matrix:
    stem = f'loki_{release_version}_{release_os}_{release_arch}'
    archive = release_root / (stem + ('.zip' if archive_type == 'zip' else '.tar.gz'))
    if not archive.exists():
        raise SystemExit(f'missing release archive: {archive}')
    dest_dir = package_root / 'vendor' / f'{npm_os}-{npm_arch}'
    dest_dir.mkdir(parents=True, exist_ok=True)
    dest = dest_dir / binary
    if archive_type == 'zip':
        with zipfile.ZipFile(archive) as zf:
            with zf.open(binary) as src, open(dest, 'wb') as out:
                out.write(src.read())
    else:
        with tarfile.open(archive, 'r:gz') as tf:
            member = tf.getmember(binary)
            extracted = tf.extractfile(member)
            if extracted is None:
                raise SystemExit(f'{binary} missing from {archive}')
            with extracted, open(dest, 'wb') as out:
                out.write(extracted.read())
    dest.chmod(dest.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
PY

pack_json=$(cd "$package_dir" && npm pack --pack-destination "$release_dir" --json)
tarball=$(node -e "const fs=require('fs'); const data=JSON.parse(fs.readFileSync(0,'utf8')); if(!Array.isArray(data)||!data[0]||!data[0].filename) process.exit(1); console.log(data[0].filename)" <<<"$pack_json")

python3 - "$release_dir" "$tarball" <<'PY'
import hashlib
import json
import os
import sys
from pathlib import Path

release_dir, tarball = sys.argv[1:]
release_root = Path(release_dir)
manifest_path = release_root / 'release-manifest.json'
manifest = json.loads(manifest_path.read_text(encoding='utf-8'))

assets = [asset for asset in manifest.get('assets', []) if asset.get('name') != tarball]
path = release_root / tarball
if not path.exists():
    raise SystemExit(f'npm tarball missing: {path}')
digest = hashlib.sha256(path.read_bytes()).hexdigest()
assets.append({
    'name': tarball,
    'size': path.stat().st_size,
    'sha256': digest,
})
manifest['assets'] = assets
manifest_path.write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')

checksum_names = [asset['name'] for asset in assets] + ['release-manifest.json']
lines = []
for name in checksum_names:
    asset_path = release_root / name
    if not asset_path.exists():
        raise SystemExit(f'checksum asset missing: {asset_path}')
    lines.append(f"{hashlib.sha256(asset_path.read_bytes()).hexdigest()}  {name}\n")
(release_root / 'checksums.txt').write_text(''.join(lines), encoding='utf-8')
PY

echo "npm package written to $release_dir/$tarball"
