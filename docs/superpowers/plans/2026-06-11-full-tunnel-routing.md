# Full-tunnel Routing Implementation Plan (Phase 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "route all traffic through the VPN" (full-tunnel) routing mode alongside the existing whitelist mode, working in both Proxy and TUN capture modes.

**Architecture:** A new boolean `Full` on `routing.Profile` drives two things: (1) the xray routing rules invert so the catch-all sends everything to the proxy (instead of direct), and (2) in TUN mode the OS routing switches from selective host-routes to split-default routes (`0.0.0.0/1` + `128.0.0.0/1`) into the TUN, plus a per-server `/32` bypass via the physical gateway, plus IPv6 capture routes so all v6 reaches xray where it is blackholed. The OS default route is never deleted (clean teardown / crash recovery). Proxy mode needs only the rules change.

**Tech Stack:** Go, xray-core (JSON routing config), tun2socks, iproute2 (Linux), netsh + PowerShell (Windows), Wails + vanilla TypeScript frontend.

**Spec:** `docs/superpowers/specs/2026-06-11-full-tunnel-routing-design.md`

**Out of scope (Phase 2):** A proper full-mode kill switch (rewriting `internal/firewall` to allow server-bypass egress while dropping all other non-tunnel egress). In Phase 1, when `Full` and the kill switch are both on, the kill switch is **skipped with a logged warning** rather than installing the wrong (whitelist-shaped) ruleset.

---

## File Structure

- `internal/routing/profile.go` — add `Full` field + full-tunnel branch in `Rules()`.
- `internal/routing/profile_test.go` — full-mode rule tests.
- `internal/app/types.go` — add `FullTunnel`, `ServerIPs`, `BlockIPv6` to `ConnConfig`.
- `internal/app/connect.go` — resolve server IPs and populate the new `ConnConfig` fields in TUN full mode.
- `internal/app/serverips.go` (new) — `resolveServerIPv4(host string) []string` helper + test.
- `internal/tunnel/tunnel.go` — add fields to `tunnel.Config`, map into `netcfg.Config`, skip kill switch in full mode with a warning.
- `internal/netcfg/netcfg.go` — add `FullTunnel`, `ServerIPs`, `BlockIPv6` to `netcfg.Config`.
- `internal/netcfg/route_cmds.go` (new) — OS-agnostic pure command builders for full mode (Linux + Windows), unit-tested on any OS.
- `internal/netcfg/gateway_linux.go` / `gateway_windows.go` / `gateway_other.go` (new) — default-gateway discovery (exec) + a pure parser each.
- `internal/netcfg/netcfg_linux.go` / `netcfg_windows.go` — branch `Up`/`Down` on `FullTunnel`.
- `gui_connector.go` — forward the new `ConnConfig` fields into `tunnel.Config`.
- `frontend/src/main.ts`, `frontend/index.html`, `frontend/wailsjs/go/models.ts` — routing-mode selector + conditional hiding of whitelist-only controls.

---

## Task 1: Add `Full` to the routing profile and invert the catch-all

**Files:**
- Modify: `internal/routing/profile.go`
- Test: `internal/routing/profile_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/routing/profile_test.go`:

```go
func TestRulesFullTunnel(t *testing.T) {
	p := Profile{Full: true, Telegram: true, ForceRUDirect: true,
		CustomProxyDomains: []string{"example.com"}}
	rules := p.Rules()

	// Last rule is the catch-all and must now go to proxy.
	last := rules[len(rules)-1]
	if last.Outbound != OutboundProxy || last.Network != "tcp,udp" {
		t.Fatalf("full catch-all = %+v, want proxy tcp,udp", last)
	}

	// Telegram and custom-proxy rules are subsumed and must NOT appear.
	for _, r := range rules {
		for _, d := range r.Domains {
			if d == "geosite:telegram" || d == "example.com" {
				t.Errorf("full mode should omit whitelist proxy rule: %+v", r)
			}
		}
	}

	// RU-Direct and private direct exceptions are kept, before the catch-all.
	var sawRU, sawPrivate, sawV6Block bool
	for _, r := range rules {
		if r.Outbound == OutboundDirect {
			for _, ip := range r.IPs {
				if ip == "geoip:ru" {
					sawRU = true
				}
				if ip == "geoip:private" {
					sawPrivate = true
				}
			}
		}
		if r.Outbound == OutboundBlock {
			for _, ip := range r.IPs {
				if ip == "::/0" {
					sawV6Block = true
				}
			}
		}
	}
	if !sawRU || !sawPrivate || !sawV6Block {
		t.Errorf("full mode missing exception/block rules: ru=%v private=%v v6block=%v",
			sawRU, sawPrivate, sawV6Block)
	}
}

func TestRulesWhitelistUnchanged(t *testing.T) {
	// Regression guard: default (whitelist) catch-all stays direct.
	p := Default()
	rules := p.Rules()
	last := rules[len(rules)-1]
	if last.Outbound != OutboundDirect || last.Network != "tcp,udp" {
		t.Fatalf("whitelist catch-all = %+v, want direct tcp,udp", last)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/routing/ -run TestRulesFull -v`
Expected: compile error — `Profile` has no field `Full`.

- [ ] **Step 3: Implement the full-tunnel branch**

In `internal/routing/profile.go`, add the field to `Profile` (first field, documented):

```go
// Profile is the user's routing choices. Full selects the full-tunnel model
// (everything through the VPN); otherwise the whitelist model applies.
type Profile struct {
	Full               bool
	Telegram           bool
	ForceRUDirect      bool
	CustomProxyDomains []string
	CustomProxyIPs     []string
}
```

Then branch at the top of `Rules()`:

