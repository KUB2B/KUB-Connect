# TUN elevation restart-with-admin — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the app start unprivileged and, when the user selects TUN mode on Windows without admin rights, offer to restart itself elevated (one UAC prompt) instead of requiring the whole app to be launched as Administrator.

**Architecture:** Expose elevation state to the frontend via `CapsDTO.Elevated`. A new `privilege.RelaunchElevated()` spawns an elevated copy of the current exe via `ShellExecuteW`/`runas` (Windows) or returns an error (other OSes). A bound `App.RelaunchElevated` persists state, relaunches, and quits the old instance through the existing `quitting`-flagged `App.quit`. The frontend shows an elevate modal when TUN is selected while unprivileged.

**Tech Stack:** Go 1.26, Wails v2, `golang.org/x/sys/windows` (already a dep), vanilla TypeScript frontend.

---

## File Structure

- `internal/app/types.go` — Modify: `CapsDTO` gains `Elevated bool`.
- `internal/app/app.go` — Modify: `snapshot()` sets `Caps.Elevated` via new `elevated()` helper; add exported `Persist()`.
- `internal/app/connect.go` — Modify: add `elevated()` helper (mirrors `tunSupported`/`killSwitchSupported`).
- `internal/app/app_test.go` — Modify: test `CapsDTO.Elevated` reflects the injected dep.
- `internal/privilege/privilege.go` — Modify: add `ErrElevationDeclined` sentinel + doc for `RelaunchElevated`.
- `internal/privilege/privilege_windows.go` — Modify: add real `RelaunchElevated` (ShellExecuteW runas).
- `internal/privilege/privilege_other.go` — Create (`//go:build !windows`): stub `RelaunchElevated`.
- `internal/privilege/privilege_test.go` — Modify: test the non-windows stub errors.
- `gui_app.go` — Modify: add bound `App.RelaunchElevated`.
- `frontend/index.html` — Modify: add `#elevate-modal`.
- `frontend/src/main.ts` — Modify: import `RelaunchElevated`; mode-select shows modal; modal button wiring.
- `frontend/wailsjs/go/main/App.*` — Regenerate.

## Verification gates

- **Service/privilege unit tests:** `go test ./internal/app/... ./internal/privilege/...`
- **Linux GUI compile:** `go build -tags "wails webkit2_41" ./...`
- **Windows cross-compile:** `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -tags wails ./...`
- Do NOT run `go mod tidy` (strips the windows/darwin-only systray dep on Linux).

GUI runtime behavior (UAC, relaunch, modal) is not testable on this headless Linux host — manual QA on Windows (Task 6).

---

### Task 1: CapsDTO.Elevated + elevated() helper + Service.Persist

