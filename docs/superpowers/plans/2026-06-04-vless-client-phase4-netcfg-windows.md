# Windows TUN Routing (netcfg) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement real `internal/netcfg` routing on Windows via `netsh`, so TUN mode assigns the TUN adapter's IP and routes the whitelisted CIDRs into it, matching the Linux behaviour; macOS stays an unsupported stub.

**Architecture:** Mirror the Linux design — pure command-builder functions return `[][]string` and a shared `runAll` executes them. Builders and the prefix→mask helper live in an untagged file so they unit-test on the Linux dev machine; the Windows `Router` glue is behind `//go:build windows`. The `!linux` stub becomes `!linux && !windows`.

**Tech Stack:** Go, Windows `netsh interface ipv4/ipv6` CLI, `github.com/xjasonlyu/tun2socks/v2` (Wintun adapter named after the device).

---

## File Structure

- Create: `internal/netcfg/netcfg_exec.go` (no build tag) — shared `runAll`, moved out of `netcfg_linux.go`.
- Create: `internal/netcfg/netcfg_windows_cmds.go` (no build tag) — `winUpCommands`, `winDownCommands`, `prefixToMaskV4`, `splitCIDRs`.
- Create: `internal/netcfg/netcfg_windows_cmds_test.go` (no build tag) — tests for the above.
- Create: `internal/netcfg/netcfg_windows.go` (`//go:build windows`) — `windowsRouter` + `New()`.
- Modify: `internal/netcfg/netcfg_linux.go` — remove `runAll` (now shared) and its now-unused imports.
- Modify: `internal/netcfg/netcfg_other.go` — build constraint `!linux` → `!linux && !windows`.

---

## Task 1: Share `runAll` across platforms

**Files:**
- Create: `internal/netcfg/netcfg_exec.go`
- Modify: `internal/netcfg/netcfg_linux.go`

This is a refactor (no behaviour change); the existing Linux tests guard it.

- [ ] **Step 1: Create `internal/netcfg/netcfg_exec.go`**

```go
package netcfg

import (
	"fmt"
	"os/exec"
)

// runAll executes a sequence of commands, stopping at the first failure. Shared
// by the Linux (iproute2) and Windows (netsh) routers.
func runAll(cmds [][]string) error {
	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", cmd, err, out)
		}
	}
	return nil
}
```

- [ ] **Step 2: Remove `runAll` from `internal/netcfg/netcfg_linux.go`**

Delete this block from `netcfg_linux.go`:

```go
func runAll(cmds [][]string) error {
	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", cmd, err, out)
		}
	}
	return nil
}
```

Then fix the import block in `netcfg_linux.go` — `fmt` and `os/exec` are no
longer used there (only `strconv` remains). Change:

```go
import (
	"fmt"
	"os/exec"
	"strconv"
)
```

to:

```go
import (
	"strconv"
)
```

- [ ] **Step 3: Verify build and existing Linux tests pass**

Run: `go build ./internal/netcfg/ && go test ./internal/netcfg/`
Expected: build OK; PASS (TestLinuxUpCommands, TestLinuxDownCommands).

- [ ] **Step 4: Commit**

```bash
git add internal/netcfg/netcfg_exec.go internal/netcfg/netcfg_linux.go
git commit -m "refactor(netcfg): share runAll across platform routers"
```

---

## Task 2: Windows command builders + helpers

**Files:**
- Create: `internal/netcfg/netcfg_windows_cmds.go`
- Test: `internal/netcfg/netcfg_windows_cmds_test.go`

- [ ] **Step 1: Write the failing tests — create `internal/netcfg/netcfg_windows_cmds_test.go`**

```go
package netcfg

import (
	"reflect"
	"testing"
)

func TestPrefixToMaskV4(t *testing.T) {
	cases := map[int]string{
		15: "255.254.0.0",
		24: "255.255.255.0",
		32: "255.255.255.255",
		0:  "0.0.0.0",
	}
	for p, want := range cases {
		if got := prefixToMaskV4(p); got != want {
			t.Errorf("prefixToMaskV4(%d) = %q, want %q", p, got, want)
		}
	}
}

func TestSplitCIDRs(t *testing.T) {
	v4, v6 := splitCIDRs([]string{"149.154.160.0/20", "2001:67c:4e8::/48", "91.108.4.0/22"})
	wantV4 := []string{"149.154.160.0/20", "91.108.4.0/22"}
	wantV6 := []string{"2001:67c:4e8::/48"}
	if !reflect.DeepEqual(v4, wantV4) {
		t.Errorf("v4 = %v, want %v", v4, wantV4)
	}
	if !reflect.DeepEqual(v6, wantV6) {
		t.Errorf("v6 = %v, want %v", v6, wantV6)
	}
}

func TestWinUpCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20", "2001:67c:4e8::/48"}}
	got := winUpCommands(c)
	want := [][]string{
		{"netsh", "interface", "ipv4", "set", "address", "name=tun0", "static", "198.18.0.1", "255.254.0.0"},
		{"netsh", "interface", "ipv4", "add", "route", "149.154.160.0/20", "interface=tun0", "store=active"},
		{"netsh", "interface", "ipv6", "add", "route", "2001:67c:4e8::/48", "interface=tun0", "store=active"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("winUpCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestWinDownCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20", "2001:67c:4e8::/48"}}
	got := winDownCommands(c)
	want := [][]string{
		{"netsh", "interface", "ipv4", "delete", "route", "149.154.160.0/20", "interface=tun0"},
		{"netsh", "interface", "ipv6", "delete", "route", "2001:67c:4e8::/48", "interface=tun0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("winDownCommands =\n%v\nwant\n%v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/netcfg/ -run 'Win|PrefixToMask|SplitCIDRs'`
