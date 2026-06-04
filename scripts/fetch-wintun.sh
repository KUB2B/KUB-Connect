#!/usr/bin/env bash
# Fetch the genuine Wintun amd64 DLL into internal/wintundll/ for embedding.
# Run this before building the Windows binary. Requires curl + unzip.
set -euo pipefail

VERSION="${WINTUN_VERSION:-0.14.1}"
URL="https://www.wintun.net/builds/wintun-${VERSION}.zip"
# Optional integrity pin: export WINTUN_ZIP_SHA256=<hash> to enforce. Obtain the
# expected value from wintun.net; this script does not ship a hardcoded hash.
ZIP_SHA256="${WINTUN_ZIP_SHA256:-}"

DEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/internal/wintundll"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fSL -o "$TMP/wintun.zip" "$URL"

echo "Downloaded zip SHA256:"
sha256sum "$TMP/wintun.zip"
if [ -n "$ZIP_SHA256" ]; then
  echo "Verifying against WINTUN_ZIP_SHA256"
  echo "${ZIP_SHA256}  ${TMP}/wintun.zip" | sha256sum -c -
fi

unzip -o -q "$TMP/wintun.zip" -d "$TMP"
cp "$TMP/wintun/bin/amd64/wintun.dll" "$DEST_DIR/wintun_amd64.dll"

echo "Installed $DEST_DIR/wintun_amd64.dll"
sha256sum "$DEST_DIR/wintun_amd64.dll"