**Files:**
- Modify: `internal/app/types.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/connect.go`
- Test: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/app/app_test.go`:

```go
func TestCapsElevatedReflectsDep(t *testing.T) {
	svc, _, _, _ := testDepsElevation(t, false)
	if svc.GetState().Caps.Elevated {
		t.Fatal("want Caps.Elevated=false when dep reports not elevated")
	}
	svc2, _, _, _ := testDepsElevation(t, true)
	if !svc2.GetState().Caps.Elevated {
		t.Fatal("want Caps.Elevated=true when dep reports elevated")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run TestCapsElevatedReflectsDep -v`
Expected: FAIL — `Caps.Elevated` undefined (compile error).

- [ ] **Step 3: Add the field to CapsDTO**

In `internal/app/types.go`, `CapsDTO` currently is:

```go
type CapsDTO struct {
	OS                  string `json:"os"`
	Version             string `json:"version"`
	TUNSupported        bool   `json:"tunSupported"`
	KillSwitchSupported bool   `json:"killSwitchSupported"`
}
```

Add an `Elevated` field:

```go
type CapsDTO struct {
	OS                  string `json:"os"`
	Version             string `json:"version"`
	TUNSupported        bool   `json:"tunSupported"`
	KillSwitchSupported bool   `json:"killSwitchSupported"`
	Elevated            bool   `json:"elevated"`
}
```

- [ ] **Step 4: Add the `elevated()` helper**

In `internal/app/connect.go`, after the `tunSupported()` helper (around line 61), add:

```go
// elevated reports whether the process has admin/root privileges. A nil dep is
// treated as elevated so unit tests need not set it (mirrors tunSupported).
func (s *Service) elevated() bool {
	return s.deps.Elevated == nil || s.deps.Elevated()
}
```

- [ ] **Step 5: Populate Caps.Elevated in snapshot**

In `internal/app/app.go`, `snapshot()` builds the `Caps` struct. Add the `Elevated` field:

```go
		Caps: CapsDTO{
			OS:                  s.deps.OS,
			Version:             s.deps.Version,
			TUNSupported:        s.tunSupported(),
			KillSwitchSupported: s.killSwitchSupported(),
			Elevated:            s.elevated(),
		},
```

- [ ] **Step 6: Add exported Persist**

In `internal/app/app.go`, after the existing unexported `persist()` (which requires the caller to hold s.mu), add:

```go
// Persist writes the current state to disk, acquiring the lock itself. Used by
// the GUI before an elevated restart so the new instance loads the latest state.
func (s *Service) Persist() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persist()
}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/app/ -run TestCapsElevatedReflectsDep -v`
Expected: PASS.

- [ ] **Step 8: Run the full app suite**

Run: `go test ./internal/app/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/app/types.go internal/app/app.go internal/app/connect.go internal/app/app_test.go
git commit -m "feat(app): expose Caps.Elevated and add Service.Persist"
```
End the commit body with:
`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

---

### Task 2: privilege.RelaunchElevated (Windows real + stub)

**Files:**
- Modify: `internal/privilege/privilege.go`
- Modify: `internal/privilege/privilege_windows.go`
- Create: `internal/privilege/privilege_other.go`
- Test: `internal/privilege/privilege_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/privilege/privilege_test.go`:

```go
func TestRelaunchElevatedUnsupportedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has a real ShellExecute implementation")
	}
	if err := RelaunchElevated(); err == nil {
		t.Fatal("want a non-nil error from the non-windows stub")
	}
}
```

Add `"runtime"` to the test file's imports if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/privilege/ -run TestRelaunchElevated -v`
Expected: FAIL — `RelaunchElevated` undefined.

- [ ] **Step 3: Add the sentinel error**

In `internal/privilege/privilege.go`, change the imports from `import "fmt"` to:

```go
import (
	"errors"
	"fmt"
)
```

and add, after the imports:

```go
// ErrElevationDeclined is returned by RelaunchElevated when the user dismisses
// the OS elevation (UAC) prompt.
var ErrElevationDeclined = errors.New("elevation request was declined")
```

- [ ] **Step 4: Add the non-windows stub**

Create `internal/privilege/privilege_other.go`:

```go
//go:build !windows

package privilege

import "errors"

// RelaunchElevated is unsupported off Windows. On Linux dev hosts run the binary
// under sudo; macOS does not support TUN in this build.
func RelaunchElevated() error {
	return errors.New("elevation restart is not supported on this OS")
}
```

- [ ] **Step 5: Add the Windows implementation**

In `internal/privilege/privilege_windows.go`, the current file imports only
`golang.org/x/sys/windows`. Replace the import line with:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)
```

Append to the file:

```go
var (
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

// RelaunchElevated starts a new elevated instance of the current executable
// (same args) via the UAC "runas" verb. The caller quits the current instance
// after a nil return. Returns ErrElevationDeclined if the user dismisses UAC.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))

	var paramsPtr *uint16
	if params := joinArgs(os.Args[1:]); params != "" {
		paramsPtr, _ = syscall.UTF16PtrFromString(params)
	}

	const swShowNormal = 1
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(dir)),
		uintptr(swShowNormal),
	)
	// ShellExecuteW returns a value > 32 on success.
	if ret <= 32 {
		// SE_ERR_ACCESSDENIED (5) and ERROR_CANCELLED (1223) both mean the user
		// declined the UAC prompt.
		if ret == 5 || ret == 1223 {
			return ErrElevationDeclined
		}
		return fmt.Errorf("ShellExecuteW failed (code %d)", ret)
	}
	return nil
}

