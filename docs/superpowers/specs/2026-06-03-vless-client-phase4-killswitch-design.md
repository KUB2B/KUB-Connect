# Phase 4 — Kill switch (selective, Linux)

**Date:** 2026-06-03
**Status:** approved design

## Goal

Prevent whitelisted traffic from leaking in plaintext past the tunnel when the
TUN capture fails. Under the project's **selective host-route** model, only
whitelisted destination CIDRs (Telegram's published ranges plus the user's
custom proxy IPs) are routed into the TUN; everything else is intentionally
direct. The kill switch therefore protects **only those whitelisted CIDRs** — it
must never block the direct traffic that is meant to bypass the VPN.

User-controlled via a toggle in settings (`Settings.KillSwitch`, already
persisted). Applies in TUN mode only. Linux only this iteration.

## Leak scenario closed

Normal operation: a packet to a whitelisted CIDR is routed to `tun0`
(`oifname == "tun0"`) → tun2socks → SOCKS → xray → server. If tun2socks dies and
`tun0` goes down, the host route for that CIDR becomes invalid and the kernel
falls back to the default route, sending the packet out the physical interface
(`oifname == "eth0"`, etc.) **in plaintext**. The kill switch drops exactly this
case.

## Mechanism

nftables (Ubuntu 24.04 default; `nft` confirmed present). A dedicated table is
created on connect and deleted on disconnect, so the rules are self-contained and
teardown is a single atomic drop.

```
table inet vless_killswitch {
    chain out {
        type filter hook output priority 0; policy accept;
        oifname != "tun0" ip  daddr { <v4 CIDRs> } drop
        oifname != "tun0" ip6 daddr { <v6 CIDRs> } drop
    }
}
```

- `inet` family handles v4 and v6 in one table; CIDRs are split by family, and a
  rule line is emitted only for a family that has at least one CIDR.
- Matching on `oifname != "<device>"` is what makes this selective and
  loop-safe: while the tunnel is healthy the whitelisted packets egress via the
  TUN device and are accepted; only egress via any other interface is dropped.
- Rejected alternatives: iptables (deprecated, scattered rules, messy cleanup);
  blackhole routes (conflict with the host routes and cannot distinguish
  "via TUN" from "via physical").

## Changes by layer

### 1. `internal/firewall` (new package)
```go
type Config struct {
    Device string   // TUN device whose egress is allowed, e.g. "tun0"
    CIDRs  []string // whitelisted destination CIDRs to protect
}
type Firewall interface {
    On(c Config) error
    Off() error
}
func New() Firewall
```
- `firewall_linux.go`: nftables implementation.
  - Pre-flight (mirrors `internal/tun`): `nft` must be in `PATH`; otherwise
    return an error instead of failing mid-apply.
  - Split `CIDRs` into v4/v6 by presence of `:`.
  - `On` builds the ruleset and applies it via `nft -f -` (stdin). Re-applying
    is made idempotent by deleting any existing table first
    (`nft delete table inet vless_killswitch` ignoring "No such file" errors)
    then creating it.
  - `Off` runs `nft delete table inet vless_killswitch`, treating
    "does not exist" as success (idempotent).
- `firewall_other.go` (`//go:build !linux`): `New()` returns a stub whose `On`
  returns `"kill switch not supported on <GOOS> yet"`; `Off` is a no-op.
- Unit-tested: family split, ruleset string assembly (no v6 line when no v6
  CIDRs, correct device name), empty-CIDR handling.

### 2. `internal/tunnel`
- `Deps` gains `Firewall firewall.Firewall`.
- `Config` gains `KillSwitch bool`.
- `Start` (ModeTUN): after `Router.Up` succeeds, if `cfg.KillSwitch` call
  `Firewall.On(firewall.Config{Device: cfg.Device, CIDRs: cfg.RouteCIDRs})`.
  On error, roll back in reverse: `Router.Down`, `Tun.Stop`, `inst.Stop`.
- `Stop` (ModeTUN): call `Firewall.Off()` as the first recorded step (before
  `Router.Down`), so blocking is lifted promptly; errors recorded like the
  others.
- `Firewall.On` is only ever called when `KillSwitch` is true, so a nil
  Firewall dep is acceptable when the toggle is off; the connector wiring will
  nonetheless always supply `firewall.New()`.

### 3. `internal/app`
- `ConnConfig` gains `KillSwitch bool`.
- `Connect` (ModeTUN): set `cc.KillSwitch = s.state.Settings.KillSwitch`. When
  the toggle is on, append a log note that the kill switch is active for
  whitelisted CIDRs.
- Unit-tested with fakes: `KillSwitch` is passed through only in TUN mode and is
  false in proxy mode regardless of the setting; CIDRs reach the config.

### 4. `gui_connector.go`
- Add `Firewall: firewall.New()` to `tunnel.Deps`.
- Pass `KillSwitch: cc.KillSwitch` into `tunnel.Config`.

### 5. Frontend
- Add `<label><input type="checkbox" id="kill-toggle" /> Kill switch (TUN)</label>`
  to the routing/settings section, near the mode select.
- `main.ts`: set its checked state from `st.settings.killSwitch` on load; on
  change, update `current.settings.killSwitch` and call `UpdateSettings`.

## Privilege

`nft` requires root. TUN mode is already gated behind `Elevated()` in
`app.Connect`, so no additional privilege gate is needed.

## Out of scope (this iteration)

- iptables fallback for systems without nftables.
- Kill switch in proxy mode.
- Kill switch on darwin/windows (the `_other.go` stub errors clearly).
- Reacting to tun2socks death at runtime (the kill switch is a static guard that
  holds regardless of process state; active supervision/auto-reconnect is later).

## Verification

- `go test ./...` green: firewall family split + ruleset assembly, tunnel
  Start/Stop ordering and rollback with a fake Firewall, app gating/pass-through.
- `wails build -tags "wails webkit2_41"` compiles.
- Manual e2e on Linux under `sudo` (root-gated, as in Phase 2): connect in TUN
  mode with kill switch on; confirm Telegram CIDR reachable via tunnel; kill the
  tun2socks engine / bring `tun0` down; confirm packets to a Telegram CIDR are
  dropped (no plaintext leak) while unrelated direct traffic still works;
  disconnect and confirm the `vless_killswitch` table is gone.
