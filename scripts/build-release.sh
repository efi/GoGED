#!/usr/bin/env bash
# Builds release binaries for all supported platforms into dist/.
#
# Every file name carries the build date (UTC), e.g.
#   goged-2026-10-08-linux-amd64
#   goged-2026-10-08-windows-amd64.exe
#   goged-2026-10-08-macos-universal   (Intel and Apple Silicon in one file)
#
# Environment:
#   BUILD_DATE  override the date (YYYY-MM-DD), defaults to today in UTC
#   DIST        output directory, defaults to dist
set -euo pipefail

cd "$(dirname "$0")/.."

DATE="${BUILD_DATE:-$(date -u +%Y-%m-%d)}"
DIST="${DIST:-dist}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
VERSION="${DATE}-${COMMIT}"
# makefat combines Mach-O binaries into a universal binary on any OS.
MAKEFAT="github.com/randall77/makefat@v0.0.0-20260406194835-1b91746796b7"

rm -rf "$DIST"
mkdir -p "$DIST"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

build() { # GOOS GOARCH OUTPUT
	echo "building $3"
	CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath \
		-ldflags "-s -w -X main.version=${VERSION}" -o "$3" .
}

build linux amd64 "$DIST/goged-${DATE}-linux-amd64"
build linux arm64 "$DIST/goged-${DATE}-linux-arm64"
build windows amd64 "$DIST/goged-${DATE}-windows-amd64.exe"
build windows arm64 "$DIST/goged-${DATE}-windows-arm64.exe"
build darwin amd64 "$work/goged-darwin-amd64"
build darwin arm64 "$work/goged-darwin-arm64"

echo "creating $DIST/goged-${DATE}-macos-universal"
go run "$MAKEFAT" "$DIST/goged-${DATE}-macos-universal" "$work/goged-darwin-amd64" "$work/goged-darwin-arm64"
chmod +x "$DIST"/goged-*

(cd "$DIST" && sha256sum goged-* > "goged-${DATE}-SHA256SUMS.txt")

echo "version ${VERSION}"
ls -l "$DIST"