Expected: FAIL — `undefined: prefixToMaskV4 / splitCIDRs / winUpCommands / winDownCommands` (build error).

- [ ] **Step 3: Create `internal/netcfg/netcfg_windows_cmds.go`**

```go
// Windows TUN routing command builders. These are deliberately free of build
// tags so they compile and unit-test on any OS; the Windows-only Router that
// executes them lives in netcfg_windows.go. tun2socks creates the Wintun
// adapter (named after Config.Device) but assigns no IP or routes, so the
// router does that here via netsh, mirroring the Linux iproute2 path.
package netcfg

import (
	"fmt"
	"net"
	"strings"
)

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

// prefixToMaskV4 renders an IPv4 prefix length as a dotted-decimal mask
// (e.g. 15 -> "255.254.0.0").
func prefixToMaskV4(prefix int) string {
	m := net.CIDRMask(prefix, 32)
	return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}

// winUpCommands assigns the TUN adapter's IPv4 address and routes each
// whitelisted CIDR into the adapter. Routes use store=active so they are
// non-persistent and cannot survive a crash/reboot.
func winUpCommands(c Config) [][]string {
	v4, v6 := splitCIDRs(c.RouteCIDRs)
	cmds := [][]string{
		{"netsh", "interface", "ipv4", "set", "address", "name=" + c.Device, "static", c.TunIP, prefixToMaskV4(c.Prefix)},
	}
	for _, r := range v4 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "add", "route", r, "interface=" + c.Device, "store=active"})
	}
	for _, r := range v6 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv6", "add", "route", r, "interface=" + c.Device, "store=active"})
	}
	return cmds
}

// winDownCommands removes the routes added by winUpCommands. The adapter's IP
// disappears when tun2socks destroys the Wintun adapter, so only routes are
// torn down here.
func winDownCommands(c Config) [][]string {
	v4, v6 := splitCIDRs(c.RouteCIDRs)
	var cmds [][]string
	for _, r := range v4 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "delete", "route", r, "interface=" + c.Device})
	}
	for _, r := range v6 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv6", "delete", "route", r, "interface=" + c.Device})
	}
	return cmds
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/netcfg/`
Expected: PASS (Linux + new Windows builder tests).

- [ ] **Step 5: Commit**

```bash
git add internal/netcfg/netcfg_windows_cmds.go internal/netcfg/netcfg_windows_cmds_test.go
git commit -m "feat(netcfg): Windows netsh command builders"
```

---

## Task 3: Windows Router + exclude Windows from the stub

**Files:**
- Create: `internal/netcfg/netcfg_windows.go`
- Modify: `internal/netcfg/netcfg_other.go`

Both changes land together: adding `windowsRouter.New()` while the stub's
`!linux` tag still matches Windows would cause a `New redeclared` build error,
so the stub's tag must be narrowed in the same task.

- [ ] **Step 1: Create `internal/netcfg/netcfg_windows.go`**

```go
//go:build windows

package netcfg

type windowsRouter struct{}

// New returns the Windows (netsh) router.
func New() Router { return windowsRouter{} }

func (windowsRouter) Up(c Config) error   { return runAll(winUpCommands(c)) }
func (windowsRouter) Down(c Config) error { return runAll(winDownCommands(c)) }
```

- [ ] **Step 2: Narrow the stub's build constraint in `internal/netcfg/netcfg_other.go`**

Change the first line from:

```go
//go:build !linux
```

to:

```go
//go:build !linux && !windows
```

- [ ] **Step 3: Verify both Windows and macOS builds**

Run: `GOOS=windows go build ./internal/netcfg/ && GOOS=darwin go build ./internal/netcfg/`
Expected: both succeed. Windows uses `windowsRouter` (no `New` redeclaration);
macOS still uses the stub.

- [ ] **Step 4: Commit**

```bash
git add internal/netcfg/netcfg_windows.go internal/netcfg/netcfg_other.go
git commit -m "feat(netcfg): Windows router (netsh), keep macOS stub"
```

---

## Task 4: Full verification + memory update

- [ ] **Step 1: Run the whole test suite on Linux**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 2: Cross-compile all three OSes**

Run: `GOOS=linux go build ./... ; GOOS=windows go build ./internal/... ; GOOS=darwin go build ./internal/...`
Expected: all succeed. (Top-level GUI packages are behind the `wails` tag; build
`./internal/...` for the non-Linux targets.)

- [ ] **Step 3: Manual e2e on a Windows host under Administrator (document result)**

Cannot run in WSL2/CI; perform on the Windows host and record the outcome:

1. Build the GUI for Windows (per the project's wails build).
2. Run elevated (Administrator), select TUN mode, enable Telegram, connect.
3. Confirm the Wintun adapter `tun0` has address `198.18.0.1`:
   `netsh interface ipv4 show addresses name=tun0`
4. Confirm Telegram CIDRs route to the adapter: `route print` (or
   `netsh interface ipv4 show route`) lists e.g. `149.154.160.0/20` on `tun0`.
5. Confirm a Telegram desktop client connects.
6. Disconnect; confirm the Telegram routes are gone (`route print`).

- [ ] **Step 4: Update the project memory phase status**

In `/home/zki/.claude/projects/-home-zki-projects-vless-client/memory/project-vless-client.md`,
mark Windows netcfg DONE under Phase 4 (note macOS remains the stub / the only
remaining Phase 4 routing TODO), referencing this plan and the spec
`docs/superpowers/specs/2026-06-04-vless-client-phase4-netcfg-windows-design.md`.
