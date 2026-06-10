# Autostart (launch on login) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register the app to launch at user login (Windows HKCU Run / macOS LaunchAgent), with Settings UI toggles for both AutoStart and AutoConnect.

**Architecture:** A new `internal/autostart` package provides a per-OS `Manager` (Enable/Disable/Supported), mirroring `internal/privilege`. The app service applies it on AutoStart-toggle delta (before persist) and refreshes it on startup; capability is surfaced via `CapsDTO.AutostartSupported` so the frontend hides the toggle where unsupported.

**Tech Stack:** Go (`golang.org/x/sys/windows/registry`, `os/exec` launchctl), Wails v2, vanilla TypeScript.

**Spec:** `docs/superpowers/specs/2026-06-10-autostart-design.md`

---

## File Structure

- `internal/autostart/autostart.go` — `Manager` interface + pure builders (`plistContent`, `runValue`) + macLabel const.
- `internal/autostart/autostart_windows.go` — `winManager` (registry Run key).
- `internal/autostart/autostart_darwin.go` — `macManager` (LaunchAgent plist + launchctl).
- `internal/autostart/autostart_other.go` — `noopManager` stub.
- `internal/autostart/autostart_test.go` — builder tests (untagged, run on Linux).
- `internal/app/types.go` — `AutostartManager` interface, `Deps.Autostart`, `CapsDTO.AutostartSupported`.
- `internal/app/autostart.go` (new) — `autostartSupported`, `applyAutostart`, `ReconcileAutostart`.
- `internal/app/settings.go` — `UpdateSettings` delta apply.
- `internal/app/app.go` — `snapshot()` sets `AutostartSupported`.
- `internal/app/autostart_test.go` (new) — fake + app-level tests.
- `gui_app.go` — wire `autostart.New()` + `ReconcileAutostart()`.
- `frontend/wailsjs/go/models.ts`, `frontend/src/main.ts`, `frontend/index.html` — UI.

---

## Task 1: `internal/autostart` package

**Files:**
- Create: `internal/autostart/autostart.go`, `autostart_windows.go`, `autostart_darwin.go`, `autostart_other.go`, `autostart_test.go`

- [ ] **Step 1: Write the failing builder tests**

Create `internal/autostart/autostart_test.go`:

```go
package autostart

import (
	"strings"
	"testing"
)

func TestPlistContent(t *testing.T) {
	out := plistContent("com.example.app", "/Applications/App.app/Contents/MacOS/app")
	for _, want := range []string{
		"com.example.app",
		"/Applications/App.app/Contents/MacOS/app",
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plist missing %q in:\n%s", want, out)
		}
	}
}

func TestRunValue(t *testing.T) {
	got := runValue(`C:\Program Files\KUB Connect\kub-connect.exe`)
	want := `"C:\Program Files\KUB Connect\kub-connect.exe"`
	if got != want {
		t.Errorf("runValue = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/autostart/ -v`
