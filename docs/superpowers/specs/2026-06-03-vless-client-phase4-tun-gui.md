# Phase 4 — TUN mode usable from the GUI (Linux)

**Date:** 2026-06-03
**Status:** approved design

## Goal

Enable TUN capture mode from the Wails GUI on Linux, using the existing
**selective host-route** model (only whitelisted CIDRs are routed into the TUN;
the default route is left untouched, so there is no routing loop and no need for
`SO_MARK`/`fwmark`). Keep the cross-platform build green and ensure the GUI
process is not killed by tun2socks' internal `log.Fatalf`.

## Routing model (unchanged, confirmed)

Everything direct by default; only selected destinations enter the TUN. The TUN
receives the proxy-side **IP CIDRs** only: Telegram's published ranges
(`routing.TelegramCIDRs`, baked-in) plus the user's `CustomProxyIPs`. Because
host routes are IP-only, **geosite domains cannot be steered at the host level**
in TUN mode — this is a known, accepted limitation; xray's internal routing
still applies once traffic is inside the tunnel.

Loop avoidance is implicit: the VLESS server IP and xray's own direct/freedom
outbounds use the OS default route (physical interface), never the TUN, because
only whitelisted CIDRs are routed into the TUN.

## Changes by layer

### 1. `internal/tun` — pre-flight validation
Before calling `engine.Start()` (which crashes via `log.Fatalf` on failure),
`Start(device, socksURL)` validates what it can and returns an error instead:
- `socksURL` non-empty, parses, scheme is `socks5`.
- `device` non-empty.
- `/dev/net/tun` exists / is openable (Linux).

`log.Fatalf` itself calls `os.Exit` and cannot be intercepted; pre-flight
removes the common failure causes. Subprocess isolation is explicitly deferred.

### 2. `internal/netcfg` — cross-platform compile
Add `netcfg_other.go` (`//go:build !linux`) whose `New()` returns a router whose
`Up`/`Down` error with `"TUN routing not supported on <GOOS> yet"`. The real
Linux implementation is unchanged. Real darwin/windows routing is a later
iteration.

### 3. `internal/app` — drop the proxy-only guard, build the TUN config
- `ConnConfig` gains `Device, TunIP, TunPrefix, RouteCIDRs`.
- `Connect()`:
  - For `ModeTUN`, require `s.deps.Elevated()`; otherwise error
    `"TUN mode requires administrator/root privileges"`.
  - Compute `RouteCIDRs` = (`routing.TelegramCIDRs` if `Profile.Telegram`) +
    `Profile.CustomProxyIPs`.
  - Defaults: `Device="tun0"`, `TunIP="198.18.0.1"`, `TunPrefix=15`.
  - Log a warning that geosite domains are not host-routed in TUN mode.
- New logic (privilege gate, CIDR assembly) is unit-tested with fakes.

### 4. `gui_connector.go` — assemble TUN deps
When `Mode==ModeTUN`: set `Tun` = `internal/tun` adapter, `Router` =
`netcfg.New()`, and pass the TUN fields through to `tunnel.Config`. Remove the
proxy-only guard.

### 5. Frontend
The TUN `<option>` already exists (was disabled, labelled "Phase 4"). Remove the
`disabled` attribute / "Phase 4" note.

## Out of scope (this iteration)
- Kill switch.
- Real darwin/windows netcfg routing.
- Geosite domains inside TUN.
- Subprocess isolation of tun2socks.

## Verification
- `go test ./...` green (tun validation, app gating/CIDR assembly).
- `wails build -tags "wails webkit2_41"` compiles.
- TUN end-to-end on Linux: manual under `sudo` (root-gated, as in Phase 2).
