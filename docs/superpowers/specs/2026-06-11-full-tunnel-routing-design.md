# Full-tunnel routing — design

**Date:** 2026-06-11
**Status:** Approved (brainstorm), pending implementation plan
**Topic:** Route all traffic through the VPN (full tunnel) as an alternative to the current whitelist model.

## Problem

KUB Connect today routes selectively (whitelist): by default everything goes direct,
and only explicitly chosen destinations (Telegram, custom proxy domains/IPs) go through
the VPN. Users need the opposite option — send **all** traffic through the VPN — for both
the Proxy and TUN capture modes.

## Decisions (from brainstorm)

- **Presentation:** a routing-mode selector "Whitelist ↔ Всё через VPN". Applies to both
  Proxy and TUN. In full mode the whitelist-only controls (Telegram toggle, custom proxy
  domains/IPs) are hidden/ignored.
- **Exceptions in full mode:** LAN/private networks always stay direct; the RU-Direct toggle
  remains available (so a user can send "everything except RU" through the VPN).
- **IPv6:** blocked (null-routed) while connected in full mode, to prevent leaks. Traffic
  falls back to IPv4 through the tunnel.
- **TUN mechanism:** Approach A — split-default routes (`0.0.0.0/1` + `128.0.0.0/1`) into the
  TUN plus a per-server `/32` bypass via the physical gateway. The OS default route is left
  intact, so teardown (and crash recovery on reboot) is clean.

## Architecture

The routing mode lives on `routing.Profile` because it changes **both** the xray routing
rules and the set of routes installed into the TUN.

```go
// internal/routing/profile.go
type Profile struct {
    Full               bool     // full-tunnel: route everything through the VPN
    Telegram           bool
    ForceRUDirect      bool
    CustomProxyDomains []string
    CustomProxyIPs     []string
}
```

Persisted inside `store.State.Profile` (already persisted); JSON tag `full`. Default `false`.
Edited via the existing `UpdateProfile` path.

### 1. xray routing rules — `routing.Profile.Rules()`

When `Full` is set, `Rules()` returns the full-tunnel rule set (the loop guard is still
prepended by `xrayconf.withLoopGuard`):

```
RU-Direct (if ForceRUDirect):
    geosite:category-ru -> direct
    geoip:ru            -> direct
geoip:private           -> direct      (LAN always direct)
catch-all tcp,udp       -> proxy       (the inversion vs whitelist)
```

Telegram and custom-proxy rules are **not** emitted in full mode — they are subsumed by the
catch-all proxy rule. Whitelist mode is unchanged (catch-all stays `direct`).

In **Proxy** capture mode this rule set is sufficient on its own: the system SOCKS proxy
captures app traffic and the catch-all proxy rule sends it all to the server. No netcfg work
is required for Proxy full-tunnel.

### 2. TUN routing — `netcfg`

Extend `netcfg.Config`:

```go
type Config struct {
    Device     string
    TunIP      string
    Prefix     int
    RouteCIDRs []string // whitelist mode: selective CIDRs into the TUN
    FullTunnel bool      // full mode: redirect-gateway + bypass + ipv6 block
    ServerIPs  []string  // server IPs to bypass via the physical gateway (full mode)
    BlockIPv6  bool       // null-route IPv6 while up (full mode)
}
```

**Up (full mode):**
1. Set the TUN adapter IP (same as whitelist mode).
2. **Bypass:** for each `ServerIP`, add a host route (`/32`) via the *physical* default
   gateway/interface, so xray's encrypted socket to the server leaves the box normally instead
   of re-entering the TUN (which would form a routing loop).
3. **Redirect:** add `0.0.0.0/1` and `128.0.0.0/1` into the TUN. These outrank the OS default
   route by specificity without deleting it.
4. **IPv6 block:** blackhole `::/1` and `8000::/1` while connected.
   - Linux: `ip -6 route add blackhole ::/1` / `8000::/1`.
   - Windows: netsh route for `::/1` + `8000::/1` to a discard/unreachable next hop.

**Down (full mode):** remove the redirect routes, the bypass routes, and the IPv6 blackhole
routes. The TUN adapter itself is destroyed by tun2socks. All routes are installed
non-persistent (`store=active` on Windows; non-persistent on Linux) so a reboot also clears
them.

