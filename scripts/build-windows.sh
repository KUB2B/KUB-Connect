#!/usr/bin/env bash
# Cross-compile the Windows GUI from Linux and produce an NSIS installer.
# Usage: VER=v0.1.0 scripts/build-windows.sh
#        VER=v0.1.0 WIN7=1 scripts/build-windows.sh   # Windows 7 artifact
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
OUT="build/release"
mkdir -p "$OUT"

# WIN7=1 builds the separate Windows 7 artifact with the XTLS go-win7 toolchain
# (official Go 1.21+ binaries crash on Win7). Everything else — NSIS, WebView2
# macro, wintun, app code — is identical to the mainline Windows build.
SUFFIX="windows"
if [ "${WIN7:-0}" = "1" ]; then
  SUFFIX="windows7"
  scripts/fetch-go-win7.sh
  GOROOT="$(cd "${GO_WIN7_DIR:-.go-win7}" && pwd)"
  export GOROOT
  export PATH="${GOROOT}/bin:${PATH}"
fi

# wails -nsis silently skips the installer (exit 0) when makensis is absent,
# which would make the cp below fail with a confusing "no such file". Fail loud.
command -v makensis >/dev/null || { echo "makensis missing (install: apt install nsis)" >&2; exit 1; }

wails build -platform windows/amd64 -tags wails \
  -ldflags "-X main.version=${VER}" -nsis

cp build/bin/kub-connect-amd64-installer.exe \
   "${OUT}/kub-connect-${VER}-${SUFFIX}-amd64-installer.exe"

echo "Windows artifact: ${OUT}/kub-connect-${VER}-${SUFFIX}-amd64-installer.exe (UNSIGNED)"
