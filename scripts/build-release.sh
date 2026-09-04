#!/bin/sh
set -eu

root="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$root"

version="${VERSION:-dev}"
commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
date="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
outdir="${OUTDIR:-"$root/dist"}"
mkdir -p "$outdir"

pkg="github.com/yoyooyooo/commandcode-bridge/internal/buildinfo"
ldflags="-s -w -X ${pkg}.Version=${version} -X ${pkg}.Commit=${commit} -X ${pkg}.Date=${date}"

targets="${TARGETS:-darwin/arm64 darwin/amd64 linux/amd64 linux/arm64}"

for target in $targets; do
	goos="${target%/*}"
	goarch="${target#*/}"
	name="commandcode-bridge_${version}_${goos}_${goarch}"
	echo "building $name"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="$ldflags" \
		-o "$outdir/$name" ./cmd/commandcode-bridge
done

(
	cd "$outdir"
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 commandcode-bridge_"${version}"_* > SHA256SUMS
	else
		sha256sum commandcode-bridge_"${version}"_* > SHA256SUMS
	fi
)

echo "wrote $outdir"
