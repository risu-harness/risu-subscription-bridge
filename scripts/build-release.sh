#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=${1:-0.2.0}
mkdir -p dist
for arch in arm64 amd64; do
 mkdir -p "dist/$arch"
 CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=$version" -o "dist/$arch/risu-bridge" ./cmd/risu-bridge
 tar -czf "dist/risu-bridge-darwin-$arch.tar.gz" -C "dist/$arch" risu-bridge
done
(cd dist && shasum -a 256 risu-bridge-darwin-*.tar.gz > SHA256SUMS)
