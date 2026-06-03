# Kill Switch (selective, Linux) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Linux kill switch that drops whitelisted destination CIDRs when they would egress any interface other than the TUN device, preventing plaintext leaks if the tunnel fails.

**Architecture:** A new `internal/firewall` package builds and applies an nftables table (`inet vless_killswitch`) with an output-hook rule `oifname != "<device>" ... daddr { <cidrs> } drop`. The `internal/tunnel` orchestrator enables it after routes come up (TUN mode, when the `KillSwitch` setting is on) and removes it on teardown. `internal/app` passes the setting through; the GUI wires the real firewall and exposes a checkbox.

**Tech Stack:** Go, nftables (`nft` CLI), Wails + vanilla TypeScript frontend.

---

## File Structure

- Create: `internal/firewall/firewall.go` — `Config`, `Firewall` interface, pure helpers (`splitCIDRs`, `buildRuleset`), `tableName` const.
- Create: `internal/firewall/firewall_linux.go` — nftables implementation + `New()`.
- Create: `internal/firewall/firewall_other.go` — `!linux` stub + `New()`.
- Create: `internal/firewall/firewall_test.go` — tests for the pure helpers.
- Modify: `internal/tunnel/tunnel.go` — `Deps.Firewall`, `Config.KillSwitch`, Start/Stop wiring + rollback.
- Modify: `internal/tunnel/tunnel_test.go` — `fakeFirewall`, updated `newDeps`, new ordering/rollback tests.
- Modify: `internal/app/types.go` — `ConnConfig.KillSwitch`.
- Modify: `internal/app/connect.go` — set `cc.KillSwitch` in TUN mode + log note.
- Modify: `internal/app/connect_test.go` — kill-switch pass-through test.
- Modify: `gui_connector.go` — supply `firewall.New()`, pass `KillSwitch`.
- Modify: `frontend/index.html` — kill-switch checkbox.
- Modify: `frontend/src/main.ts` — bind the checkbox to settings.

---

## Task 1: firewall package — pure helpers (split + ruleset)

**Files:**
- Create: `internal/firewall/firewall.go`
- Test: `internal/firewall/firewall_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/firewall/firewall_test.go`:

```go
package firewall

import (
	"strings"
	"testing"
)

func TestSplitCIDRs(t *testing.T) {
	v4, v6 := splitCIDRs([]string{"149.154.160.0/20", "2001:db8::/32", "203.0.113.0/24"})
	if len(v4) != 2 || v4[0] != "149.154.160.0/20" || v4[1] != "203.0.113.0/24" {
		t.Errorf("v4 = %v", v4)
	}
	if len(v6) != 1 || v6[0] != "2001:db8::/32" {
		t.Errorf("v6 = %v", v6)
	}
}

func TestBuildRulesetV4Only(t *testing.T) {
	rs := buildRuleset("tun0", []string{"149.154.160.0/20"}, nil)
	if !strings.Contains(rs, "table inet vless_killswitch") {
		t.Errorf("missing table name:\n%s", rs)
	}
	if !strings.Contains(rs, `oifname != "tun0" ip daddr { 149.154.160.0/20 } drop`) {
		t.Errorf("missing v4 drop rule:\n%s", rs)
	}
	if strings.Contains(rs, "ip6 daddr") {
		t.Errorf("should not emit ip6 rule when no v6 CIDRs:\n%s", rs)
	}
}

func TestBuildRulesetBothFamilies(t *testing.T) {
	rs := buildRuleset("tun0", []string{"203.0.113.0/24"}, []string{"2001:db8::/32"})
	if !strings.Contains(rs, `oifname != "tun0" ip daddr { 203.0.113.0/24 } drop`) {
		t.Errorf("missing v4 rule:\n%s", rs)
	}
	if !strings.Contains(rs, `oifname != "tun0" ip6 daddr { 2001:db8::/32 } drop`) {
		t.Errorf("missing v6 rule:\n%s", rs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/firewall/`
