# VLESS Client — Phase 2: Traffic Capture (sysproxy + TUN PoC) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add traffic-capture on top of the Phase 1 headless core: a full cross-platform system-proxy (SOCKS) mode, privilege detection, an orchestrator that wires xray-core + capture together, and a Linux proof-of-concept TUN mode that routes only whitelisted CIDRs into a TUN device (no routing loop).

**Architecture:** New packages: `internal/privilege` (elevation check), `internal/sysproxy` (set/clear system SOCKS proxy per OS), `internal/tun` (tun2socks engine wrapper), `internal/netcfg` (Linux TUN IP+route management), `internal/tunnel` (orchestrator coordinating core + capture). Each platform implementation splits into a pure command/argument *builder* (unit-tested without privileges) plus a thin exec runner (integration-tested where possible). The orchestrator depends on small interfaces so its start/stop sequencing is unit-tested with fakes.

**Tech Stack:** Go 1.26, existing `internal/{vless,routing,xrayconf,core,store}`, `github.com/xjasonlyu/tun2socks/v2` (gVisor netstack, pure-Go, no cgo), `golang.org/x/sys` (Windows token + registry). tun2socks uses wintun on Windows, utun on macOS, /dev/net/tun on Linux.

**Phasing note:** Phase 2 of 4 (re-cut). This phase delivers a usable cross-platform product in **proxy mode** (Telegram Desktop honors system SOCKS5) plus a Linux-verifiable TUN data path. Deferred to later phases: correct full-tunnel TUN with loop avoidance (fwmark/policy-routing) on Windows/macOS, kill switch, autostart, stats, GUI.

**Loop-avoidance decision (PoC):** Whitelist routing means most traffic is `direct`. If we routed the default route into TUN, xray's own `direct` outbound packets would re-enter TUN → infinite loop. The PoC instead routes ONLY the whitelisted CIDRs (e.g. `routing.TelegramCIDRs`) into the TUN. Direct traffic never enters TUN, so there is no loop, and the server connection (not in the whitelist CIDRs) egresses normally.

---

### Task 1: privilege detection

**Files:**
- Create: `internal/privilege/privilege.go`
- Create: `internal/privilege/privilege_unix.go`
- Create: `internal/privilege/privilege_windows.go`
- Test: `internal/privilege/privilege_test.go`

- [ ] **Step 1: Write the failing test**

`internal/privilege/privilege_test.go`:
```go
package privilege

import "testing"

func TestIsElevatedReturnsBool(t *testing.T) {
	// We can't assert the value (depends on how tests run), but the call
	// must not panic and must return without error on this platform.
	_ = IsElevated()
}

func TestRequireElevatedMessage(t *testing.T) {
	err := RequireElevated("TUN mode")
	// On a non-root CI run this returns an error; if running as root it's nil.
	if err != nil && err.Error() == "" {
		t.Error("RequireElevated returned an empty error message")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/privilege/ -v`
Expected: FAIL — package does not compile (`undefined: IsElevated`).

- [ ] **Step 3: Write the shared API**

`internal/privilege/privilege.go`:
```go
// Package privilege reports whether the process has the OS privileges
// (root / Administrator) required for TUN-mode networking.
package privilege

import "fmt"

// RequireElevated returns a descriptive error if the process is not elevated.
// purpose names the feature needing elevation, for the message.
func RequireElevated(purpose string) error {
	if IsElevated() {
		return nil
	}
	return fmt.Errorf("%s requires administrator/root privileges; re-run elevated or use proxy mode", purpose)
}
```

- [ ] **Step 4: Write the unix implementation**

`internal/privilege/privilege_unix.go`:
```go
//go:build linux || darwin

package privilege

import "os"

// IsElevated reports whether the effective user is root.
func IsElevated() bool {
	return os.Geteuid() == 0
}
```

- [ ] **Step 5: Write the windows implementation**

`internal/privilege/privilege_windows.go`:
```go
//go:build windows

package privilege

import "golang.org/x/sys/windows"

// IsElevated reports whether the process token is elevated (Administrator).
func IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}
```

Note: `windows.Token` has an `IsElevated()` method in `golang.org/x/sys/windows`. Verify with `go doc golang.org/x/sys/windows.Token.IsElevated` after `go get golang.org/x/sys`. If absent in the pulled version, use `token.GetTokenElevation()` or query `TokenElevation` via `windows.GetTokenInformation`; adapt and note what you used.

- [ ] **Step 6: Add dependency, run test, confirm pass**

Run:
```bash
go get golang.org/x/sys/windows
go mod tidy
go test ./internal/privilege/ -v
gofmt -l internal/privilege/ && go vet ./internal/privilege/
```
Expected: tests PASS, no fmt/vet output. (`go get` for a windows-only import still records the dependency.)

- [ ] **Step 7: Commit**

