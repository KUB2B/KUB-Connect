#!/usr/bin/env bash
# Cross-compile the Windows GUI from Linux and produce an NSIS installer.
# Usage: VER=v0.1.0 scripts/build-windows.sh
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
OUT="build/release"
mkdir -p "$OUT"

# wails -nsis silently skips the installer (exit 0) when makensis is absent,
# which would make the cp below fail with a confusing "no such file". Fail loud.
command -v makensis >/dev/null || { echo "makensis missing (install: apt install nsis)" >&2; exit 1; }

wails build -platform windows/amd64 -tags wails \
  -ldflags "-X main.version=${VER}" -nsis

cp build/bin/kub-connect-amd64-installer.exe \
   "${OUT}/kub-connect-${VER}-windows-amd64-installer.exe"

echo "Windows artifact: ${OUT}/kub-connect-${VER}-windows-amd64-installer.exe (UNSIGNED)"