Whitelist-mode `Up`/`Down` (selective `RouteCIDRs`) is unchanged.

### 3. Server resolution + gateway discovery (new in `netcfg`)

- **Server IPs:** the active server's host is resolved to IPs via `net.LookupIP` at connect
  time, **before** routes are installed; if the host is already a literal IP it is used as-is.
  Only IPv4 (A) records are added to the bypass set (IPv6 is blocked). For VLESS+Reality this
  is normally a single static IP. `connect.go` populates `Config.ServerIPs`.
- **Default gateway:** a per-OS helper `defaultGateway() (gw, dev string, err error)` discovers
  the current physical default route **before** the TUN routes shadow it.
  - Linux: parse `ip route show default` (`default via <gw> dev <dev>`).
  - Windows: `Get-NetRoute -DestinationPrefix 0.0.0.0/0` (or parse `route print`) for the next
    hop + interface.
  The discovered gateway is captured for use by both the bypass routes (Up) and teardown (Down).

### 4. Wiring — `internal/app/connect.go`

`Connect()` already builds `ConnConfig` and (for TUN) sets `Device`/`TunIP`/`RouteCIDRs`. Add:
when `s.state.Profile.Full` and mode is TUN, resolve server IPs, discover the gateway, and set
`FullTunnel=true`, `ServerIPs=...`, `BlockIPv6=true` on the config instead of the selective
`RouteCIDRs`. The xray config is built from `Profile.Rules()` which already branches on `Full`.

### 5. Frontend — `frontend/`

- `Profile` type gains `full: boolean`.
- Add a "Режим маршрутизации" selector (Whitelist / Всё через VPN) bound to `profile.full`,
  pushed via `UpdateProfile`.
- When full is selected, hide the Telegram toggle and the custom proxy domains/IPs inputs.
  Keep RU-Direct, Kill switch, and Mux visible.

## Safety & interplay

- **Crash recovery:** Approach A never deletes the OS default route, so a crash without
  teardown still leaves working internet; the shadow `/1` routes and IPv6 blackholes are
  non-persistent and clear on reboot.
- **Kill switch in full mode:** the routed set becomes "everything", so kill-switch semantics
  become "block all traffic that leaves the tunnel". It must **not** block the server bypass
  connection. Whether `internal/firewall` already handles a `0.0.0.0/0` routed set (or needs a
  server-IP allow rule) is finalized in the implementation plan.
- **DNS:** in full mode IPv4 DNS enters the TUN and is resolved by xray (`1.1.1.1`). Known edge:
  a DNS query addressed to a LAN resolver (e.g. the router) goes direct under the LAN exception
  — a minor query-path leak. Documented; not addressed in this iteration.

## Open risks (resolved during planning, not now)

1. Exact Windows IPv6-block primitive (netsh discard route vs WFP/firewall rule).
2. Whether `internal/firewall` kill switch needs changes for the full `0.0.0.0/0` routed set and
   the server-IP allow exception.
3. Multi-IP / CDN-fronted servers: the bypass set is resolved once at connect; rotating server
   IPs are out of scope for this iteration.

## Testing

- `routing.Rules()` full-mode table tests: rule order (loop guard, optional RU-Direct, private
  direct, catch-all proxy) and that Telegram/custom rules are omitted.
- `netcfg` command-builder tests (pure builders, run on any OS): full-mode `Up` emits the two
  `/1` redirect routes, the per-server bypass routes, and the IPv6 blackholes; `Down` removes
  exactly those.
- Gateway-parse tests against sample `ip route show default` and `route print` / `Get-NetRoute`
  output.
- `xrayconf.Build` snapshot for a full-mode profile.
- Existing whitelist tests must stay green (regression guard for the default path).

## Scope

Files touched: `internal/routing/profile.go`, `internal/xrayconf/build.go` (only if rule
plumbing needs it; likely no change since it consumes `Rules()`), `internal/netcfg/*`,
`internal/app/connect.go` (+ `ConnConfig`/types), `frontend/` (Profile type, settings UI),
plus tests. Proxy full-tunnel is rules-only; TUN full-tunnel is the bulk of the work.