// joinArgs builds a Windows command-line parameter string, quoting args that
// contain whitespace or quotes. This build passes no runtime args, so the
// result is normally empty; quoting is defensive.
func joinArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			out[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}
```

- [ ] **Step 6: Run the test (non-windows stub passes)**

Run: `go test ./internal/privilege/ -run TestRelaunchElevated -v`
Expected: PASS (stub returns error on this Linux host).

- [ ] **Step 7: Windows cross-compile gate (real impl compiles)**

Run: `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -tags wails ./...`
Expected: success.

> If it fails on `ShellExecuteW`/syscall usage, verify `procShellExecute.Call` arg count (6) and the unsafe pointer conversions against the installed `golang.org/x/sys/windows` API.

- [ ] **Step 8: Full suite + linux build**

Run: `go test ./... && go build -tags "wails webkit2_41" ./...`
Expected: PASS + success.

- [ ] **Step 9: Commit**

```bash
git add internal/privilege/
git commit -m "feat(privilege): add RelaunchElevated (Windows UAC restart) + stub"
```
End the commit body with:
`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

---

### Task 3: App.RelaunchElevated bound method

**Files:**
- Modify: `gui_app.go`

- [ ] **Step 1: Add the bound method**

In `gui_app.go`, after the `QuitApp`/`quit` methods, add:

```go
// RelaunchElevated persists state, launches an elevated instance of the app, and
// on success quits this (unprivileged) one via quit (so the close is not vetoed).
// Bound to the frontend; called when the user opts to restart for TUN mode.
// Returns the error (e.g. privilege.ErrElevationDeclined) so the frontend can
// revert to proxy mode.
func (a *App) RelaunchElevated() error {
	if a.svc != nil {
		if err := a.svc.Persist(); err != nil {
			log.Printf("persist before elevate: %v", err)
		}
	}
	if err := privilege.RelaunchElevated(); err != nil {
		return err
	}
	a.quit()
	return nil
}
```

(`privilege` and `log` are already imported in `gui_app.go`.)

- [ ] **Step 2: Linux compile gate**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success.

- [ ] **Step 3: Windows cross-compile gate**

Run: `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -tags wails ./...`
Expected: success.

- [ ] **Step 4: gofmt check**

Run: `gofmt -l gui_app.go`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add gui_app.go
git commit -m "feat(gui): add RelaunchElevated bound method for TUN admin restart"
```
End the commit body with:
`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

---

### Task 4: Regenerate Wails bindings

**Files:**
- Regenerate: `frontend/wailsjs/go/main/App.js`, `App.d.ts`

- [ ] **Step 1: Generate the module (with build tags)**

Run: `wails generate module -tags "wails webkit2_41"`
Expected: regenerates `frontend/wailsjs/go/...`. (The `-tags` are required so the wails-tagged `App` methods are seen.)

- [ ] **Step 2: Confirm the new binding exists**

Run: `grep -n "RelaunchElevated" frontend/wailsjs/go/main/App.d.ts`
Expected: `export function RelaunchElevated():Promise<void>;`

- [ ] **Step 3: Restore any file-mode churn**

Run: `git diff frontend/wailsjs | grep -E "old mode|new mode" && chmod 644 frontend/wailsjs/runtime/package.json frontend/wailsjs/runtime/runtime.d.ts frontend/wailsjs/runtime/runtime.js || echo "no mode churn"`
Then: `git status --short frontend/wailsjs/`
Expected: only `App.d.ts` and `App.js` show as modified.

- [ ] **Step 4: Commit**

```bash
git add frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js
git commit -m "chore(frontend): regenerate wails bindings (RelaunchElevated)"
```
End the commit body with:
`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

---

### Task 5: Elevate modal in the frontend

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.ts`

- [ ] **Step 1: Add the modal markup**

In `frontend/index.html`, the `#close-modal` div is a sibling of `#app` (added earlier). Immediately AFTER the `#close-modal` closing `</div>` and BEFORE `<script type="module" src="/src/main.ts"></script>`, add:

```html
    <div id="elevate-modal" class="modal-overlay hidden">
      <div class="modal">
        <p class="modal-title">Режим TUN требует прав администратора</p>
        <p class="modal-text">Чтобы включить TUN, приложение нужно перезапустить с правами администратора. Proxy-режим работает без них.</p>
        <div class="modal-actions">
          <button id="elevate-restart" class="modal-btn primary">Перезапустить с правами</button>
          <button id="elevate-cancel" class="modal-btn">Отмена</button>
        </div>
      </div>
    </div>
```

(Reuses the `.modal-overlay`/`.modal`/`.modal-btn` styles already in style.css — no CSS change.)

