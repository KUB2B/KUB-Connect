#!/usr/bin/env bash
# Fetch the XTLS go-win7 toolchain (a patched Go SDK whose binaries run on
# Windows 7 SP1 / Server 2008 R2, which official Go 1.21+ dropped). Used only
# for the separate Windows 7 release artifact; the normal build uses stock Go.
#
# Pin note: GO_WIN7_RELEASE must track go.mod's Go directive — when go.mod bumps
# a Go minor (e.g. 1.26 -> 1.27), bump this to the matching XTLS patched-1.N.x
# release, or the win7 binary drifts from the mainline one. The patched Go's
# minor must be >= go.mod's directive.
# Releases: https://github.com/XTLS/go-win7/releases
set -euo pipefail

GO_WIN7_RELEASE="${GO_WIN7_RELEASE:-patched-1.26.4}"
GO_WIN7_DIR="${GO_WIN7_DIR:-.go-win7}"
ASSET="go-for-win7-linux-amd64.zip"
URL="https://github.com/XTLS/go-win7/releases/download/${GO_WIN7_RELEASE}/${ASSET}"
# Optional integrity pin: export GO_WIN7_ZIP_SHA256=<hash> to enforce.
ZIP_SHA256="${GO_WIN7_ZIP_SHA256:-}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
case "$GO_WIN7_DIR" in
  /*) DEST="$GO_WIN7_DIR" ;;          # absolute: use as-is
  *)  DEST="${ROOT}/${GO_WIN7_DIR}" ;; # relative: anchor to repo root
esac

if [ -x "${DEST}/bin/go" ]; then
  echo "go-win7 toolchain already present: ${DEST}"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fSL -o "$TMP/go-win7.zip" "$URL"

echo "Downloaded zip SHA256:"
sha256sum "$TMP/go-win7.zip"
if [ -n "$ZIP_SHA256" ]; then
  echo "Verifying against GO_WIN7_ZIP_SHA256"
  echo "${ZIP_SHA256}  ${TMP}/go-win7.zip" | sha256sum -c -
fi

rm -rf "$DEST"
mkdir -p "$DEST"
unzip -o -q "$TMP/go-win7.zip" -d "$DEST"
# The zip roots at "go/"; flatten so $DEST is a valid GOROOT.
if [ -d "${DEST}/go" ]; then
  shopt -s dotglob
  mv "${DEST}/go"/* "${DEST}/"
  rmdir "${DEST}/go"
fi
# zip extraction drops the exec bit on the toolchain binaries.
chmod -R +x "${DEST}/bin" "${DEST}/pkg/tool" 2>/dev/null || true

echo "Installed go-win7 toolchain: ${DEST}"
"${DEST}/bin/go" version