```go
func (p Profile) Rules() []Rule {
	if p.Full {
		return p.fullRules()
	}

	var rules []Rule
	// ... existing whitelist body unchanged ...
}

// fullRules is the full-tunnel rule set: keep LAN (and optionally RU) direct,
// blackhole all IPv6 (it is captured into the TUN but the server path is IPv4
// only — see netcfg BlockIPv6), and send everything else to the proxy.
func (p Profile) fullRules() []Rule {
	var rules []Rule
	if p.ForceRUDirect {
		rules = append(rules,
			Rule{Outbound: OutboundDirect, Domains: []string{"geosite:category-ru"}},
			Rule{Outbound: OutboundDirect, IPs: []string{"geoip:ru"}},
		)
	}
	rules = append(rules, Rule{Outbound: OutboundDirect, IPs: []string{"geoip:private"}})
	rules = append(rules, Rule{Outbound: OutboundBlock, IPs: []string{"::/0"}})
	rules = append(rules, Rule{Outbound: OutboundProxy, Network: "tcp,udp"})
	return rules
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/routing/ -v`
Expected: PASS (including the existing whitelist tests).

- [ ] **Step 5: Commit**

```bash
git add internal/routing/profile.go internal/routing/profile_test.go
git commit -m "feat(routing): full-tunnel rule set on Profile.Full"
```

---

## Task 2: Resolve the server's IPv4 addresses (bypass set)

**Files:**
- Create: `internal/app/serverips.go`
- Test: `internal/app/serverips_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/app/serverips_test.go`:

```go
package app

import "testing"

func TestResolveServerIPv4Literal(t *testing.T) {
	got := resolveServerIPv4("203.0.113.7")
	if len(got) != 1 || got[0] != "203.0.113.7" {
		t.Fatalf("literal IPv4 = %v, want [203.0.113.7]", got)
	}
}

func TestResolveServerIPv4LiteralV6Ignored(t *testing.T) {
	// A literal IPv6 server address yields no bypass entries (v6 is blocked).
	if got := resolveServerIPv4("2001:db8::1"); len(got) != 0 {
		t.Fatalf("literal IPv6 = %v, want empty", got)
	}
}

func TestResolveServerIPv4Empty(t *testing.T) {
	if got := resolveServerIPv4(""); got != nil {
		t.Fatalf("empty host = %v, want nil", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/ -run TestResolveServerIPv4 -v`
Expected: compile error — `resolveServerIPv4` undefined.

- [ ] **Step 3: Implement the resolver**

Create `internal/app/serverips.go`:

```go
package app

import "net"

// resolveServerIPv4 returns the IPv4 addresses to bypass the tunnel for (so the
// encrypted connection to the server does not loop back into the TUN). A literal
// IPv4 host is returned as-is; a hostname is resolved via DNS. IPv6 addresses are
// dropped because full-tunnel mode blocks IPv6. Returns nil on empty input or
// resolution failure (the caller proceeds without a bypass, logging upstream).
func resolveServerIPv4(host string) []string {
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return []string{ip4.String()}
		}
		return nil // literal IPv6: nothing to bypass
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		if ip4 := a.To4(); ip4 != nil {
			out = append(out, ip4.String())
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/app/ -run TestResolveServerIPv4 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/serverips.go internal/app/serverips_test.go
git commit -m "feat(app): resolve server IPv4 addresses for tunnel bypass"
```

---

## Task 3: Carry full-tunnel fields through ConnConfig and populate them in Connect

**Files:**
- Modify: `internal/app/types.go:41-52` (`ConnConfig`)
- Modify: `internal/app/connect.go:117-139`
- Test: `internal/app/connect_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/app/connect_test.go`:

```go
func TestConnectTUNFullPopulatesConfig(t *testing.T) {
	svc, _, fc, captured := testDeps(t) // testDeps is elevated
	mustAdd(t, svc, sampleLink)
	// Full-tunnel profile + TUN mode.
	if err := svc.UpdateProfile(ProfileDTO{Full: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !fc.started {
		t.Fatal("connector should have started")
	}
	if !captured.FullTunnel {
		t.Error("ConnConfig.FullTunnel should be true in full TUN mode")
	}
	if !captured.BlockIPv6 {
		t.Error("ConnConfig.BlockIPv6 should be true in full TUN mode")
	}
	if len(captured.ServerIPs) == 0 {
		t.Error("ConnConfig.ServerIPs should be populated from the server host")
	}
}
```

This assumes `sampleLink` points at a literal-IP host. Check the existing `sampleLink` in `internal/app/connect_test.go`; if its host is a domain that may not resolve in CI, define a local link with a literal IP for this test instead:

```go
// At the top of the test, if sampleLink's host is not a literal IP:
const fullSampleLink = "vless://00000000-0000-0000-0000-000000000000@203.0.113.7:443?security=reality&pbk=x&sni=example.com&type=tcp#full"
// and use mustAdd(t, svc, fullSampleLink)
```

