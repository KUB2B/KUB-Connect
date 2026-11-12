#!/usr/bin/env bash
# Refresh the geo databases embedded into the app (data/geoip.dat,
# data/geosite.dat) from the upstream v2fly releases.
#
# The files are committed so builds are reproducible and offline; run this
# deliberately (e.g. before a release), review the size change, and commit.
# Downloads are checked against the published SHA-256 and against the
# categories the app's routing rules reference before data/ is touched.
#
# Usage: scripts/fetch-geo.sh        Requires curl, sha256sum, go.
set -euo pipefail

GEOIP_URL="${GEOIP_URL:-https://github.com/v2fly/geoip/releases/latest/download/geoip.dat}"
GEOSITE_URL="${GEOSITE_URL:-https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fetch() { # fetch <url> <name>: download and verify against <url>.sha256sum
  local url="$1" name="$2" want got
  echo "Downloading $url"
  curl -fSL --retry 3 -o "$TMP/$name" "$url"
  want="$(curl -fsSL --retry 3 "$url.sha256sum" | awk '{print $1}')"
  got="$(sha256sum "$TMP/$name" | awk '{print $1}')"
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    echo "SHA-256 mismatch for $name: got $got, want ${want:-<none>}" >&2
    exit 1
  fi
}

fetch "$GEOIP_URL" geoip.dat
fetch "$GEOSITE_URL" geosite.dat

echo "Checking that every category the routing rules use is present"
(cd "$ROOT" && GEO_DIR="$TMP" go test -count=1 -run TestShippedGeoDataHasRequiredCategories ./internal/core/)

mv "$TMP/geoip.dat" "$TMP/geosite.dat" "$ROOT/data/"
ls -l "$ROOT/data/"
echo "Updated. Review and commit data/."
