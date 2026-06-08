#!/usr/bin/env bash
# Cross-compile the Windows GUI from Linux and produce an NSIS installer.
# Usage: VER=v0.1.0 scripts/build-windows.sh
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
OUT="build/release"
mkdir -p "$OUT"

wails build -platform windows/amd64 -tags wails \
  -ldflags "-X main.version=${VER}" -nsis

cp build/bin/vless-client-amd64-installer.exe \
   "${OUT}/vless-client-${VER}-windows-amd64-installer.exe"

echo "Windows artifact: ${OUT}/vless-client-${VER}-windows-amd64-installer.exe (UNSIGNED)"