```bash
git add internal/privilege/ go.mod go.sum
git commit -m "feat(privilege): elevation detection per OS"
```

---

### Task 2: sysproxy — interface + Linux

**Files:**
- Create: `internal/sysproxy/sysproxy.go`
- Create: `internal/sysproxy/sysproxy_linux.go`
- Test: `internal/sysproxy/sysproxy_linux_test.go`

The interface is `Proxy` with `Set(host string, port int) error` and `Clear() error`. Each platform file provides `New() Proxy` and a pure command-builder that the test exercises.

- [ ] **Step 1: Write the failing test**

`internal/sysproxy/sysproxy_linux_test.go`:
```go
//go:build linux

package sysproxy

import (
	"reflect"
	"testing"
)

func TestLinuxSetCommands(t *testing.T) {
	got := setCommands("127.0.0.1", 10808)
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", "127.0.0.1"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", "10808"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("setCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestLinuxClearCommands(t *testing.T) {
	got := clearCommands()
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "none"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clearCommands =\n%v\nwant\n%v", got, want)
	}
}
```

- [ ] **Step 2: Run test, confirm fail**

Run: `go test ./internal/sysproxy/ -v`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write the interface**

`internal/sysproxy/sysproxy.go`:
```go
// Package sysproxy sets and clears the OS-wide SOCKS proxy.
package sysproxy

// Proxy configures the system-wide SOCKS proxy.
type Proxy interface {
	// Set points the system SOCKS proxy at host:port.
	Set(host string, port int) error
	// Clear disables the system SOCKS proxy.
	Clear() error
}
```

- [ ] **Step 4: Write the Linux implementation**

`internal/sysproxy/sysproxy_linux.go`:
```go
package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
)

type linuxProxy struct{}

// New returns the Linux (GNOME gsettings) system proxy controller.
func New() Proxy { return linuxProxy{} }

func setCommands(host string, port int) [][]string {
	return [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", host},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", strconv.Itoa(port)},
	}
}

func clearCommands() [][]string {
	return [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "none"},
	}
}

func runAll(cmds [][]string) error {
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", c, err, out)
		}
	}
	return nil
}

func (linuxProxy) Set(host string, port int) error { return runAll(setCommands(host, port)) }
func (linuxProxy) Clear() error                    { return runAll(clearCommands()) }
```

- [ ] **Step 5: Run test, confirm pass**

Run: `go test ./internal/sysproxy/ -v && gofmt -l internal/sysproxy/ && go vet ./internal/sysproxy/`
Expected: PASS, no fmt/vet output.

- [ ] **Step 6: Commit**

```bash
git add internal/sysproxy/
git commit -m "feat(sysproxy): interface and Linux (gsettings) implementation"
```

---

### Task 3: sysproxy — macOS

**Files:**
- Create: `internal/sysproxy/sysproxy_darwin.go`
- Test: `internal/sysproxy/sysproxy_darwin_test.go`

macOS uses `networksetup`. The network service name (e.g. "Wi-Fi") is discovered at runtime; the pure builder takes it as a parameter so it is testable.

- [ ] **Step 1: Write the failing test**

`internal/sysproxy/sysproxy_darwin_test.go`:
```go
//go:build darwin

package sysproxy

import (
	"reflect"
	"testing"
)

func TestDarwinSetCommands(t *testing.T) {
	got := setCommands("Wi-Fi", "127.0.0.1", 10808)
	want := [][]string{
		{"networksetup", "-setsocksfirewallproxy", "Wi-Fi", "127.0.0.1", "10808"},
		{"networksetup", "-setsocksfirewallproxystate", "Wi-Fi", "on"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("setCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestDarwinClearCommands(t *testing.T) {
	got := clearCommands("Wi-Fi")
	want := [][]string{
		{"networksetup", "-setsocksfirewallproxystate", "Wi-Fi", "off"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clearCommands =\n%v\nwant\n%v", got, want)
	}
}
```

- [ ] **Step 2: Run test, confirm fail**

Run: `GOOS=darwin go vet ./internal/sysproxy/` first to confirm it compiles for darwin, then on a mac `go test ./internal/sysproxy/ -v`. Off-mac, rely on `GOOS=darwin go build ./internal/sysproxy/`.
Expected: build/compile FAIL — `undefined: setCommands` (darwin file not written yet).

- [ ] **Step 3: Write the implementation**