- [ ] **Step 2: Import RelaunchElevated**

In `frontend/src/main.ts`, add `RelaunchElevated` to the existing import from `../wailsjs/go/main/App` (which already lists GetState, AddServer, …, HideToTray, QuitApp):

```ts
  HideToTray,
  QuitApp,
  RelaunchElevated,
} from "../wailsjs/go/main/App";
```

- [ ] **Step 3: Show the modal when TUN is selected unprivileged**

In `frontend/src/main.ts`, the current mode-select handler in `wire()` is:

```ts
  $("mode-select").addEventListener("change", () => {
    current.settings.mode = (<HTMLSelectElement>$("mode-select")).value;
    pushSettings();
  });
```

Replace it with:

```ts
  $("mode-select").addEventListener("change", () => {
    const val = (<HTMLSelectElement>$("mode-select")).value;
    current.settings.mode = val;
    pushSettings();
    // TUN needs admin. If we are not elevated, offer a restart-with-admin.
    if (val === "tun" && current.caps.tunSupported && !current.caps.elevated) {
      $("elevate-modal").classList.remove("hidden");
    }
  });
```

- [ ] **Step 4: Wire the elevate modal buttons**

In `frontend/src/main.ts`, immediately AFTER the close-modal wiring block (after the `EventsOn("close-requested", …)` / Escape handler and before `EventsOn("state", …)`), add:

```ts
  // Elevate (restart-with-admin) modal.
  const elevateModal = $("elevate-modal");
  const revertToProxy = () => {
    elevateModal.classList.add("hidden");
    const sel = <HTMLSelectElement>$("mode-select");
    sel.value = "proxy";
    current.settings.mode = "proxy";
    pushSettings();
  };
  $("elevate-restart").addEventListener("click", () => {
    RelaunchElevated().catch((e) => {
      revertToProxy();
      $("error-line").textContent = String(e);
    });
  });
  $("elevate-cancel").addEventListener("click", revertToProxy);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !elevateModal.classList.contains("hidden")) {
      revertToProxy();
    }
  });
```

- [ ] **Step 5: Build the frontend**

Run: `cd frontend && npm run build && cd ..`
Expected: TypeScript compiles, no errors.

- [ ] **Step 6: Full GUI build**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success.

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html frontend/src/main.ts
git commit -m "feat(frontend): elevate-for-TUN modal with restart-with-admin"
```
End the commit body with:
`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

---

### Task 6: Manual QA (Windows, deferred — cannot run on Linux host)

**Files:** none (verification only)

- [ ] **Step 1: Build the Windows installer**

Run: `make windows` (requires `nsis`).

- [ ] **Step 2: Unprivileged TUN flow**

On Windows, launch the app normally (NOT as admin):
- Settings → Mode → select **TUN**. The elevate modal appears.
- **Отмена** (or Esc) → modal closes, mode reverts to Proxy.
- Select TUN again → **Перезапустить с правами** → UAC prompt (Publisher: Unknown, expected for unsigned). Accept → the app relaunches; the old window closes (only one instance running); mode is TUN; Connect works.
- Decline UAC → app stays open, mode reverts to Proxy, error line shows the declined message.

- [ ] **Step 3: Already-elevated flow**

Launch as admin → select TUN → NO modal (caps.elevated true) → Connect works directly.

---

## Self-Review notes

- **Spec coverage:** CapsDTO.Elevated → Task 1; RelaunchElevated (win real + stub + sentinel) → Task 2; App.RelaunchElevated using a.quit + Persist → Tasks 1 (Persist) + 3; bindings → Task 4; elevate modal + mode-select + revert-to-proxy + Escape → Task 5; manual QA → Task 6. All spec sections covered.
- **Type consistency:** `Caps.Elevated`/`elevated` used in app (Task 1) and frontend (`current.caps.elevated`, Task 5) match the json tag `elevated`. `RelaunchElevated()` signature (no args, returns error) matches across privilege (Task 2), bound method (Task 3), binding (Task 4), and frontend call (Task 5). `Service.Persist()` defined in Task 1, used in Task 3. `a.quit()` already exists (from the quit-veto fix).
- **Risks from spec carried:** Windows-only (stub elsewhere); UAC unsigned-publisher expected; two-instance avoided by routing old-instance exit through `a.quit()`; real UAC path manual-QA only.
