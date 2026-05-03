#!/usr/bin/env bash
set -euo pipefail

mkdir -p .testbin dist

targets=(
  windows/amd64
  windows/arm64
  darwin/amd64
  darwin/arm64
  linux/amd64
  linux/arm64
)

for target in "${targets[@]}"; do
  GOOS=${target%/*}
  GOARCH=${target#*/}
  ext=""
  if [[ "$GOOS" == "windows" ]]; then
    ext=".exe"
  fi

  echo "== $GOOS/$GOARCH package test compile =="
  while IFS= read -r pkg; do
    name=$(printf '%s' "$pkg" | tr '/.' '__')
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go test -c -o ".testbin/${GOOS}-${GOARCH}-${name}${ext}" "$pkg"
  done < <(go list ./...)

  echo "== $GOOS/$GOARCH build =="
  CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -o "dist/loki-${GOOS}-${GOARCH}${ext}" ./cmd/loki
done

echo "cross-platform compile matrix complete"
