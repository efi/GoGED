#!/usr/bin/env bash
# Regenerates licenses/third_party.txt with the license texts of the Go
# standard library and every module compiled into goged. Run it after
# changing dependencies.
set -euo pipefail

cd "$(dirname "$0")/.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

go build -o "$tmp/goged" .
modcache="$(go env GOMODCACHE)"

{
	echo "goged includes the following third-party software."
	echo
	echo "== Go standard library"
	echo
	cat "$(go env GOROOT)/LICENSE"
	go version -m "$tmp/goged" | awk '$1 == "dep" { print $2, $3 }' | while read -r mod ver; do
		# Module paths are case-encoded in the module cache.
		dir="$modcache/$(printf '%s' "$mod" | sed 's/[A-Z]/!\L&/g')@$ver"
		license="$(ls "$dir"/LICENSE* "$dir"/COPYING* 2>/dev/null | head -n 1 || true)"
		if [ -z "$license" ]; then
			echo "no license file found for $mod $ver" >&2
			exit 1
		fi
		echo
		echo "== $mod $ver"
		echo
		cat "$license"
	done
} > licenses/third_party.txt
echo "wrote licenses/third_party.txt"