Expected: FAIL — `undefined: plistContent` (package/file doesn't exist yet).

- [ ] **Step 3: Create the shared file**

Create `internal/autostart/autostart.go`:

```go
// Package autostart registers the application to launch at user login. The
// concrete Manager is per-OS; obtain one with New().
package autostart

// Manager controls launch-on-login registration.
type Manager interface {
	// Supported reports whether autostart is implemented on this OS.
	Supported() bool
	// Enable registers the app to launch at user login, resolving its own exe
	// path. Idempotent: re-enabling overwrites the existing entry.
	Enable() error
	// Disable removes the login entry. A missing entry is not an error.
	Disable() error
}

// macLabel is the LaunchAgent label (reverse of the qb2b.pro domain); it does
// not depend on a bundle ID, which the build does not set.
const macLabel = "pro.qb2b.kub-connect"

// plistContent builds a macOS LaunchAgent plist for the given label and exe path.
func plistContent(label, execPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + execPath + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
}

// runValue builds the Windows Run registry value: the exe path wrapped in quotes
// so paths containing spaces (e.g. Program Files) launch correctly.
func runValue(execPath string) string {
	return `"` + execPath + `"`
}
```

- [ ] **Step 4: Create the per-OS files**

Create `internal/autostart/autostart_windows.go`:

```go
//go:build windows

package autostart

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	winRunKey     = `Software\Microsoft\Windows\CurrentVersion\Run`
	winValueName  = "KUB Connect"
)

type winManager struct{}

// New returns the Windows autostart manager.
func New() Manager { return winManager{} }

func (winManager) Supported() bool { return true }

func (winManager) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, winRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(winValueName, runValue(exe))
}

func (winManager) Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, winRunKey, registry.SET_VALUE)
	if err != nil {
		return nil // key absent → nothing registered
	}
	defer k.Close()
	if err := k.DeleteValue(winValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
```

Create `internal/autostart/autostart_darwin.go`:

```go
//go:build darwin

package autostart

import (
	"os"
	"os/exec"
	"path/filepath"
)

type macManager struct{}

// New returns the macOS autostart manager.
func New() Manager { return macManager{} }

func (macManager) Supported() bool { return true }

func macPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", macLabel+".plist"), nil
}

func (macManager) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := macPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(plistContent(macLabel, exe)), 0o644); err != nil {
		return err
	}
	// Best-effort immediate (re)load; the plist also takes effect at next login.
	_ = exec.Command("launchctl", "unload", "-w", path).Run()
	_ = exec.Command("launchctl", "load", "-w", path).Run()
	return nil
}

func (macManager) Disable() error {
	path, err := macPlistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", "-w", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
```

Create `internal/autostart/autostart_other.go`:

```go
//go:build !windows && !darwin

package autostart

import (
	"fmt"
	"runtime"
)

type noopManager struct{}

// New returns a no-op autostart manager for unsupported platforms.
func New() Manager { return noopManager{} }

func (noopManager) Supported() bool { return false }

func (noopManager) Enable() error {
	return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
}

func (noopManager) Disable() error { return nil }
```

- [ ] **Step 5: Run builder tests + cross-compile**

Run: `go test ./internal/autostart/ -v`
Expected: PASS (TestPlistContent, TestRunValue).
Run: `GOOS=windows GOARCH=amd64 go build ./internal/autostart/ && GOOS=darwin GOARCH=amd64 go build ./internal/autostart/`
Expected: both succeed (no output).

- [ ] **Step 6: Commit**

```bash
git add internal/autostart/
git commit -m "feat(autostart): per-OS launch-on-login manager (Windows Run / macOS LaunchAgent)"
```

---

## Task 2: app — Deps, interface, caps, helpers

**Files:**
- Modify: `internal/app/types.go` (Deps ~line 70; CapsDTO ~line 121)
- Create: `internal/app/autostart.go`
- Modify: `internal/app/app.go` (`snapshot()` Caps block ~line 59)
- Create/append: `internal/app/autostart_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/app/autostart_test.go`:

```go
package app

import "testing"

type fakeAutostart struct {
	supported    bool
	enableCalls  int
	disableCalls int
	enableErr    error
}

func (f *fakeAutostart) Supported() bool { return f.supported }
func (f *fakeAutostart) Enable() error   { f.enableCalls++; return f.enableErr }
func (f *fakeAutostart) Disable() error  { f.disableCalls++; return nil }

func TestAutostartSupportedInCaps(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	svc.deps.Autostart = &fakeAutostart{supported: true}
	if !svc.GetState().Caps.AutostartSupported {
		t.Error("Caps.AutostartSupported should be true with a supported manager")
	}

	svc2, _, _, _ := testDeps(t)
	// Autostart dep left nil.
	if svc2.GetState().Caps.AutostartSupported {
		t.Error("Caps.AutostartSupported should be false when dep is nil")
	}
}

func TestReconcileAutostartRefreshesWhenEnabled(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	fa.enableCalls = 0 // ignore the enable from the toggle above
	svc.ReconcileAutostart()
	if fa.enableCalls != 1 {
		t.Errorf("ReconcileAutostart enableCalls = %d, want 1", fa.enableCalls)
	}
}

func TestReconcileAutostartSkipsWhenDisabled(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	// AutoStart defaults false.
	svc.ReconcileAutostart()
	if fa.enableCalls != 0 {
		t.Errorf("ReconcileAutostart should not enable when disabled; calls = %d", fa.enableCalls)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/app/ -run 'Autostart' -v`
Expected: FAIL — `svc.deps.Autostart undefined` / `Caps.AutostartSupported undefined` (compile error).

- [ ] **Step 3: Add the interface, Deps field, and Caps field**

In `internal/app/types.go`, add the interface just above `Deps`:

```go
// AutostartManager registers the app to launch at user login. Satisfied by
// *autostart.Manager values; nil is treated as unsupported.
type AutostartManager interface {
	Supported() bool
	Enable() error
	Disable() error
}
```

Add a field inside the `Deps` struct (after `TUNSupported`):

```go
	// Autostart registers launch-on-login (autostart.New()). Nil is treated as
	// unsupported, so tests need not set it.
	Autostart AutostartManager
```

Add a field to `CapsDTO` (after `Elevated`):

```go
	AutostartSupported bool `json:"autostartSupported"`
```

- [ ] **Step 4: Create the app autostart helpers**

Create `internal/app/autostart.go`:

```go
package app

import "fmt"

// autostartSupported reports whether launch-on-login is available on this OS.
func (s *Service) autostartSupported() bool {
	return s.deps.Autostart != nil && s.deps.Autostart.Supported()
}

// applyAutostart enables or disables OS launch-on-login. Disabling when
// unsupported is a no-op; enabling when unsupported is an error.
func (s *Service) applyAutostart(enable bool) error {
	if !s.autostartSupported() {
		if enable {
			return fmt.Errorf("autostart not supported on this OS")
		}
		return nil
	}
	if enable {
		return s.deps.Autostart.Enable()
	}
	return s.deps.Autostart.Disable()
}

// ReconcileAutostart refreshes the OS login entry to the current exe path when
// AutoStart is enabled (handles path drift after an update/reinstall).
// Non-fatal: a failure is logged to the bus.
func (s *Service) ReconcileAutostart() {
	s.mu.Lock()
	enabled := s.state.Settings.AutoStart
	s.mu.Unlock()
	if !enabled || !s.autostartSupported() {
		return
	}
	if err := s.deps.Autostart.Enable(); err != nil {
		s.bus.Append("warning: refresh autostart: " + err.Error())
	}
}
```

- [ ] **Step 5: Set the Caps field in snapshot**

In `internal/app/app.go`, the `Caps: CapsDTO{...}` literal inside `snapshot()` gains a line (after `Elevated: s.elevated(),`):

```go
			AutostartSupported:  s.autostartSupported(),
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/app/ -run 'Autostart' -v`
Expected: PASS (TestAutostartSupportedInCaps, TestReconcileAutostartRefreshesWhenEnabled, TestReconcileAutostartSkipsWhenDisabled).

Note: `TestReconcile...RefreshesWhenEnabled` depends on `UpdateSettings` applying the toggle (Task 3). It still compiles and passes here because `UpdateSettings` already persists `AutoStart` even before Task 3 wires the apply — the test only checks that `ReconcileAutostart` calls `Enable` afterward. If it fails for a delta-apply reason, proceed to Task 3 and re-run.

- [ ] **Step 7: Commit**

```bash
git add internal/app/types.go internal/app/autostart.go internal/app/app.go internal/app/autostart_test.go
git commit -m "feat(app): autostart Deps/caps + applyAutostart/ReconcileAutostart helpers"
```

---

## Task 3: app — UpdateSettings delta apply

**Files:**
- Modify: `internal/app/settings.go` (`UpdateSettings` ~line 27)
- Append: `internal/app/autostart_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/app/autostart_test.go`:

```go
func TestUpdateSettingsEnablesAutostartOnDelta(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if fa.enableCalls != 1 || fa.disableCalls != 0 {
		t.Errorf("enable=%d disable=%d, want 1/0", fa.enableCalls, fa.disableCalls)
	}
}

func TestUpdateSettingsDisablesAutostartOnDelta(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: false}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if fa.disableCalls != 1 {
		t.Errorf("disableCalls = %d, want 1", fa.disableCalls)
	}
}

func TestUpdateSettingsNoAutostartCallWithoutDelta(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	// AutoStart stays false (default) → no apply.
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: false}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if fa.enableCalls != 0 || fa.disableCalls != 0 {
		t.Errorf("enable=%d disable=%d, want 0/0", fa.enableCalls, fa.disableCalls)
	}
}

func TestUpdateSettingsAutostartErrorNotPersisted(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true, enableErr: fmt.Errorf("boom")}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err == nil {
		t.Fatal("expected error from failed Enable")
	}
	// In-memory unchanged.
	if svc.GetState().Settings.AutoStart {
		t.Error("AutoStart should not be set when Enable failed")
	}
	// Not persisted: a fresh service loads AutoStart=false.
	svc2, _ := New(svc.deps)
	if svc2.GetState().Settings.AutoStart {
		t.Error("AutoStart should not have been persisted")
	}
}
```

Add `"fmt"` to the test file's imports (the error test uses `fmt.Errorf`):

```go
import (
	"fmt"
	"testing"
)
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/app/ -run 'UpdateSettings.*Autostart|AutostartError' -v`
Expected: FAIL — `TestUpdateSettingsEnablesAutostartOnDelta` reports enable=0 (no apply wired yet), `TestUpdateSettingsAutostartErrorNotPersisted` fails (error not returned, flag persisted).

- [ ] **Step 3: Wire the delta apply in UpdateSettings**

In `internal/app/settings.go`, modify `UpdateSettings` to apply the autostart change before committing the new settings. The body becomes:

```go
// UpdateSettings replaces app settings after validating the capture mode. An
// AutoStart change is applied to the OS before the new settings are committed,
// so a failure leaves state unchanged.
func (s *Service) UpdateSettings(in SettingsDTO) error {
	mode := store.Mode(in.Mode)
	if mode != store.ModeProxy && mode != store.ModeTUN {
		return fmt.Errorf("invalid mode %q", in.Mode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.AutoStart != s.state.Settings.AutoStart {
		if err := s.applyAutostart(in.AutoStart); err != nil {
			return err // state not modified; frontend reverts the toggle
		}
	}
	s.state.Settings = store.Settings{
		Mode:        mode,
		AutoConnect: in.AutoConnect,
		AutoStart:   in.AutoStart,
		KillSwitch:  in.KillSwitch,
		Mux:         in.Mux,
		LogLevel:    store.NormalizeLogLevel(in.LogLevel),
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}
```

(No import change needed — `fmt` and `store` are already imported in settings.go.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -v`
Expected: PASS — all autostart tests plus the existing suite (no regressions).

- [ ] **Step 5: Commit**

```bash
git add internal/app/settings.go internal/app/autostart_test.go
git commit -m "feat(app): apply autostart on AutoStart toggle delta (before persist)"
```

---

## Task 4: gui_app wiring

**Files:**
- Modify: `gui_app.go` (Deps literal ~line 96-106; `startup` ~line 115-119)

No unit test (GUI glue); must compile.

- [ ] **Step 1: Add the import + Deps field**

In `gui_app.go`, add to the import block:

```go
	"github.com/zki/vless-client/internal/autostart"
```

In the `app.New(app.Deps{...})` literal, add:

```go
		Autostart: autostart.New(),
```

- [ ] **Step 2: Call ReconcileAutostart on startup**

In `startup`, immediately after the `if !a.svc.ResumePendingConnect() { a.svc.MaybeAutoConnect() }` block, add:

```go
	a.svc.ReconcileAutostart()
```

- [ ] **Step 3: Verify it compiles (Linux dev tag + Windows)**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success.
Run: `GOOS=windows GOARCH=amd64 go build -tags wails ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add gui_app.go
git commit -m "feat(gui): wire autostart.New() + ReconcileAutostart on startup"
```

---

## Task 5: Frontend — bindings, types, toggles

**Files:**
- Modify: `frontend/wailsjs/go/models.ts` (CapsDTO ~line 3-22)
- Modify: `frontend/src/main.ts` (Caps type ~line 34-40; `render` ~line 128-134; `wire` handlers near kill/mux ~line 220)
- Modify: `frontend/index.html` (settings section ~line 58)
- Modify: `frontend/src/style.css` (add hide rule for the autostart row)

NOTE: `.hidden` in this codebase is defined only per-element (`.tab-panel.hidden`,
`.modal-overlay.hidden`, `.update-banner.hidden`) — there is no generic
`.hidden` rule. The autostart row needs its own rule (Step 3b).

- [ ] **Step 1: Add the field to the generated model**

In `frontend/wailsjs/go/models.ts`, `CapsDTO`: add `autostartSupported: boolean;` after `elevated: boolean;` (line 8) and `this.autostartSupported = source["autostartSupported"];` after line 20.

- [ ] **Step 2: Add the field to the main.ts Caps type**

In `frontend/src/main.ts`, the `Caps` type gains a field after `elevated: boolean;`:

```ts
  autostartSupported: boolean;
```

- [ ] **Step 3: Add the checkbox rows to index.html**

In `frontend/index.html`, after the mux-toggle label (line 58), insert:

```html

        <h2>Запуск</h2>
        <label id="autostart-row"><input type="checkbox" id="autostart-toggle" /> Автозапуск при входе в систему</label>
        <label><input type="checkbox" id="autoconnect-toggle" /> Автоподключение при старте</label>
```

- [ ] **Step 3b: Add the hide rule to style.css**

In `frontend/src/style.css`, near the other `.hidden` rules (~line 101), add:

```css
#autostart-row.hidden { display: none; }
```

- [ ] **Step 4: Populate the toggles in render()**

In `frontend/src/main.ts` `render()`, after the mux line (`(<HTMLInputElement>$("mux-toggle")).checked = st.settings.mux;`), add:

```ts
  (<HTMLInputElement>$("autostart-toggle")).checked = st.settings.autoStart;
  (<HTMLInputElement>$("autoconnect-toggle")).checked = st.settings.autoConnect;
  $("autostart-row").classList.toggle("hidden", !st.caps.autostartSupported);
```

- [ ] **Step 5: Add the change handlers**

In `frontend/src/main.ts` `wire()`, after the `mux-toggle` handler (ends ~line 210), add:

```ts
  $("autostart-toggle").addEventListener("change", () => {
    const cb = <HTMLInputElement>$("autostart-toggle");
    const prev = current.settings.autoStart;
    current.settings.autoStart = cb.checked;
    UpdateSettings(current.settings).catch((e) => {
      current.settings.autoStart = prev;
      cb.checked = prev;
      $("error-line").textContent = String(e);
    });
  });
  $("autoconnect-toggle").addEventListener("change", () => {
    const cb = <HTMLInputElement>$("autoconnect-toggle");
    const prev = current.settings.autoConnect;
    current.settings.autoConnect = cb.checked;
    UpdateSettings(current.settings).catch((e) => {
      current.settings.autoConnect = prev;
      cb.checked = prev;
      $("error-line").textContent = String(e);
    });
  });
```

(`UpdateSettings` is already imported — `pushSettings` uses it.)

- [ ] **Step 6: Build the frontend**

Run: `cd frontend && npm run build`
Expected: success, no TypeScript errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/wailsjs/go/models.ts frontend/src/main.ts frontend/index.html frontend/src/style.css
git commit -m "feat(frontend): autostart + autoconnect Settings toggles"
```

---

## Task 6: Full verification

- [ ] **Step 1: Go tests**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 2: Linux wails build**

Run: `wails build -tags "wails webkit2_41"`
Expected: success.

- [ ] **Step 3: Windows + macOS cross-compile**

Run: `GOOS=windows GOARCH=amd64 go build -tags wails ./... && GOOS=darwin GOARCH=amd64 go build -tags wails ./...`
Expected: both succeed.

- [ ] **Step 4: Frontend build**

Run: `cd frontend && npm run build`
Expected: success.

- [ ] **Step 5: Manual QA notes (deferred — no Win/macOS host)**

- Windows: toggle "Автозапуск при входе" → check `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` has value `KUB Connect` = quoted exe path; untoggle → value gone. Reboot/relogin → app launches.
- macOS: toggle → `~/Library/LaunchAgents/pro.qb2b.kub-connect.plist` exists + `launchctl list | grep pro.qb2b`; untoggle → file gone.
- "Автоподключение при старте" → with a server selected, app connects on launch.
- Autostart row hidden on Linux (unsupported).
- TUN + autostart + autoconnect (Windows): expect a UAC prompt at login (documented caveat).

---

## Self-Review Notes

- **Spec coverage:** package + per-OS (Task 1) ✓; Deps/interface/caps/helpers (Task 2) ✓; UpdateSettings delta apply before persist (Task 3) ✓; gui wiring + reconcile (Task 4) ✓; caps gating + both toggles + revert-on-error (Task 5) ✓; verification + manual QA (Task 6) ✓.
- **Naming consistency:** `AutostartManager` (app interface), `autostart.Manager` (pkg interface, structurally compatible), `Autostart` (Deps field), `autostartSupported`/`applyAutostart`/`ReconcileAutostart` (methods), `AutostartSupported`/`autostartSupported` (DTO/JSON), `autostart-toggle`/`autoconnect-toggle`/`autostart-row` (DOM ids) used consistently.
- **No placeholders:** every step has concrete code/commands.
- **Note:** `.hidden` CSS class is already used in the codebase (elevate-modal/close-modal); reused for `autostart-row`.