Inspect `sampleLink` first and pick whichever keeps `ServerIPs` non-empty without network access.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/ -run TestConnectTUNFull -v`
Expected: compile error — `ConnConfig` has no field `FullTunnel`.

- [ ] **Step 3: Add the fields to ConnConfig**

In `internal/app/types.go`, extend `ConnConfig`:

```go
type ConnConfig struct {
	XrayJSON   []byte
	SocksHost  string
	SocksPort  int
	Mode       store.Mode
	Device     string
	TunIP      string
	TunPrefix  int
	RouteCIDRs []string
	KillSwitch bool
	LogLevel   string
	// Full-tunnel (TUN mode) fields, set only when Profile.Full is enabled.
	FullTunnel bool     // route everything into the TUN (split-default + bypass)
	ServerIPs  []string // server IPv4s to bypass via the physical gateway
	BlockIPv6  bool     // capture + blackhole IPv6 while connected
}
```

- [ ] **Step 4: Populate the fields in Connect**

In `internal/app/connect.go`, inside the `if mode == store.ModeTUN {` block (currently lines 125-139), after the existing `cc.RouteCIDRs = tunRouteCIDRs(...)` assignment, add the full-mode branch:

```go
	if mode == store.ModeTUN {
		cc.Device = tunDevice
		cc.TunIP = tunIP
		cc.TunPrefix = tunPrefix
		cc.RouteCIDRs = tunRouteCIDRs(s.state.Profile)
		if s.state.Profile.Full {
			cc.FullTunnel = true
			cc.BlockIPv6 = true
			cc.ServerIPs = resolveServerIPv4(srv.Host)
			if len(cc.ServerIPs) == 0 {
				s.bus.Append("warning: could not resolve server IP for bypass; full-tunnel may loop")
			}
			cc.RouteCIDRs = nil // split-default replaces selective routes
		}
		s.bus.Append("note: TUN mode routes whitelisted IPs only; geosite domains are not host-routed")
		// ... existing kill-switch block unchanged ...
	}
```

Note `srv` is already in scope (`srv := s.state.Servers[s.state.ActiveServer]`). Leave the existing "routes whitelisted IPs only" note; it is harmless, or optionally guard it with `if !s.state.Profile.Full`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/app/ -run TestConnectTUNFull -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/types.go internal/app/connect.go internal/app/connect_test.go
git commit -m "feat(app): populate full-tunnel ConnConfig fields in Connect"
```

---

## Task 4: Add full-tunnel fields to netcfg.Config and the pure command builders

**Files:**
- Modify: `internal/netcfg/netcfg.go`
- Create: `internal/netcfg/route_cmds.go`
- Test: `internal/netcfg/route_cmds_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/netcfg/route_cmds_test.go`:

```go
package netcfg

import (
	"strings"
	"testing"
)

func joinAll(cmds [][]string) string {
	var lines []string
	for _, c := range cmds {
		lines = append(lines, strings.Join(c, " "))
	}
	return strings.Join(lines, "\n")
}

func fullCfg() Config {
	return Config{
		Device: "tun0", TunIP: "198.18.0.1", Prefix: 30,
		FullTunnel: true, BlockIPv6: true,
		ServerIPs: []string{"203.0.113.7"},
	}
}

func TestLinuxFullUpCommands(t *testing.T) {
	out := joinAll(linuxFullUpCommands(fullCfg(), "192.168.1.1", "eth0"))
	for _, want := range []string{
		"ip route add 203.0.113.7/32 via 192.168.1.1 dev eth0",
		"ip route add 0.0.0.0/1 dev tun0",
		"ip route add 128.0.0.0/1 dev tun0",
		"ip -6 route add ::/1 dev tun0",
		"ip -6 route add 8000::/1 dev tun0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("linux full up missing %q in:\n%s", want, out)
		}
	}
}

func TestLinuxFullDownCommands(t *testing.T) {
	out := joinAll(linuxFullDownCommands(fullCfg()))
	for _, want := range []string{
		"ip route del 0.0.0.0/1",
		"ip route del 128.0.0.0/1",
		"ip route del 203.0.113.7/32",
		"ip -6 route del ::/1",
		"ip -6 route del 8000::/1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("linux full down missing %q in:\n%s", want, out)
		}
	}
}

func TestWinFullUpCommands(t *testing.T) {
	out := joinAll(winFullUpCommands(fullCfg(), "192.168.1.1", "Ethernet"))
	for _, want := range []string{
		`netsh interface ipv4 add route prefix=203.0.113.7/32 interface=Ethernet nexthop=192.168.1.1 store=active`,
		`netsh interface ipv4 add route prefix=0.0.0.0/1 interface=tun0 store=active`,
		`netsh interface ipv4 add route prefix=128.0.0.0/1 interface=tun0 store=active`,
		`netsh interface ipv6 add route prefix=::/1 interface=tun0 store=active`,
		`netsh interface ipv6 add route prefix=8000::/1 interface=tun0 store=active`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("win full up missing %q in:\n%s", want, out)
		}
	}
}

func TestWinFullDownCommands(t *testing.T) {
	out := joinAll(winFullDownCommands(fullCfg(), "Ethernet"))
	for _, want := range []string{
		`netsh interface ipv4 delete route prefix=0.0.0.0/1 interface=tun0`,
		`netsh interface ipv4 delete route prefix=128.0.0.0/1 interface=tun0`,
		`netsh interface ipv4 delete route prefix=203.0.113.7/32 interface=Ethernet`,
		`netsh interface ipv6 delete route prefix=::/1 interface=tun0`,
		`netsh interface ipv6 delete route prefix=8000::/1 interface=tun0`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("win full down missing %q in:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/netcfg/ -run Full -v`
Expected: compile error — builders + `Config` fields undefined.

- [ ] **Step 3: Add the fields to netcfg.Config**

In `internal/netcfg/netcfg.go`:

```go
type Config struct {
	Device     string
	TunIP      string
	Prefix     int
	RouteCIDRs []string // whitelist mode: selective CIDRs into the TUN
	FullTunnel bool      // full mode: split-default + server bypass + ipv6 block
	ServerIPs  []string  // full mode: server IPv4s to bypass via the physical gateway
	BlockIPv6  bool       // full mode: capture IPv6 into the TUN (xray blackholes it)
}
```

- [ ] **Step 4: Implement the pure builders**

Create `internal/netcfg/route_cmds.go`:

```go
// Full-tunnel route command builders. Like the whitelist builders these are
// free of build tags so they compile and unit-test on any OS; the OS-specific
// Routers (netcfg_linux.go / netcfg_windows.go) call them with a discovered
// physical gateway. Approach A: split-default routes (0.0.0.0/1 + 128.0.0.0/1)
// shadow the OS default without deleting it, while the server IPs are pinned to
// the physical gateway so the encrypted tunnel does not loop back into the TUN.
// IPv6 is captured into the TUN (::/1 + 8000::/1) where xray blackholes it.
package netcfg

// linuxFullUpCommands builds the iproute2 commands to enter full-tunnel mode.
// gw/dev are the physical default gateway and its interface.
func linuxFullUpCommands(c Config, gw, dev string) [][]string {
	cmds := [][]string{
		{"ip", "addr", "add", cidr(c.TunIP, c.Prefix), "dev", c.Device},
		{"ip", "link", "set", "dev", c.Device, "up"},
	}
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"ip", "route", "add", ip + "/32", "via", gw, "dev", dev})
	}
	cmds = append(cmds,
		[]string{"ip", "route", "add", "0.0.0.0/1", "dev", c.Device},
		[]string{"ip", "route", "add", "128.0.0.0/1", "dev", c.Device},
	)
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"ip", "-6", "route", "add", "::/1", "dev", c.Device},
			[]string{"ip", "-6", "route", "add", "8000::/1", "dev", c.Device},
		)
	}
	return cmds
}

// linuxFullDownCommands reverses linuxFullUpCommands. Routes are matched by
// prefix (gateway not needed for deletion); the TUN address is removed last.
func linuxFullDownCommands(c Config) [][]string {
	var cmds [][]string
	cmds = append(cmds,
		[]string{"ip", "route", "del", "0.0.0.0/1"},
		[]string{"ip", "route", "del", "128.0.0.0/1"},
	)
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"ip", "route", "del", ip + "/32"})
	}
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"ip", "-6", "route", "del", "::/1"},
			[]string{"ip", "-6", "route", "del", "8000::/1"},
		)
	}
	cmds = append(cmds, []string{"ip", "addr", "del", cidr(c.TunIP, c.Prefix), "dev", c.Device})
	return cmds
}

// winFullUpCommands builds the netsh commands to enter full-tunnel mode.
// gw/dev are the physical default gateway IP and its interface alias.
func winFullUpCommands(c Config, gw, dev string) [][]string {
	cmds := [][]string{
		{"netsh", "interface", "ipv4", "set", "address", "name=" + c.Device, "static", c.TunIP, prefixToMaskV4(c.Prefix)},
	}
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "add", "route",
			"prefix=" + ip + "/32", "interface=" + dev, "nexthop=" + gw, "store=active"})
	}
	cmds = append(cmds,
		[]string{"netsh", "interface", "ipv4", "add", "route", "prefix=0.0.0.0/1", "interface=" + c.Device, "store=active"},
		[]string{"netsh", "interface", "ipv4", "add", "route", "prefix=128.0.0.0/1", "interface=" + c.Device, "store=active"},
	)
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"netsh", "interface", "ipv6", "add", "route", "prefix=::/1", "interface=" + c.Device, "store=active"},
			[]string{"netsh", "interface", "ipv6", "add", "route", "prefix=8000::/1", "interface=" + c.Device, "store=active"},
		)
	}
	return cmds
}

// winFullDownCommands reverses winFullUpCommands. dev is the physical interface
// alias (needed to delete the server-bypass routes). The TUN adapter's address
// disappears when tun2socks destroys the adapter, so only routes are removed.
func winFullDownCommands(c Config, dev string) [][]string {
	cmds := [][]string{
		{"netsh", "interface", "ipv4", "delete", "route", "prefix=0.0.0.0/1", "interface=" + c.Device},
		{"netsh", "interface", "ipv4", "delete", "route", "prefix=128.0.0.0/1", "interface=" + c.Device},
	}
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "delete", "route",
			"prefix=" + ip + "/32", "interface=" + dev})
	}
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"netsh", "interface", "ipv6", "delete", "route", "prefix=::/1", "interface=" + c.Device},
			[]string{"netsh", "interface", "ipv6", "delete", "route", "prefix=8000::/1", "interface=" + c.Device},
		)
	}
	return cmds
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/netcfg/ -run Full -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/netcfg/netcfg.go internal/netcfg/route_cmds.go internal/netcfg/route_cmds_test.go
git commit -m "feat(netcfg): full-tunnel route command builders + config fields"
```

---

## Task 5: Default-gateway discovery (Linux parser + exec)

**Files:**
- Create: `internal/netcfg/gateway_linux.go`
- Create: `internal/netcfg/gateway_other.go`
- Test: `internal/netcfg/gateway_linux_test.go`

Note: the **parser** is build-tag-free so it tests on any OS; the exec wrapper is Linux-tagged.

- [ ] **Step 1: Write the failing test**

Create `internal/netcfg/gateway_linux_test.go`:

```go
package netcfg

import "testing"

func TestParseLinuxDefaultRoute(t *testing.T) {
	cases := []struct {
		in, gw, dev string
		ok          bool
	}{
		{"default via 192.168.1.1 dev eth0 proto dhcp metric 100", "192.168.1.1", "eth0", true},
		{"default via 10.0.0.1 dev wlan0\n", "10.0.0.1", "wlan0", true},
		{"", "", "", false},
		{"something unexpected", "", "", false},
	}
	for _, c := range cases {
		gw, dev, err := parseLinuxDefaultRoute(c.in)
		if c.ok && (err != nil || gw != c.gw || dev != c.dev) {
			t.Errorf("parse(%q) = %q,%q,%v want %q,%q,nil", c.in, gw, dev, err, c.gw, c.dev)
		}
		if !c.ok && err == nil {
			t.Errorf("parse(%q) expected error", c.in)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/netcfg/ -run ParseLinuxDefault -v`
Expected: compile error — `parseLinuxDefaultRoute` undefined.

- [ ] **Step 3: Implement the parser + Linux exec**

Create `internal/netcfg/gateway_linux.go`:

```go
//go:build linux

package netcfg

import (
	"fmt"
	"os/exec"
	"strings"
)

// defaultGateway returns the physical default route's gateway IP and interface,
// read from `ip route show default`. Called before the split-default routes
// shadow it, so the original physical path is captured for the server bypass.
func defaultGateway() (gw, dev string, err error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("ip route show default: %w", err)
	}
	return parseLinuxDefaultRoute(string(out))
}

// parseLinuxDefaultRoute extracts the gateway and device from an
// `ip route show default` line: "default via <gw> dev <dev> ...".
func parseLinuxDefaultRoute(s string) (gw, dev string, err error) {
	// Use the first line only (there may be several default routes).
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		switch fields[i] {
		case "via":
			gw = fields[i+1]
		case "dev":
			dev = fields[i+1]
		}
	}
	if gw == "" || dev == "" {
		return "", "", fmt.Errorf("no default route found in %q", line)
	}
	return gw, dev, nil
}
```

Create `internal/netcfg/gateway_other.go` (stub for non-linux, non-windows builds so the package compiles everywhere; the parser lives in the OS-specific files):

```go
//go:build !linux && !windows

package netcfg

import "fmt"

func defaultGateway() (gw, dev string, err error) {
	return "", "", fmt.Errorf("default gateway discovery not supported on this OS")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/netcfg/ -run ParseLinuxDefault -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/netcfg/gateway_linux.go internal/netcfg/gateway_other.go internal/netcfg/gateway_linux_test.go
git commit -m "feat(netcfg): linux default-gateway discovery"
```

---

## Task 6: Default-gateway discovery (Windows parser + exec)

**Files:**
- Create: `internal/netcfg/gateway_windows.go`
- Test: `internal/netcfg/gateway_windows_test.go`

The PowerShell command prints two whitespace-separated tokens — `NextHop InterfaceAlias` — for the lowest-metric default route. The parser is build-tag-free.

- [ ] **Step 1: Write the failing test**

Create `internal/netcfg/gateway_windows_test.go`:

```go
package netcfg

import "testing"

func TestParseWinDefaultRoute(t *testing.T) {
	cases := []struct {
		in, gw, dev string
		ok          bool
	}{
		{"192.168.1.1 Ethernet\n", "192.168.1.1", "Ethernet", true},
		{"  10.0.0.1   Wi-Fi  ", "10.0.0.1", "Wi-Fi", true},
		{"", "", "", false},
		{"0.0.0.0", "", "", false}, // missing interface token
	}
	for _, c := range cases {
		gw, dev, err := parseWinDefaultRoute(c.in)
		if c.ok && (err != nil || gw != c.gw || dev != c.dev) {
			t.Errorf("parse(%q) = %q,%q,%v want %q,%q,nil", c.in, gw, dev, err, c.gw, c.dev)
		}
		if !c.ok && err == nil {
			t.Errorf("parse(%q) expected error", c.in)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/netcfg/ -run ParseWinDefault -v`
Expected: compile error — `parseWinDefaultRoute` undefined.

- [ ] **Step 3: Implement the parser + Windows exec**

Create `internal/netcfg/gateway_windows.go`:

```go
//go:build windows

package netcfg

import (
	"fmt"
	"os/exec"
	"strings"
)

// psDefaultRoute prints "<NextHop> <InterfaceAlias>" for the lowest-metric
// IPv4 default route. Wrapped in a single line so the output is trivial to parse.
const psDefaultRoute = `$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction Stop | Sort-Object RouteMetric | Select-Object -First 1; "$($r.NextHop) $($r.InterfaceAlias)"`

// defaultGateway returns the physical default route's gateway IP and interface
// alias via PowerShell's Get-NetRoute, before the split-default routes shadow it.
func defaultGateway() (gw, dev string, err error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psDefaultRoute).Output()
	if err != nil {
		return "", "", fmt.Errorf("get-netroute default: %w", err)
	}
	return parseWinDefaultRoute(string(out))
}

// parseWinDefaultRoute splits "<gw> <interface alias>"; the interface alias may
// contain spaces (e.g. "Wi-Fi" is one token, but "Local Area Connection" is not),
// so everything after the first token is treated as the alias.
func parseWinDefaultRoute(s string) (gw, dev string, err error) {
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if line == "" {
		return "", "", fmt.Errorf("empty default route output")
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("malformed default route output %q", line)
	}
	gw = strings.TrimSpace(parts[0])
	dev = strings.TrimSpace(parts[1])
	if gw == "" || dev == "" {
		return "", "", fmt.Errorf("malformed default route output %q", line)
	}
	return gw, dev, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/netcfg/ -run ParseWinDefault -v`
Expected: PASS. (Runs on any OS — the parser has no build tag conflict because the test file has none and the exec wrapper is Windows-tagged. On Linux, `gateway_windows.go` is excluded but `parseWinDefaultRoute` must still be visible to the test: move the parser into a build-tag-free file — see note below.)

> **Important build-tag detail:** Go test files without build tags compile on every OS, but `parseWinDefaultRoute` lives in a `//go:build windows` file and would be invisible on Linux, breaking the test there. To keep the parser testable on any OS, put **both** `parseLinuxDefaultRoute` and `parseWinDefaultRoute` in a single build-tag-free file `internal/netcfg/gateway_parse.go`, and keep only the `defaultGateway()` exec wrappers in the OS-tagged files. Refactor Task 5 and Task 6 accordingly: move the two parser funcs into `gateway_parse.go`; `gateway_linux.go`/`gateway_windows.go` keep just `defaultGateway()` + the `const psDefaultRoute`. Re-run `go test ./internal/netcfg/ -run 'ParseLinuxDefault|ParseWinDefault'` and expect PASS on the host OS.

- [ ] **Step 5: Commit**

```bash
git add internal/netcfg/gateway_windows.go internal/netcfg/gateway_parse.go internal/netcfg/gateway_windows_test.go internal/netcfg/gateway_linux.go
git commit -m "feat(netcfg): windows default-gateway discovery + shared parsers"
```

---

## Task 7: Branch the Routers on FullTunnel

**Files:**
- Modify: `internal/netcfg/netcfg_linux.go`
- Modify: `internal/netcfg/netcfg_windows.go`

These call exec, so they are validated at runtime, not unit-tested here (the command builders and parsers already have unit tests). Keep the changes minimal.

- [ ] **Step 1: Update the Linux router**

In `internal/netcfg/netcfg_linux.go`, replace the `Up`/`Down` methods:

```go
func (linuxRouter) Up(c Config) error {
	if c.FullTunnel {
		gw, dev, err := defaultGateway()
		if err != nil {
			return err
		}
		return runAll(linuxFullUpCommands(c, gw, dev))
	}
	return runAll(upCommands(c))
}

func (linuxRouter) Down(c Config) error {
	if c.FullTunnel {
		return runAll(linuxFullDownCommands(c))
	}
	return runAll(downCommands(c))
}
```

- [ ] **Step 2: Update the Windows router**

In `internal/netcfg/netcfg_windows.go`:

```go
func (windowsRouter) Up(c Config) error {
	if c.FullTunnel {
		gw, dev, err := defaultGateway()
		if err != nil {
			return err
		}
		return runAll(winFullUpCommands(c, gw, dev))
	}
	return runAll(winUpCommands(c))
}

func (windowsRouter) Down(c Config) error {
	if c.FullTunnel {
		_, dev, err := defaultGateway()
		if err != nil {
			// Best effort: still try to remove the split-default + ipv6 routes,
			// which do not need the physical interface. Server-bypass deletes are
			// skipped if the interface is unknown (they are non-persistent anyway).
			dev = ""
		}
		return runAll(winFullDownCommands(c, dev))
	}
	return runAll(winDownCommands(c))
}
```

- [ ] **Step 3: Build to verify both OSes compile**

Run:
```bash
go build ./...
GOOS=windows go build ./internal/netcfg/
```
Expected: both succeed.

- [ ] **Step 4: Run the netcfg tests**

Run: `go test ./internal/netcfg/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/netcfg/netcfg_linux.go internal/netcfg/netcfg_windows.go
git commit -m "feat(netcfg): route full-tunnel through split-default Up/Down"
```

---

## Task 8: Thread full-tunnel through tunnel.Config and skip kill switch in full mode

**Files:**
- Modify: `internal/tunnel/tunnel.go`
- Modify: `gui_connector.go`
- Test: `internal/tunnel/tunnel_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tunnel/tunnel_test.go` (inspect the existing fakes in that file; they implement `Core`/`Tun`/`Router`/`Firewall`. Reuse them):

```go
func TestStartTUNFullSkipsKillSwitch(t *testing.T) {
	// In full mode the whitelist-shaped kill switch must NOT be installed
	// (Phase 2 will add a full-mode kill switch). Reuse this file's fakes.
	fw := &fakeFirewall{}
	rt := &fakeRouter{}
	tn := New(Config{
		Mode: store.ModeTUN, Device: "tun0",
		Full: true, KillSwitch: true,
		ServerIPs: []string{"203.0.113.7"}, BlockIPv6: true,
	}, Deps{
		Core: &fakeCore{}, Tun: &fakeTun{}, Router: rt, Firewall: fw,
	})
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fw.onCalled {
		t.Error("kill switch must be skipped in full mode (Phase 1)")
	}
	if !rt.up.FullTunnel {
		t.Error("router should receive FullTunnel=true config")
	}
}
```

Adapt the fake field/method names to whatever the existing fakes use (e.g. the fake firewall may track calls differently). If the file lacks suitable fakes, add minimal ones mirroring the `Core`/`Tun`/`Router`/`Firewall` interfaces, where `fakeRouter` records the last `Up(Config)` in a field `up`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tunnel/ -run TUNFull -v`
Expected: compile error — `Config` has no field `Full`.

- [ ] **Step 3: Extend tunnel.Config and the mapping**

In `internal/tunnel/tunnel.go`, add fields to `Config`:

```go
type Config struct {
	XrayJSON   []byte
	SocksHost  string
	SocksPort  int
	Mode       store.Mode
	Device     string
	TunIP      string
	TunPrefix  int
	RouteCIDRs []string
	KillSwitch bool
	Full       bool
	ServerIPs  []string
	BlockIPv6  bool
}
```

Update `netcfgConfig`:

```go
func (t *Tunnel) netcfgConfig() netcfg.Config {
	return netcfg.Config{
		Device:     t.cfg.Device,
		TunIP:      t.cfg.TunIP,
		Prefix:     t.cfg.TunPrefix,
		RouteCIDRs: t.cfg.RouteCIDRs,
		FullTunnel: t.cfg.Full,
		ServerIPs:  t.cfg.ServerIPs,
		BlockIPv6:  t.cfg.BlockIPv6,
	}
}
```

In `Start()`, guard the kill-switch block so it is skipped in full mode. Replace the `if t.cfg.KillSwitch {` line inside the `store.ModeTUN` case with:

```go
		if t.cfg.KillSwitch && !t.cfg.Full {
			if err := t.deps.Firewall.On(firewall.Config{Device: t.cfg.Device, CIDRs: t.cfg.RouteCIDRs}); err != nil {
				_ = t.deps.Router.Down(t.netcfgConfig())
				_ = t.deps.Tun.Stop()
				_ = t.inst.Stop()
				t.inst = nil
				return fmt.Errorf("enable kill switch: %w", err)
			}
		}
```

And in `Stop()`, guard the `Firewall.Off()` call the same way:

```go
		if t.cfg.KillSwitch && !t.cfg.Full {
			record(t.deps.Firewall.Off())
		}
```

- [ ] **Step 4: Forward the fields in gui_connector.go**

In `gui_connector.go`, inside the `case store.ModeTUN:` block, add after the existing assignments:

```go
		cfg.RouteCIDRs = c.RouteCIDRs
		cfg.KillSwitch = c.KillSwitch
		cfg.Full = c.FullTunnel
		cfg.ServerIPs = c.ServerIPs
		cfg.BlockIPv6 = c.BlockIPv6
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tunnel/ -v`
Expected: PASS.

- [ ] **Step 6: Build everything**

Run: `go build -tags wails ./...` (the wails build tag pulls in `gui_connector.go`).
Expected: success.

- [ ] **Step 7: Commit**

```bash
git add internal/tunnel/tunnel.go internal/tunnel/tunnel_test.go gui_connector.go
git commit -m "feat(tunnel): thread full-tunnel config + skip whitelist kill switch in full mode"
```

---

## Task 9: Frontend — routing-mode selector

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.ts`
- Reference: `frontend/wailsjs/go/models.ts` (the `ProfileDTO` type — confirm it now carries `full` after Go regeneration; see Step 4)

- [ ] **Step 1: Add the selector to the settings markup**

In `frontend/index.html`, near the routing controls (the Telegram toggle and custom-proxy inputs), add a routing-mode selector. Place it directly above the Telegram toggle:

```html
<label>Режим маршрутизации
  <select id="routing-mode-select">
    <option value="whitelist">Whitelist (только выбранное)</option>
    <option value="full">Всё через VPN</option>
  </select>
</label>
```

Wrap the whitelist-only controls (the Telegram toggle and the custom proxy domains/IPs inputs) so they can be hidden as a group. Give their container `id="whitelist-only"`, e.g.:

```html
<div id="whitelist-only">
  <!-- existing Telegram toggle + custom proxy domains/IPs inputs -->
</div>
```

(Find the exact existing elements in `index.html` and move them inside this wrapper without changing their ids.)

- [ ] **Step 2: Read the routing mode in render()**

In `frontend/src/main.ts`, the `Profile` TypeScript type (search for where `telegram`, `forceRUDirect` etc. are declared) gains a `full: boolean` field. In the render function that paints settings (where `forceRUDirect` etc. are applied), add:

```ts
  const routingSel = <HTMLSelectElement>$("routing-mode-select");
  routingSel.value = st.profile.full ? "full" : "whitelist";
  $("whitelist-only").classList.toggle("hidden", st.profile.full);
```

(`hidden` is the existing utility class already used for `autostart-row` — reuse it.)

- [ ] **Step 3: Handle changes and persist via UpdateProfile**

In the event-wiring section of `main.ts` (where the other profile toggles call `UpdateProfile`), add:

```ts
  $("routing-mode-select").addEventListener("change", () => {
    const full = (<HTMLSelectElement>$("routing-mode-select")).value === "full";
    const prev = current.profile.full;
    current.profile.full = full;
    $("whitelist-only").classList.toggle("hidden", full);
    UpdateProfile(current.profile).catch((e) => {
      current.profile.full = prev;
      (<HTMLSelectElement>$("routing-mode-select")).value = prev ? "full" : "whitelist";
      $("whitelist-only").classList.toggle("hidden", prev);
      $("error-line").textContent = String(e);
    });
  });
```

Match the exact `UpdateProfile` import/usage style already in the file (the profile object shape must match what `UpdateProfile` expects — the regenerated DTO).

- [ ] **Step 4: Regenerate Wails bindings and build the frontend**

The Go `ProfileDTO` must expose `full`. Confirm `internal/app/types.go`'s `ProfileDTO` has a `Full bool json:"full"` field and that `profileDTO()`/`UpdateProfile` map it (see Task 10 — do Task 10 before this build step). Then:

```bash
wails generate module    # regenerates frontend/wailsjs/go/models.ts
cd frontend && npm run build && cd ..
```

Expected: `frontend/wailsjs/go/models.ts` `ProfileDTO` includes `full: boolean`; the frontend compiles (tsc + vite) with no type errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html frontend/src/main.ts frontend/wailsjs/go/models.ts
git commit -m "feat(frontend): routing-mode selector (whitelist / full tunnel)"
```

---

## Task 10: Backend DTO plumbing for Profile.Full

**Files:**
- Modify: `internal/app/types.go` (`ProfileDTO`, `profileDTO()`)
- Modify: `internal/app/settings.go` (`UpdateProfile`)
- Test: `internal/app/settings_test.go`

> Do this task **before** Task 9 Step 4 (bindings regeneration depends on it). It is ordered here because it is small; if executing strictly top-to-bottom, swap Task 9 Step 4 to run after this task.

- [ ] **Step 1: Write the failing test**

Add to `internal/app/settings_test.go`:

```go
func TestUpdateProfileFullRoundTrips(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.UpdateProfile(ProfileDTO{Full: true, ForceRUDirect: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	st := svc.GetState()
	if !st.Profile.Full {
		t.Error("StateDTO.Profile.Full should round-trip true")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/ -run TestUpdateProfileFull -v`
Expected: compile error — `ProfileDTO` has no field `Full`.

- [ ] **Step 3: Add Full to ProfileDTO and the mappings**

In `internal/app/types.go`, add the field to `ProfileDTO`:

```go
type ProfileDTO struct {
	Full               bool     `json:"full"`
	Telegram           bool     `json:"telegram"`
	ForceRUDirect      bool     `json:"forceRUDirect"`
	CustomProxyDomains []string `json:"customProxyDomains"`
	CustomProxyIPs     []string `json:"customProxyIPs"`
}
```

In `profileDTO()` (same file), map it back out:

```go
func profileDTO(p routing.Profile) ProfileDTO {
	return ProfileDTO{
		Full:               p.Full,
		Telegram:           p.Telegram,
		ForceRUDirect:      p.ForceRUDirect,
		CustomProxyDomains: p.CustomProxyDomains,
		CustomProxyIPs:     p.CustomProxyIPs,
	}
}
```

In `internal/app/settings.go`, `UpdateProfile` builds `routing.Profile` from the DTO — add `Full`:

```go
	s.state.Profile = routing.Profile{
		Full:               p.Full,
		Telegram:           p.Telegram,
		ForceRUDirect:      p.ForceRUDirect,
		CustomProxyDomains: p.CustomProxyDomains,
		CustomProxyIPs:     p.CustomProxyIPs,
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/app/ -run TestUpdateProfileFull -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/types.go internal/app/settings.go internal/app/settings_test.go
git commit -m "feat(app): plumb Profile.Full through ProfileDTO/UpdateProfile"
```

---

## Task 11: xray config snapshot for full mode + full suite

**Files:**
- Modify: `internal/xrayconf/build_test.go`

- [ ] **Step 1: Write the test**

Add to `internal/xrayconf/build_test.go` (mirror the style of the existing build tests in that file — inspect how a server config is constructed there and reuse the helper):

```go
func TestBuildFullTunnelRouting(t *testing.T) {
	// Reuse the existing test server constructor from this file (e.g. testServer()).
	srv := testServer()
	p := routing.Profile{Full: true, ForceRUDirect: true}
	out, err := Build(srv, p, Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	s := string(out)
	// The catch-all must target the proxy outbound, and IPv6 must be blocked.
	if !strings.Contains(s, `"outboundTag":"proxy"`) {
		t.Error("full-mode config should route the catch-all to proxy")
	}
	if !strings.Contains(s, `"::/0"`) {
		t.Error("full-mode config should contain the IPv6 block rule")
	}
}
```

If `build_test.go` lacks a reusable `testServer()` helper, construct a minimal `*vless.ServerConfig` inline matching what the other tests in the file use. Ensure `strings` and `routing` are imported.

- [ ] **Step 2: Run the test**

Run: `go test ./internal/xrayconf/ -run FullTunnel -v`
Expected: PASS (no production code change needed — `Build` already consumes `Profile.Rules()`).

- [ ] **Step 3: Run the entire Go suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/xrayconf/build_test.go
git commit -m "test(xrayconf): full-tunnel routing config snapshot"
```

---

## Task 12: Docs + final verification

**Files:**
- Modify: `docs/USER-GUIDE.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Document the routing mode in the user guide**

In `docs/USER-GUIDE.md`, under "Настройки маршрутизации", add a short subsection:

```markdown
### Режим маршрутизации

- **Whitelist** — по умолчанию весь трафик идёт напрямую, через VPN — только выбранное
  (Telegram, кастомные домены/IP).
- **Всё через VPN** — весь трафик идёт через VPN. Локальная сеть (LAN) всегда напрямую;
  IPv6 на время подключения блокируется (защита от утечки). Галочка **RU Direct** остаётся
  доступной — можно пускать «всё, кроме РФ» через VPN.

В режиме **Всё через VPN** опции Telegram и кастомного прокси скрыты — они не нужны.
```

- [ ] **Step 2: Add a changelog entry**

In `CHANGELOG.md`, add a new top section:

```markdown
## v1.0.14 — 2026-06-11

### Возможности
- Новый режим маршрутизации «Всё через VPN» (full tunnel) для Proxy и TUN. LAN остаётся
  напрямую, IPv6 блокируется на время подключения, RU-Direct можно комбинировать.
```

(Coordinate the version with the pending autoconnect fix if it ships in the same release.)

- [ ] **Step 3: Full build + test gate**

Run:
```bash
go build ./...
go build -tags wails ./...
GOOS=windows go build ./internal/netcfg/
go test ./...
```
Expected: all succeed / PASS.

- [ ] **Step 4: Commit**

```bash
git add docs/USER-GUIDE.md CHANGELOG.md
git commit -m "docs: document full-tunnel routing mode"
```

---

## Self-Review Notes (coverage vs spec)

- Spec §1 (xray rules full mode) → Task 1 + Task 11.
- Spec §2 (netcfg config + Up/Down full mode) → Tasks 4, 7.
- Spec §3 (server resolution + gateway discovery) → Tasks 2, 5, 6.
- Spec §4 (wiring in connect.go) → Task 3.
- Spec §5 (frontend selector + hide whitelist controls) → Tasks 9, 10.
- Spec §6 IPv6 block → Task 1 (xray ::/0 block) + Task 4 (v6 capture routes).
- Spec §6 kill-switch interplay → Task 8 (skipped-with-guard in Phase 1); full implementation is **Phase 2** (separate plan).
- Spec §6 DNS edge → documented (Task 12), no code.
- Proxy-mode full tunnel → covered by Task 1 alone (rules-only); no netcfg path.

## Phase 2 (separate plan, not in this document)

Rewrite `internal/firewall` for a full-mode kill switch: allow `oifname tun0` and the server-bypass IPs out the physical interface, drop all other v4/v6 egress. Then re-enable the kill switch in full mode (remove the `&& !t.cfg.Full` guards from Task 8) and add a server-IP allow list to `firewall.Config`.
