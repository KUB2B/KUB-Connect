# Phase 4 — Windows TUN routing (netcfg)

**Date:** 2026-06-04
**Status:** approved design

## Goal

Implement real `internal/netcfg` routing for Windows so TUN mode steers the
whitelisted destination CIDRs into the tunnel, matching the existing Linux
behaviour. macOS remains the unsupported stub (no hardware to verify). This
keeps the selective host-route model: only whitelisted CIDRs enter the TUN; the
default route is untouched.

## Background (verified against the vendored library)

The project uses `github.com/xjasonlyu/tun2socks/v2 v2.6.1-0.20260507...`. On
Windows its TUN device is created via wireguard-go's Wintun
(`tun.CreateTUN(name, mtu)`), where `name` is the `Device` host we pass
(`"tun://tun0"` → adapter name `"tun0"`). The library creates the adapter but
does **not** assign an IP address or add routes — exactly as on Linux, where
`internal/netcfg` performs that step. So the Windows router has the same
responsibility as the Linux one: assign the TUN IP and route whitelisted CIDRs
into the adapter.

Ordering is already correct: `tunnel.Start` calls `Tun.Start` (creates the
adapter) before `Router.Up` (configures it), and `tunnel.Stop` calls
`Router.Down` before `Tun.Stop`. Administrator privilege is already required for
TUN mode via `app.Connect`'s `Elevated()` gate, and
`privilege.IsElevated()` has a real Windows implementation.

## Mechanism

Use the built-in `netsh` CLI, mirroring the Linux iproute2 approach: pure
command-builder functions return `[][]string`, and a shared `runAll` executes
them. The adapter is referenced by its name (`tun0`); arguments are passed as
separate exec tokens, so no shell quoting is involved.

### Up commands (in order)
1. IPv4 address on the adapter:
   `netsh interface ipv4 set address name=tun0 static 198.18.0.1 255.254.0.0`
   (the dotted mask is derived from `Config.Prefix` via a helper).
2. For each IPv4 whitelisted CIDR:
   `netsh interface ipv4 add route <cidr> interface=tun0 store=active`
3. For each IPv6 whitelisted CIDR:
   `netsh interface ipv6 add route <cidr> interface=tun0 store=active`

`store=active` keeps the routes non-persistent (cleared on reboot), so a crash
cannot leave stale routes behind. IPv6 CIDRs are routed by interface (on-link
through the adapter) without assigning an IPv6 address to the adapter, matching
how the Linux code routes v6 CIDRs with a device-scoped route.

### Down commands (in order)
1. For each IPv4 CIDR: `netsh interface ipv4 delete route <cidr> interface=tun0`
2. For each IPv6 CIDR: `netsh interface ipv6 delete route <cidr> interface=tun0`

The adapter's IP address disappears when the Wintun adapter is destroyed at
`Tun.Stop`, so `Down` only needs to remove the routes it added.

### Prefix-to-mask
`prefixToMaskV4(prefix int) string` converts a prefix length (e.g. 15) to a
dotted IPv4 mask (e.g. `255.254.0.0`). Pure and unit-tested.

### CIDR family split
CIDRs are partitioned into IPv4 and IPv6 by the presence of a colon, so the
correct `netsh` address family is used per entry.

## Changes by layer

### 1. `internal/netcfg/netcfg_windows_cmds.go` (new, no build tag)
Pure builders, compiled and unit-tested on any OS (including the dev Linux
machine):
- `winUpCommands(c Config) [][]string`
- `winDownCommands(c Config) [][]string`
- `prefixToMaskV4(prefix int) string`
- a small CIDR family-split helper

No build tag means these functions exist on all platforms; they are simply
unused on Linux/macOS, which Go permits for functions.

### 2. `internal/netcfg/netcfg_exec.go` (new, no build tag)
Move the existing `runAll(cmds [][]string) error` here from
`netcfg_linux.go` so both the Linux and Windows routers share one
implementation. Remove `runAll` from `netcfg_linux.go`.

### 3. `internal/netcfg/netcfg_windows.go` (new, `//go:build windows`)
```go
type windowsRouter struct{}
func New() Router { return windowsRouter{} }
func (windowsRouter) Up(c Config) error   { return runAll(winUpCommands(c)) }
func (windowsRouter) Down(c Config) error { return runAll(winDownCommands(c)) }
```

### 4. `internal/netcfg/netcfg_other.go`
Change the build constraint from `//go:build !linux` to
`//go:build !linux && !windows`, so Windows uses the new router and macOS keeps
the existing "not supported on <GOOS> yet" stub.

### 5. `internal/netcfg/netcfg_windows_cmds_test.go` (new, no build tag)
Table-style tests for `winUpCommands`, `winDownCommands`, and
`prefixToMaskV4`, plus mixed v4/v6 CIDR coverage. Runs under `go test ./...` on
the Linux dev machine.

## Out of scope (this iteration)

- macOS routing (stub remains).
- Assigning an IPv6 address to the TUN adapter.
- Kill switch on Windows (the `internal/firewall` stub already errors clearly;
  separate iteration).
- Geosite domains inside TUN (known limitation of the host-route model).

## Verification

- `go test ./internal/netcfg/` green on Linux: the untagged builder tests cover
  command assembly and mask conversion.
- `GOOS=windows go build ./...` compiles; `GOOS=darwin go build ./...` compiles
  (stub).
- Manual e2e on a Windows host under Administrator (cannot run in WSL2/CI):
  connect in TUN mode with Telegram enabled; confirm the `tun0` Wintun adapter
  gets `198.18.0.1`; confirm `route print` lists the Telegram CIDRs pointing at
  the adapter; confirm a Telegram desktop client reaches the network; disconnect
  and confirm the routes are gone.