Expected: FAIL — `undefined: splitCIDRs` / `undefined: buildRuleset` (build error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/firewall/firewall.go`:

```go
// Package firewall installs a selective kill switch: it drops whitelisted
// destination CIDRs when they would leave the host through any interface other
// than the TUN device, so a tunnel failure cannot leak that traffic in
// plaintext. Everything not in the whitelist is untouched (it is meant to go
// direct under the project's selective host-route model).
package firewall

import (
	"fmt"
	"strings"
)

// tableName is the nftables table the kill switch owns end to end.
const tableName = "vless_killswitch"

// Config describes one kill-switch session.
type Config struct {
	Device string   // TUN device whose egress is allowed, e.g. "tun0"
	CIDRs  []string // whitelisted destination CIDRs to protect
}

// Firewall installs and removes the kill switch.
type Firewall interface {
	On(c Config) error
	Off() error
}

// splitCIDRs partitions CIDRs into IPv4 and IPv6 by the presence of a colon.
func splitCIDRs(cidrs []string) (v4, v6 []string) {
	for _, c := range cidrs {
		if strings.Contains(c, ":") {
			v6 = append(v6, c)
		} else {
			v4 = append(v4, c)
		}
	}
	return v4, v6
}

// buildRuleset renders the nftables ruleset. A family's drop line is emitted
// only when that family has at least one CIDR.
func buildRuleset(device string, v4, v6 []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s {\n", tableName)
	b.WriteString("\tchain out {\n")
	b.WriteString("\t\ttype filter hook output priority 0; policy accept;\n")
	if len(v4) > 0 {
		fmt.Fprintf(&b, "\t\toifname != %q ip daddr { %s } drop\n", device, strings.Join(v4, ", "))
	}
	if len(v6) > 0 {
		fmt.Fprintf(&b, "\t\toifname != %q ip6 daddr { %s } drop\n", device, strings.Join(v6, ", "))
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/firewall/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/firewall/firewall.go internal/firewall/firewall_test.go
git commit -m "feat(firewall): kill-switch ruleset builder and CIDR split"
```

---

## Task 2: firewall — Linux nftables implementation

**Files:**
- Create: `internal/firewall/firewall_linux.go`

No new unit test: `On`/`Off` shell out to `nft` and need root, so they are exercised by the manual e2e step, not `go test`. The pure logic they call is already covered in Task 1.

- [ ] **Step 1: Write the implementation**

Create `internal/firewall/firewall_linux.go`:

```go
package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

type linuxFirewall struct{}

// New returns the Linux (nftables) kill switch.
func New() Firewall { return linuxFirewall{} }

// checkNft verifies the nft binary is available before we try to use it, so a
// missing dependency is a clear error rather than a confusing apply failure.
func checkNft() error {
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("firewall: nft not found in PATH: %w", err)
	}
	return nil
}

// On installs the kill-switch table. It is idempotent: any stale table from a
// previous run is removed first. With no CIDRs to protect it is a no-op.
func (linuxFirewall) On(c Config) error {
	if c.Device == "" {
		return fmt.Errorf("firewall: empty device name")
	}
	if err := checkNft(); err != nil {
		return err
	}
	v4, v6 := splitCIDRs(c.CIDRs)
	if len(v4) == 0 && len(v6) == 0 {
		return nil
	}
	// Drop any leftover table so re-applying is clean; ignore "not found".
	_ = exec.Command("nft", "delete", "table", "inet", tableName).Run()

	ruleset := buildRuleset(c.Device, v4, v6)
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(ruleset)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("firewall: apply ruleset: %w: %s", err, out)
	}
	return nil
}

// Off removes the kill-switch table. Removing an absent table is success.
func (linuxFirewall) Off() error {
	out, err := exec.Command("nft", "delete", "table", "inet", tableName).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "No such file") {
		return fmt.Errorf("firewall: delete table: %w: %s", err, out)
	}
	return nil
}
```

- [ ] **Step 2: Verify it builds and the package tests still pass**

Run: `go test ./internal/firewall/`
Expected: PASS (ok, no compile errors).

- [ ] **Step 3: Commit**

```bash
git add internal/firewall/firewall_linux.go
git commit -m "feat(firewall): nftables On/Off (Linux)"
```

---

## Task 3: firewall — non-Linux stub

**Files:**
- Create: `internal/firewall/firewall_other.go`

- [ ] **Step 1: Write the implementation**

Create `internal/firewall/firewall_other.go`:

```go
//go:build !linux

package firewall

import (
	"fmt"
	"runtime"
)

type stubFirewall struct{}

// New returns a stub kill switch that errors on use. Real darwin/windows
// firewall support is a later iteration.
func New() Firewall { return stubFirewall{} }

func (stubFirewall) On(Config) error {
	return fmt.Errorf("firewall: kill switch not supported on %s yet", runtime.GOOS)
}

func (stubFirewall) Off() error { return nil }
```

- [ ] **Step 2: Verify cross-compile**

Run: `GOOS=windows go build ./internal/firewall/ && GOOS=darwin go build ./internal/firewall/`
Expected: both succeed (no output).

- [ ] **Step 3: Commit**

```bash
git add internal/firewall/firewall_other.go
git commit -m "feat(firewall): non-linux stub"
```

---

## Task 4: tunnel — wire the kill switch into Start/Stop

**Files:**
- Modify: `internal/tunnel/tunnel.go`
- Modify: `internal/tunnel/tunnel_test.go`

- [ ] **Step 1: Write the failing tests**

In `internal/tunnel/tunnel_test.go`, add the firewall import, a fake, and update `newDeps`. Replace the import block, add `fakeFirewall`, and replace `newDeps`:

Change the import block at the top to:

```go
import (
	"errors"
	"testing"

	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/store"
)
```

Add after `fakeRouter` (around line 43):

```go
type fakeFirewall struct {
	on, off bool
	device  string
	cidrs   []string
	err     error
}

func (f *fakeFirewall) On(c firewall.Config) error {
	if f.err != nil {
		return f.err
	}
	f.on = true
	f.device = c.Device
	f.cidrs = c.CIDRs
	return nil
}
func (f *fakeFirewall) Off() error { f.off = true; return nil }
```

Replace `newDeps` with:

```go
func newDeps() (*fakeCore, *fakeProxy, *fakeTun, *fakeRouter, *fakeFirewall, Deps) {
	c, p, tn, r, fw := &fakeCore{}, &fakeProxy{}, &fakeTun{}, &fakeRouter{}, &fakeFirewall{}
	return c, p, tn, r, fw, Deps{Core: c, Proxy: p, Tun: tn, Router: r, Firewall: fw}
}
```

Update the three existing `newDeps()` call sites to take the extra return value:
- `TestProxyModeStartStop`: `c, p, _, _, _, deps := newDeps()`
- `TestTunModeStartStop`: `c, _, tnsvc, r, _, deps := newDeps()`
- `TestStartRollsBackOnCoreFailure`: `c, p, _, _, _, deps := newDeps()`

Add two new tests at the end of the file:

```go
func TestTunModeKillSwitchOnOff(t *testing.T) {
	c, _, tnsvc, r, fw, deps := newDeps()
	cfg := baseConfig(store.ModeTUN)
	cfg.KillSwitch = true
	tn := New(cfg, deps)
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !fw.on {
		t.Error("kill switch should be enabled in TUN mode when KillSwitch is set")
	}
	if fw.device != "tun0" || len(fw.cidrs) == 0 {
		t.Errorf("firewall got device=%q cidrs=%v", fw.device, fw.cidrs)
	}
	_ = c
	_ = tnsvc
	_ = r
	if err := tn.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !fw.off {
		t.Error("kill switch should be disabled on Stop")
	}
}

func TestTunModeKillSwitchDisabled(t *testing.T) {
	_, _, _, _, fw, deps := newDeps()
	tn := New(baseConfig(store.ModeTUN), deps) // KillSwitch false by default
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fw.on {
		t.Error("kill switch must stay off when KillSwitch is false")
	}
	_ = tn.Stop()
}

func TestStartRollsBackOnFirewallFailure(t *testing.T) {
	c, _, tnsvc, r, fw, deps := newDeps()
	fw.err = errors.New("nft boom")
	cfg := baseConfig(store.ModeTUN)
	cfg.KillSwitch = true
	tn := New(cfg, deps)
	if err := tn.Start(); err == nil {
		t.Fatal("expected Start to fail when firewall fails")
	}
	if !r.down || !tnsvc.stopped || !c.inst.stopped {
		t.Errorf("firewall failure must roll back routes/tun/core: down=%v tunStopped=%v coreStopped=%v",
			r.down, tnsvc.stopped, c.inst.stopped)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tunnel/`
Expected: FAIL — `unknown field Firewall in struct literal` / `cfg.KillSwitch undefined` (build error).

- [ ] **Step 3: Implement in `internal/tunnel/tunnel.go`**

Add the import:

```go
	"github.com/zki/vless-client/internal/firewall"
```

Add `Firewall` to `Deps`:

```go
// Deps are the injected platform dependencies.
type Deps struct {
	Core     Core
	Proxy    Proxy
	Tun      Tun
	Router   netcfg.Router
	Firewall firewall.Firewall
}
```

Add `KillSwitch` to `Config` (after `RouteCIDRs`):

```go
	RouteCIDRs []string
	KillSwitch bool
}
```

In `Start`, replace the `case store.ModeTUN:` block with:

```go
	case store.ModeTUN:
		if err := t.deps.Tun.Start(t.cfg.Device, t.socksURL()); err != nil {
			_ = t.inst.Stop()
			t.inst = nil
			return fmt.Errorf("start tun: %w", err)
		}
		if err := t.deps.Router.Up(t.netcfgConfig()); err != nil {
			_ = t.deps.Tun.Stop()
			_ = t.inst.Stop()
			t.inst = nil
			return fmt.Errorf("apply routes: %w", err)
		}
		if t.cfg.KillSwitch {
			if err := t.deps.Firewall.On(firewall.Config{Device: t.cfg.Device, CIDRs: t.cfg.RouteCIDRs}); err != nil {
				_ = t.deps.Router.Down(t.netcfgConfig())
				_ = t.deps.Tun.Stop()
				_ = t.inst.Stop()
				t.inst = nil
				return fmt.Errorf("enable kill switch: %w", err)
			}
		}
```

In `Stop`, replace the `case store.ModeTUN:` block with:

```go
	case store.ModeTUN:
		if t.cfg.KillSwitch {
			record(t.deps.Firewall.Off())
		}
		record(t.deps.Router.Down(t.netcfgConfig()))
		record(t.deps.Tun.Stop())
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tunnel/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/tunnel.go internal/tunnel/tunnel_test.go
git commit -m "feat(tunnel): enable kill switch after routes in TUN mode"
```

---

## Task 5: app — pass the KillSwitch setting through

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/app/connect.go`
- Modify: `internal/app/connect_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/app/connect_test.go`, add after `TestConnectTUNHappyPath`:

```go
func TestConnectTUNPassesKillSwitch(t *testing.T) {
	svc, _, _, captured := testDeps(t) // elevated, default Mode=tun
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", KillSwitch: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.UpdateProfile(ProfileDTO{Telegram: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !captured.KillSwitch {
		t.Error("ConnConfig.KillSwitch should be true when setting is on in TUN mode")
	}
}

func TestConnectProxyIgnoresKillSwitch(t *testing.T) {
	svc, _, _, captured := testDeps(t)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", KillSwitch: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if captured.KillSwitch {
		t.Error("ConnConfig.KillSwitch must be false in proxy mode")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run KillSwitch`
Expected: FAIL — `captured.KillSwitch undefined` (build error).

- [ ] **Step 3: Implement**

In `internal/app/types.go`, add `KillSwitch` to `ConnConfig` (after `RouteCIDRs`):

```go
	RouteCIDRs []string
	KillSwitch bool
}
```

In `internal/app/connect.go`, inside `Connect`, extend the `if mode == store.ModeTUN {` block to set the field and log when on:

```go
	if mode == store.ModeTUN {
		cc.Device = tunDevice
		cc.TunIP = tunIP
		cc.TunPrefix = tunPrefix
		cc.RouteCIDRs = tunRouteCIDRs(s.state.Profile)
		cc.KillSwitch = s.state.Settings.KillSwitch
		s.bus.Append("note: TUN mode routes whitelisted IPs only; geosite domains are not host-routed")
		if cc.KillSwitch {
			s.bus.Append("note: kill switch active — whitelisted IPs are blocked if they leave the TUN")
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/`
Expected: PASS (all app tests).

- [ ] **Step 5: Commit**

```bash
git add internal/app/types.go internal/app/connect.go internal/app/connect_test.go
git commit -m "feat(app): pass KillSwitch setting into TUN connection config"
```

---

## Task 6: GUI connector — supply the real firewall

**Files:**
- Modify: `gui_connector.go`

This file is behind the `wails` build tag; verify with the wails build, not `go test`.

- [ ] **Step 1: Implement**

In `gui_connector.go`, add the import:

```go
	"github.com/zki/vless-client/internal/firewall"
```

Add `Firewall` to the base `deps` (it is harmless when unused, and used in TUN mode):

```go
	deps := tunnel.Deps{Core: coreAdapter{}, Firewall: firewall.New()}
```

In the `case store.ModeTUN:` block, pass the setting through (after `cfg.RouteCIDRs = c.RouteCIDRs`):

```go
		cfg.KillSwitch = c.KillSwitch
```

- [ ] **Step 2: Verify the tagged build compiles**

Run: `wails build -tags "wails webkit2_41"`
Expected: build succeeds (binary written under `build/bin`).

- [ ] **Step 3: Commit**

```bash
git add gui_connector.go
git commit -m "feat(gui): wire nftables kill switch into TUN connector"
```

---

## Task 7: Frontend — kill-switch checkbox

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.ts`

- [ ] **Step 1: Add the checkbox to `frontend/index.html`**

In the `#routing` section, after the mode `<div class="row">...</div>` (closing at line 43) and before `</section>`, add:

```html
        <label><input type="checkbox" id="kill-toggle" /> Kill switch (TUN)</label>
```

- [ ] **Step 2: Bind it in `frontend/src/main.ts`**

In the state-render function, after the line that sets the mode select
(`(<HTMLSelectElement>$("mode-select")).value = st.settings.mode;`), add:

```ts
  (<HTMLInputElement>$("kill-toggle")).checked = st.settings.killSwitch;
```

In the event-wiring section, after the existing `mode-select` change listener
(the block ending around line 113), add:

```ts
  $("kill-toggle").addEventListener("change", () => {
    current.settings.killSwitch = (<HTMLInputElement>$("kill-toggle")).checked;
    UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
  });
```

- [ ] **Step 3: Verify the tagged build still compiles (frontend bundles)**

Run: `wails build -tags "wails webkit2_41"`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html frontend/src/main.ts
git commit -m "feat(gui): kill switch toggle in routing settings"
```

---

## Task 8: Full verification

- [ ] **Step 1: Run the whole test suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 2: Cross-compile the stub platforms**

Run: `GOOS=windows go build ./... ; GOOS=darwin go build ./internal/firewall/`
Expected: succeed (the firewall stub compiles; note other GUI packages may be gated and is fine).

- [ ] **Step 3: Manual e2e on Linux under sudo (root-gated, document result)**

This cannot run in CI; perform manually and record the outcome:

1. Build: `wails build -tags "wails webkit2_41"`.
2. Run elevated, select TUN mode, enable the kill-switch checkbox, enable Telegram, connect.
3. Confirm a Telegram CIDR is reachable through the tunnel.
4. Confirm the table exists: `sudo nft list table inet vless_killswitch`.
5. Simulate failure: `sudo ip link set tun0 down` (or kill the engine).
6. Confirm traffic to a Telegram CIDR is now dropped (e.g. `curl --max-time 5` to an address in range times out / fails) while unrelated direct traffic still works.
7. Disconnect and confirm the table is gone: `sudo nft list table inet vless_killswitch` returns "No such file or directory".

- [ ] **Step 4: Update the project memory phase status**

Mark the kill-switch item DONE under Phase 4 in
`/home/zki/.claude/projects/-home-zki-projects-vless-client/memory/project-vless-client.md`
(leave darwin/windows netcfg as the remaining Phase 4 TODO).
