# Phase 3 build/verification notes

## What shipped
- `internal/logbus`: capped fan-out line buffer + poll-based file tailer (unit-tested).
- `internal/app`: GUI service layer — state machine (disconnected/connecting/
  connected/disconnecting/error), server CRUD, profile/settings updates,
  connect/disconnect, log streaming, auto-connect. Fully unit-tested with fakes.
- `internal/xrayconf`: optional `LogFile` (xray error log → tailable file).
- Wails GUI (`main.go`, `gui_app.go`, `gui_connector.go`, `frontend/`), all Go
  GUI files behind `//go:build wails`. Frontend: vanilla TS.

## Build model
- Default build/test (no toolchain beyond Go): `go build ./... && go test ./...`
  is green; root `main` is the stub (`main_stub.go`).
- GUI build: `wails build -tags wails` / `wails dev -tags wails` (needs Node +
  webkit2gtk on Linux, WebView2 on Windows, native WebKit on macOS).
- Connector is **proxy mode only** this phase (cross-platform). TUN selection is
  rejected at Connect with a clear message.

## Verified
| Item | Linux dev (WSL2, this box) | Win/mac dev |
|---|---|---|
| `go test ./...` (logbus, app, xrayconf, prior) | PASS | — |
| `go build ./...` default | PASS | — |
| `wails generate module -tags wails` | PASS | — |
| Frontend compile (`npx tsc --noEmit`) | PASS | — |
| `wails build -tags wails` | FAIL: `pkg-config` not found (webkit2gtk missing on WSL2) | \<fill on run\> |
| Manual GUI flow (add/connect/route/logs/disconnect) | \<fill on Linux desktop or Win/mac\> | \<fill\> |

**WSL2 build failure detail:** `github.com/wailsapp/wails/v2/pkg/assetserver/webview: exec: "pkg-config": executable file not found in $PATH` — needs `libgtk-3-dev libwebkit2gtk-4.0-dev` (or `4.1-dev`). Install with `sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config` to build on Linux desktop.

## Deferred to Phase 4 (networking)
- netcfg for darwin/windows; full default-route TUN with loop avoidance.
- Wire TUN mode into the GUI connector (remove the proxy-only guard).
- Supervise tun2socks `engine.Start/Stop` `log.Fatalf` (GUI must not be killed).
- Kill switch.

## Deferred to Phase 5
- Autostart (Win Run key / macOS LaunchAgent) — `Settings.AutoStart` UI is present but inert.
- Ping/latency test per server.
- Traffic stats (xray StatsService) — up/down speed indicator.
