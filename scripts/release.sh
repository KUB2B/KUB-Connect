#!/usr/bin/env bash
# Orchestrate a release build. On Linux: builds Windows. On macOS: builds macOS.
# Publish step is manual (see PUBLISH note printed at the end).
# Usage: VER=v0.1.0 scripts/release.sh
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
export VER

case "$(uname -s)" in
  Linux)  scripts/build-windows.sh ;;
  Darwin) scripts/build-macos.sh ;;
  *) echo "unsupported build host: $(uname -s)" >&2; exit 1 ;;
esac

echo
echo "PUBLISH (run after both Windows and macOS artifacts are in build/release/):"
echo "  gh release create ${VER} build/release/* \\"
echo "    --title \"VLESS Client ${VER}\" --notes-file CHANGELOG.md"
