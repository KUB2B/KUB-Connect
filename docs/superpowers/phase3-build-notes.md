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
- GUI build: `wails build -tags "wails webkit2_41"` / `wails dev -tags "wails webkit2_41"` (needs Node +
  webkit2gtk-4.1 on Linux Ubuntu 24.04+, WebView2 on Windows, native WebKit on macOS).
- Connector is **proxy mode only** this phase (cross-platform). TUN selection is
  rejected at Connect with a clear message.

## Verified
| Item | Linux dev (WSL2, this box) | Win/mac dev |
|---|---|---|
| `go test ./...` (logbus, app, xrayconf, prior) | PASS | — |
| `go build ./...` default | PASS | — |
| `wails generate module -tags wails` | PASS | — |
| Frontend compile (`npx tsc --noEmit`) | PASS | — |
| `wails build -tags "wails webkit2_41"` | PASS — `build/bin/vless-client` produced | \<fill on run\> |
| Manual GUI flow (add/connect/route/logs/disconnect) | \<fill on Linux desktop or Win/mac\> | \<fill\> |

**Ubuntu 24.04 note:** webkit2gtk-4.0 renamed to 4.1. Install `libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config gcc`, then build with `-tags "wails webkit2_41"` (not just `-tags wails`).

## Deferred to Phase 4 (networking)
- ~~**Embed geosite.dat + geoip.dat in binary:**~~ **DONE.** `data/geoip.dat`+`data/geosite.dat` committed (~29MB); `//go:embed` `geoAssets` in `main.go`; `internal/geoassets.Sync` extracts to `os.UserCacheDir()/vless-client/geo` (skips rewrite when size matches); `startup` sets `XRAY_LOCATION_ASSET` unless already set in env. Unit-tested; no more manual dat copy.
- netcfg for darwin/windows; full default-route TUN with loop avoidance.
- Wire TUN mode into the GUI connector (remove the proxy-only guard).
- Supervise tun2socks `engine.Start/Stop` `log.Fatalf` (GUI must not be killed).
- Kill switch.

## Deferred to Phase 5
- Autostart (Win Run key / macOS LaunchAgent) — `Settings.AutoStart` UI is present but inert.
- Ping/latency test per server.
- Traffic stats (xray StatsService) — up/down speed indicator.