`internal/sysproxy/sysproxy_darwin.go`:
```go
package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type darwinProxy struct{}

// New returns the macOS (networksetup) system proxy controller.
func New() Proxy { return darwinProxy{} }

func setCommands(service, host string, port int) [][]string {
	return [][]string{
		{"networksetup", "-setsocksfirewallproxy", service, host, strconv.Itoa(port)},
		{"networksetup", "-setsocksfirewallproxystate", service, "on"},
	}
}

func clearCommands(service string) [][]string {
	return [][]string{
		{"networksetup", "-setsocksfirewallproxystate", service, "off"},
	}
}

// primaryService returns the first active network service (e.g. "Wi-Fi").
func primaryService() (string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return "", fmt.Errorf("list network services: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Skip the header line and disabled services (prefixed with '*').
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("no active network service found")
}

func runAll(cmds [][]string) error {
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", c, err, out)
		}
	}
	return nil
}

func (darwinProxy) Set(host string, port int) error {
	svc, err := primaryService()
	if err != nil {
		return err
	}
	return runAll(setCommands(svc, host, port))
}

func (darwinProxy) Clear() error {
	svc, err := primaryService()
	if err != nil {
		return err
	}
	return runAll(clearCommands(svc))
}
```

- [ ] **Step 4: Verify build, run test (on mac)**

Run: `GOOS=darwin go build ./internal/sysproxy/` (off-mac) and, on a mac, `go test ./internal/sysproxy/ -v`.
Expected: builds clean; on mac, the two darwin tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sysproxy/sysproxy_darwin.go internal/sysproxy/sysproxy_darwin_test.go
git commit -m "feat(sysproxy): macOS (networksetup) implementation"
```

---

### Task 4: sysproxy — Windows

**Files:**
- Create: `internal/sysproxy/sysproxy_windows.go`
- Test: `internal/sysproxy/sysproxy_windows_test.go`

Windows sets `ProxyServer`/`ProxyEnable` under `HKCU\...\Internet Settings` then notifies WinINet. The pure builder computes the `ProxyServer` value string and is unit-tested; registry/WinINet calls are integration (Windows only).

- [ ] **Step 1: Write the failing test**

`internal/sysproxy/sysproxy_windows_test.go`:
```go
//go:build windows

package sysproxy

import "testing"

func TestProxyServerValue(t *testing.T) {
	got := proxyServerValue("127.0.0.1", 10808)
	want := "socks=127.0.0.1:10808"
	if got != want {
		t.Errorf("proxyServerValue = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Confirm fail**

Run (off-Windows): `GOOS=windows go build ./internal/sysproxy/`
Expected: FAIL — `undefined: proxyServerValue`.

- [ ] **Step 3: Write the implementation**

`internal/sysproxy/sysproxy_windows.go`:
```go
package sysproxy

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsProxy struct{}

// New returns the Windows (WinINet registry) system proxy controller.
func New() Proxy { return windowsProxy{} }

const inetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func proxyServerValue(host string, port int) string {
	return fmt.Sprintf("socks=%s:%d", host, port)
}

func openSettings() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, inetSettingsPath, registry.SET_VALUE)
}

func (windowsProxy) Set(host string, port int) error {
	k, err := openSettings()
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue("ProxyServer", proxyServerValue(host, port)); err != nil {
		return err
	}
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	return refreshWinINet()
}

func (windowsProxy) Clear() error {
	k, err := openSettings()
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	return refreshWinINet()
}

// refreshWinINet notifies WinINet that proxy settings changed so they take
// effect without a reboot.
func refreshWinINet() error {
	const (
		internetOptionSettingsChanged = 39
		internetOptionRefresh         = 37
	)
	wininet := windows.NewLazySystemDLL("wininet.dll")
	proc := wininet.NewProc("InternetSetOptionW")
	for _, opt := range []uintptr{internetOptionSettingsChanged, internetOptionRefresh} {
		// InternetSetOptionW(NULL, opt, NULL, 0)
		r, _, err := proc.Call(0, opt, uintptr(unsafe.Pointer(nil)), 0)
		if r == 0 {
			return fmt.Errorf("InternetSetOption(%d): %w", opt, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Verify build + run test (on Windows)**

Run (off-Windows): `GOOS=windows go build ./internal/sysproxy/` and `GOOS=windows go vet ./internal/sysproxy/`.
On Windows: `go test ./internal/sysproxy/ -v`.
Expected: builds clean; on Windows `TestProxyServerValue` PASSES. Verify `registry.Key` has `SetStringValue`/`SetDWordValue` and `windows.NewLazySystemDLL` exists via `go doc`; adapt if the API differs.

- [ ] **Step 5: Commit**

```bash
git add internal/sysproxy/sysproxy_windows.go internal/sysproxy/sysproxy_windows_test.go
git commit -m "feat(sysproxy): Windows (WinINet registry) implementation"
```

---

### Task 5: tun2socks engine wrapper

**Files:**
- Create: `internal/tun/tun.go`
- Test: `internal/tun/tun_test.go`
- Modify: `go.mod`/`go.sum`

Thin wrapper around the tun2socks v2 `engine` package. tun2socks creates and owns the TUN device and forwards its TCP/UDP to a SOCKS proxy.

- [ ] **Step 1: Add dependency**

Run:
```bash
go get github.com/xjasonlyu/tun2socks/v2@latest
go mod tidy
```
Expected: module added (pulls gVisor netstack — pure Go). If `@latest` fails to build on Go 1.26, pin a recent tag and report which.

- [ ] **Step 2: Inspect the engine API**

Run: `go doc github.com/xjasonlyu/tun2socks/v2/engine` and `go doc github.com/xjasonlyu/tun2socks/v2/engine.Key`.
Confirm: `Key` struct with at least `Device string`, `Proxy string`, `LogLevel string`; package funcs `Insert(*Key)`, `Start()`, `Stop()`. The wrapper below assumes this shape — **adapt field/func names to what `go doc` reports** and note any change.

- [ ] **Step 3: Write the failing test**

`internal/tun/tun_test.go`:
```go
package tun

import (
	"os"
	"testing"
)

func TestStartRequiresProxyURL(t *testing.T) {
	if err := Start("tun0", ""); err == nil {
		t.Error("Start with empty proxy URL should error")
	}
}

func TestStartStopLinux(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("creating a TUN device requires root; skipping")
	}
	// Use the SOCKS proxy a running xray would expose; here just any URL —
	// the device must be created and torn down cleanly.
	if err := Start("tuntest0", "socks5://127.0.0.1:10808"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
```

- [ ] **Step 4: Confirm fail**

Run: `go test ./internal/tun/ -v`
Expected: FAIL — `undefined: Start`.

- [ ] **Step 5: Write the wrapper**

`internal/tun/tun.go`:
```go
// Package tun runs a tun2socks engine that forwards a TUN device's traffic
// to a SOCKS proxy. The engine creates and owns the TUN device; IP and route
// configuration is handled separately (see internal/netcfg).
package tun

import (
	"errors"

	"github.com/xjasonlyu/tun2socks/v2/engine"
)

// Start brings up a TUN device named `device` and forwards its traffic to
// socksURL (e.g. "socks5://127.0.0.1:10808"). Requires elevated privileges.
func Start(device, socksURL string) error {
	if socksURL == "" {
		return errors.New("tun: empty proxy URL")
	}
	if device == "" {
		return errors.New("tun: empty device name")
	}
	key := &engine.Key{
		Device:   "tun://" + device,
		Proxy:    socksURL,
		LogLevel: "warning",
	}
	engine.Insert(key)
	engine.Start()
	return nil
}

// Stop tears down the engine and its TUN device.
func Stop() error {
	engine.Stop()
	return nil
}
```

Note: if `engine.Start()`/`engine.Stop()` return errors in the pulled version, propagate them. If the device string format differs (some versions want just the name, or `fd://`), use what `go doc`/examples show. The package uses global engine state — only one tunnel at a time, which matches this app.

- [ ] **Step 6: Run test, confirm pass**

Run: `go test ./internal/tun/ -v` (the root-only test self-skips when unprivileged), then `gofmt -l internal/tun/ && go vet ./internal/tun/`.
Expected: `TestStartRequiresProxyURL` PASS; `TestStartStopLinux` PASS if run as root, else SKIP.

- [ ] **Step 7: Commit**

```bash
git add internal/tun/ go.mod go.sum
git commit -m "feat(tun): tun2socks engine wrapper"
```

---

### Task 6: netcfg — Linux TUN IP + route management

**Files:**
- Create: `internal/netcfg/netcfg.go`
- Create: `internal/netcfg/netcfg_linux.go`
- Test: `internal/netcfg/netcfg_linux_test.go`

PoC scope: assign an IP to the TUN device, bring it up, and route a set of whitelisted CIDRs into it. Pure command builders are unit-tested; the exec runner is integration (root).

- [ ] **Step 1: Write the failing test**

`internal/netcfg/netcfg_linux_test.go`:
```go
//go:build linux

package netcfg

import (
	"reflect"
	"testing"
)

func TestLinuxUpCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20", "91.108.4.0/22"}}
	got := upCommands(c)
	want := [][]string{
		{"ip", "addr", "add", "198.18.0.1/15", "dev", "tun0"},
		{"ip", "link", "set", "dev", "tun0", "up"},
		{"ip", "route", "add", "149.154.160.0/20", "dev", "tun0"},
		{"ip", "route", "add", "91.108.4.0/22", "dev", "tun0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("upCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestLinuxDownCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20"}}
	got := downCommands(c)
	want := [][]string{
		{"ip", "route", "del", "149.154.160.0/20", "dev", "tun0"},
		{"ip", "addr", "del", "198.18.0.1/15", "dev", "tun0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("downCommands =\n%v\nwant\n%v", got, want)
	}
}
```

- [ ] **Step 2: Confirm fail**

Run: `go test ./internal/netcfg/ -v`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write the shared config + Router**

`internal/netcfg/netcfg.go`:
```go
// Package netcfg configures the TUN device IP address and the routes that
// steer whitelisted destinations into the tunnel.
package netcfg

// Config describes the TUN device and which CIDRs to route into it.
type Config struct {
	Device     string   // TUN device name, e.g. "tun0"
	TunIP      string   // device IP, e.g. "198.18.0.1"
	Prefix     int      // device IP prefix length, e.g. 15
	RouteCIDRs []string // destination CIDRs to route into the TUN
}

// Router applies and reverts TUN IP + routing configuration.
type Router interface {
	Up(c Config) error
	Down(c Config) error
}
```

- [ ] **Step 4: Write the Linux implementation**

`internal/netcfg/netcfg_linux.go`:
```go
package netcfg

import (
	"fmt"
	"os/exec"
	"strconv"
)

type linuxRouter struct{}

// New returns the Linux (iproute2) router.
func New() Router { return linuxRouter{} }

func cidr(ip string, prefix int) string {
	return ip + "/" + strconv.Itoa(prefix)
}

func upCommands(c Config) [][]string {
	cmds := [][]string{
		{"ip", "addr", "add", cidr(c.TunIP, c.Prefix), "dev", c.Device},
		{"ip", "link", "set", "dev", c.Device, "up"},
	}
	for _, r := range c.RouteCIDRs {
		cmds = append(cmds, []string{"ip", "route", "add", r, "dev", c.Device})
	}
	return cmds
}

func downCommands(c Config) [][]string {
	var cmds [][]string
	for _, r := range c.RouteCIDRs {
		cmds = append(cmds, []string{"ip", "route", "del", r, "dev", c.Device})
	}
	cmds = append(cmds, []string{"ip", "addr", "del", cidr(c.TunIP, c.Prefix), "dev", c.Device})
	return cmds
}

func runAll(cmds [][]string) error {
	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", cmd, err, out)
		}
	}
	return nil
}

func (linuxRouter) Up(c Config) error   { return runAll(upCommands(c)) }
func (linuxRouter) Down(c Config) error { return runAll(downCommands(c)) }
```

- [ ] **Step 5: Run test, confirm pass**

Run: `go test ./internal/netcfg/ -v && gofmt -l internal/netcfg/ && go vet ./internal/netcfg/`
Expected: PASS, no fmt/vet output.

- [ ] **Step 6: Commit**

```bash
git add internal/netcfg/
git commit -m "feat(netcfg): Linux TUN IP and whitelist-CIDR route management"
```

---

### Task 7: tunnel orchestrator

**Files:**
- Create: `internal/tunnel/tunnel.go`
- Test: `internal/tunnel/tunnel_test.go`

Coordinates capture. Depends on small interfaces (injected) so start/stop sequencing and rollback are unit-tested with fakes. Two modes: `proxy` (start core, set system proxy) and `tun` (start core, start tun2socks, apply routes).

- [ ] **Step 1: Write the failing test**

`internal/tunnel/tunnel_test.go`:
```go
package tunnel

import (
	"errors"
	"testing"

	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/store"
)

type fakeStopper struct{ stopped bool }

func (f *fakeStopper) Stop() error { f.stopped = true; return nil }

type fakeCore struct {
	started bool
	inst    *fakeStopper
	err     error
}

func (f *fakeCore) Start(_ []byte) (Stopper, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.started = true
	f.inst = &fakeStopper{}
	return f.inst, nil
}

type fakeProxy struct{ set, cleared bool }

func (f *fakeProxy) Set(_ string, _ int) error { f.set = true; return nil }
func (f *fakeProxy) Clear() error              { f.cleared = true; return nil }

type fakeTun struct{ started, stopped bool }

func (f *fakeTun) Start(_, _ string) error { f.started = true; return nil }
func (f *fakeTun) Stop() error             { f.stopped = true; return nil }

type fakeRouter struct{ up, down bool }

func (f *fakeRouter) Up(_ netcfg.Config) error   { f.up = true; return nil }
func (f *fakeRouter) Down(_ netcfg.Config) error { f.down = true; return nil }

func newDeps() (*fakeCore, *fakeProxy, *fakeTun, *fakeRouter, Deps) {
	c, p, tn, r := &fakeCore{}, &fakeProxy{}, &fakeTun{}, &fakeRouter{}
	return c, p, tn, r, Deps{Core: c, Proxy: p, Tun: tn, Router: r}
}

func baseConfig(mode store.Mode) Config {
	return Config{
		XrayJSON: []byte("{}"), SocksHost: "127.0.0.1", SocksPort: 10808,
		Mode: mode, Device: "tun0", TunIP: "198.18.0.1", TunPrefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20"},
	}
}

func TestProxyModeStartStop(t *testing.T) {
	c, p, _, _, deps := newDeps()
	tn := New(baseConfig(store.ModeProxy), deps)
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !c.started || !p.set {
		t.Errorf("proxy mode should start core and set proxy: core=%v proxy=%v", c.started, p.set)
	}
	if err := tn.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !p.cleared || !c.inst.stopped {
		t.Errorf("proxy mode stop should clear proxy and stop core")
	}
}

func TestTunModeStartStop(t *testing.T) {
	c, _, tnsvc, r, deps := newDeps()
	tn := New(baseConfig(store.ModeTUN), deps)
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !c.started || !tnsvc.started || !r.up {
		t.Errorf("tun mode should start core+tun and apply routes")
	}
	if err := tn.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !r.down || !tnsvc.stopped || !c.inst.stopped {
		t.Errorf("tun mode stop should revert routes, stop tun, stop core")
	}
}

func TestStartRollsBackOnCoreFailure(t *testing.T) {
	c, p, _, _, deps := newDeps()
	c.err = errors.New("boom")
	tn := New(baseConfig(store.ModeProxy), deps)
	if err := tn.Start(); err == nil {
		t.Fatal("expected Start to fail when core fails")
	}
	if p.set {
		t.Error("proxy must not be set when core failed to start")
	}
}
```

- [ ] **Step 2: Confirm fail**

Run: `go test ./internal/tunnel/ -v`
Expected: FAIL — package does not compile (`undefined: New`, `Deps`, etc.).

- [ ] **Step 3: Write the orchestrator**

`internal/tunnel/tunnel.go`:
```go
// Package tunnel orchestrates traffic capture: it starts the xray-core
// instance and then either sets the system SOCKS proxy (proxy mode) or
// brings up a TUN device and routes whitelisted CIDRs into it (tun mode).
package tunnel

import (
	"fmt"

	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/store"
)

// Stopper stops a running xray-core instance (satisfied by *core.Instance).
type Stopper interface {
	Stop() error
}

// Core starts an xray-core instance from JSON config.
type Core interface {
	Start(jsonConfig []byte) (Stopper, error)
}

// Proxy sets/clears the system SOCKS proxy.
type Proxy interface {
	Set(host string, port int) error
	Clear() error
}

// Tun runs the tun2socks engine.
type Tun interface {
	Start(device, socksURL string) error
	Stop() error
}

// Deps are the injected platform dependencies.
type Deps struct {
	Core   Core
	Proxy  Proxy
	Tun    Tun
	Router netcfg.Router
}

// Config describes one tunnel session.
type Config struct {
	XrayJSON   []byte
	SocksHost  string
	SocksPort  int
	Mode       store.Mode
	Device     string
	TunIP      string
	TunPrefix  int
	RouteCIDRs []string
}

// Tunnel is a configured, possibly-running capture session.
type Tunnel struct {
	cfg  Config
	deps Deps
	inst Stopper
}

// New builds a Tunnel.
func New(cfg Config, deps Deps) *Tunnel {
	return &Tunnel{cfg: cfg, deps: deps}
}

func (t *Tunnel) socksURL() string {
	return fmt.Sprintf("socks5://%s:%d", t.cfg.SocksHost, t.cfg.SocksPort)
}

func (t *Tunnel) netcfgConfig() netcfg.Config {
	return netcfg.Config{
		Device:     t.cfg.Device,
		TunIP:      t.cfg.TunIP,
		Prefix:     t.cfg.TunPrefix,
		RouteCIDRs: t.cfg.RouteCIDRs,
	}
}

// Start launches the tunnel. On any step failure it rolls back prior steps.
func (t *Tunnel) Start() error {
	inst, err := t.deps.Core.Start(t.cfg.XrayJSON)
	if err != nil {
		return fmt.Errorf("start core: %w", err)
	}
	t.inst = inst

	switch t.cfg.Mode {
	case store.ModeProxy:
		if err := t.deps.Proxy.Set(t.cfg.SocksHost, t.cfg.SocksPort); err != nil {
			_ = t.inst.Stop()
			t.inst = nil
			return fmt.Errorf("set system proxy: %w", err)
		}
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
	default:
		_ = t.inst.Stop()
		t.inst = nil
		return fmt.Errorf("unknown mode %q", t.cfg.Mode)
	}
	return nil
}

// Stop reverts capture and stops core. It attempts every step and returns the
// first error encountered.
func (t *Tunnel) Stop() error {
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	switch t.cfg.Mode {
	case store.ModeProxy:
		record(t.deps.Proxy.Clear())
	case store.ModeTUN:
		record(t.deps.Router.Down(t.netcfgConfig()))
		record(t.deps.Tun.Stop())
	}
	if t.inst != nil {
		record(t.inst.Stop())
		t.inst = nil
	}
	return firstErr
}
```

- [ ] **Step 4: Run test, confirm pass**

Run: `go test ./internal/tunnel/ -v && gofmt -l internal/tunnel/ && go vet ./internal/tunnel/`
Expected: all PASS, no fmt/vet output.

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/
git commit -m "feat(tunnel): orchestrator for proxy and tun capture modes"
```

---

### Task 8: headless `-mode` flag + adapters

**Files:**
- Modify: `cmd/headless/main.go`

Wire the real packages into the orchestrator via thin adapters (the interfaces differ slightly from concrete signatures). Add a `-mode proxy|tun` flag (default `proxy`).

- [ ] **Step 1: Replace main.go**

`cmd/headless/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zki/vless-client/internal/core"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
	"github.com/zki/vless-client/internal/tun"
	"github.com/zki/vless-client/internal/tunnel"
	"github.com/zki/vless-client/internal/vless"
	"github.com/zki/vless-client/internal/xrayconf"
)

// coreAdapter adapts core.Start to the tunnel.Core interface.
type coreAdapter struct{}

func (coreAdapter) Start(jsonConfig []byte) (tunnel.Stopper, error) {
	return core.Start(jsonConfig)
}

// tunAdapter adapts the package-level tun funcs to the tunnel.Tun interface.
type tunAdapter struct{}

func (tunAdapter) Start(device, socksURL string) error { return tun.Start(device, socksURL) }
func (tunAdapter) Stop() error                         { return tun.Stop() }

func main() {
	link := flag.String("link", "", "vless:// share link")
	port := flag.Int("port", 10808, "local SOCKS inbound port")
	mode := flag.String("mode", "proxy", "capture mode: proxy | tun")
	device := flag.String("device", "tun0", "TUN device name (tun mode)")
	flag.Parse()

	if *link == "" {
		log.Fatal("usage: headless -link 'vless://...' [-port 10808] [-mode proxy|tun]")
	}

	m := store.Mode(*mode)
	if m != store.ModeProxy && m != store.ModeTUN {
		log.Fatalf("invalid -mode %q (want proxy or tun)", *mode)
	}
	if m == store.ModeTUN {
		if err := privilege.RequireElevated("TUN mode"); err != nil {
			log.Fatal(err)
		}
	}

	srv, err := vless.Parse(*link)
	if err != nil {
		log.Fatalf("parse link: %v", err)
	}
	fmt.Printf("server: %s (%s:%d) mode=%s\n", srv.Name, srv.Host, srv.Port, m)

	cfgJSON, err := xrayconf.Build(srv, routing.Default(), xrayconf.Options{SocksPort: *port})
	if err != nil {
		log.Fatalf("build config: %v", err)
	}

	tn := tunnel.New(tunnel.Config{
		XrayJSON:   cfgJSON,
		SocksHost:  "127.0.0.1",
		SocksPort:  *port,
		Mode:       m,
		Device:     *device,
		TunIP:      "198.18.0.1",
		TunPrefix:  15,
		RouteCIDRs: routing.TelegramCIDRs,
	}, tunnel.Deps{
		Core:   coreAdapter{},
		Proxy:  sysproxy.New(),
		Tun:    tunAdapter{},
		Router: netcfg.New(),
	})

	if err := tn.Start(); err != nil {
		log.Fatalf("start tunnel: %v", err)
	}
	fmt.Printf("tunnel up (mode=%s). Ctrl-C to stop.\n", m)
	if m == store.ModeProxy {
		fmt.Printf("system SOCKS proxy -> 127.0.0.1:%d\n", *port)
	} else {
		fmt.Printf("routing Telegram CIDRs into %s\n", *device)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := tn.Stop(); err != nil {
		log.Printf("stop: %v", err)
	}
	fmt.Println("stopped.")
}
```

Note: `netcfg.New()` only exists on Linux (Task 6 wrote `netcfg_linux.go`). For Windows/macOS builds of this PoC, this will fail to compile until those phases add `netcfg_<goos>.go`. That is expected — the headless TUN PoC is Linux-only this phase. (Proxy mode works on all three OSes.) Likewise `tun2socks` builds cross-platform, but routing is Linux-only here.

- [ ] **Step 2: Build + vet (Linux) and full test**

Run:
```bash
go build ./...
go vet ./...
gofmt -l .
go test ./...
```
Expected: builds clean, no fmt/vet output, all tests pass (root-gated ones skip).

- [ ] **Step 3: Cross-build sanity for proxy mode**

The TUN PoC main wires `netcfg.New()`, which is Linux-only — so `cmd/headless` cross-builds will fail on darwin/windows this phase. Confirm the *library* packages that should be cross-platform still build:
```bash
GOOS=darwin  go build ./internal/...
GOOS=windows go build ./internal/...
```
Expected: both succeed (privilege, sysproxy, tun, tunnel, netcfg all have the right per-OS files; note `netcfg` has only a Linux file, so `GOOS=darwin go build ./internal/netcfg/` WILL fail — that is the known PoC limitation). Run the per-package builds that should pass:
```bash
GOOS=darwin  go build ./internal/sysproxy/ ./internal/privilege/ ./internal/tun/ ./internal/tunnel/
GOOS=windows go build ./internal/sysproxy/ ./internal/privilege/ ./internal/tun/ ./internal/tunnel/
```
Expected: all succeed. Record any failures as findings.

- [ ] **Step 4: Manual e2e — proxy mode (no root)**

```bash
export XRAY_LOCATION_ASSET=~/.config/xray-assets   # geo .dat from Phase 1
go run ./cmd/headless -link 'vless://YOUR-REAL-LINK' -mode proxy -port 10808
```
Expected: prints "tunnel up (mode=proxy)". With a GNOME session, `gsettings get org.gnome.system.proxy mode` reports `manual`. (On WSL2 without GNOME, gsettings may error — that is environment, not a code bug; note it.) Ctrl-C restores `mode=none`.

- [ ] **Step 5: Manual e2e — TUN PoC (root)**

```bash
export XRAY_LOCATION_ASSET=~/.config/xray-assets
sudo -E go run ./cmd/headless -link 'vless://YOUR-REAL-LINK' -mode tun -device tun0 -port 10808
```
In another shell, confirm Telegram CIDRs route into the TUN and reach Telegram via the proxy, while other traffic is unaffected:
```bash
ip route get 149.154.167.51        # should show dev tun0
curl -s --max-time 8 https://api.telegram.org/ -o /dev/null -w "telegram=%{http_code}\n"  # via tunnel
curl -s --max-time 8 https://api.ipify.org -w "\nnon-tunneled egress IP above\n"          # direct, unchanged
```
Expected: `ip route get` for a Telegram IP shows `dev tun0`; Telegram reachable; non-Telegram traffic unaffected and not looping. Ctrl-C removes routes and the device.

- [ ] **Step 6: Commit**

```bash
git add cmd/headless/main.go
git commit -m "feat(cmd): -mode proxy|tun via tunnel orchestrator"
```

---

### Task 9: cross-build verification + CI matrix note

**Files:**
- Create: `docs/superpowers/phase2-build-notes.md`

- [ ] **Step 1: Record verified build matrix**

Create `docs/superpowers/phase2-build-notes.md`:
```markdown
# Phase 2 build/verification notes

## Cross-platform build status (this phase)

| Package | linux | darwin | windows |
|---|---|---|---|
| internal/privilege | built+tested | builds | builds |
| internal/sysproxy  | built+tested | builds (test on mac) | builds (test on win) |
| internal/tun       | built (+root smoke) | builds | builds |
| internal/netcfg    | built+tested | NOT built (Linux-only PoC) | NOT built (Linux-only PoC) |
| internal/tunnel    | built+tested | builds | builds |
| cmd/headless       | built (proxy+tun) | NOT built (needs netcfg) | NOT built (needs netcfg) |

## Manual verification done
- proxy mode: Linux (gsettings) — note GNOME requirement.
- tun PoC: Linux as root — Telegram CIDRs route via tun0, no loop.

## Deferred (later phases)
- netcfg for darwin/windows + full default-route TUN with loop avoidance
  (fwmark/policy-routing on Linux, bind-to-interface on macOS, route metrics
  on Windows).
- Kill switch, autostart, traffic stats, GUI.
- CI matrix: GitHub Actions ubuntu/macos/windows runners running
  `go build ./...` + `go test ./...` per OS once netcfg is implemented for all.
```

- [ ] **Step 2: Run the build matrix and reconcile the table**

Run the commands from Task 8 Step 3 plus `go test ./...`, and edit the table above to match actual results. Fix any unexpected build failure in the relevant package before committing.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/phase2-build-notes.md
git commit -m "docs: phase 2 build matrix and deferred work"
```

---

## Phase 2 Done — Definition of Done

- `go test ./...` green (root-gated TUN tests skip when unprivileged).
- Proxy mode works end-to-end on Linux; `sysproxy` builds for all three OSes; macOS/Windows builders unit-tested on their platforms.
- TUN PoC works on Linux as root: whitelisted Telegram CIDRs route through the tunnel with no routing loop; other traffic unaffected.
- `privilege.RequireElevated` gates TUN mode with a clear message.
- Build matrix documented.

## What Phase 2 deliberately leaves out (later phases)

- Full default-route TUN with loop avoidance (fwmark/policy-routing) on all OSes.
- netcfg implementations for macOS and Windows.
- Kill switch, autostart, ping/latency, traffic stats.
- Wails GUI (Phase 3).
