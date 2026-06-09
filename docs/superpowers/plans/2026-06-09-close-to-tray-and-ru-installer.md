# Close-to-tray, background operation, and Russian installer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Localize the Windows installer to Russian, prompt the user on window close (minimize-to-tray / quit / cancel), and keep the app running in the background reachable via a system tray icon with connect/disconnect.

**Architecture:** A Wails `OnBeforeClose` hook cancels the native close and asks the frontend to show an HTML modal. "Minimize" hides the window; "Quit" calls `runtime.Quit` (which runs the existing `OnShutdown` → `Disconnect` → proxy cleanup). A system tray (energye/systray, via `RunWithExternalLoop`) provides Show / Connect-Disconnect / Quit. The tray's connect/disconnect label tracks `Service` connection state through a new `SubscribeConn` fan-out, mirroring the existing log `Subscribe`.

**Tech Stack:** Go 1.26, Wails v2 (v2.12.0), `github.com/energye/systray`, TypeScript frontend. Build tag `wails`; tray real impl gated to `windows || darwin`, Linux gets a no-op stub (Linux is dev-only; release targets Windows + macOS).

---

## File Structure

- `internal/app/app.go` — Modify: add `connSubs`/`connNextID` fields, init in `New`, add `SubscribeConn`, notify subscribers in a new helper.
- `internal/app/connect.go` — Modify: `setConn` notifies conn subscribers.
- `internal/app/app_test.go` — Modify: add `TestSubscribeConn*` tests (white-box, package `app`).
- `tray.go` — Create: real tray, build tag `wails && (windows || darwin)`.
- `tray_stub.go` — Create: no-op tray, build tag `wails && !windows && !darwin` (Linux dev).
- `gui_app.go` — Modify: start/stop tray, `beforeClose`, `HideToTray`, `QuitApp`, subscribe conn→tray.
- `main.go` — Modify: wire `OnBeforeClose`.
- `frontend/wailsjs/go/main/App.*` — Regenerate (adds `HideToTray`, `QuitApp`).
- `frontend/index.html` — Modify: close-choice modal markup.
- `frontend/src/style.css` — Modify: modal styling.
- `frontend/src/main.ts` — Modify: `close-requested` handler + modal button wiring.
- `build/windows/installer/project.nsi` — Modify: Russian MUI language.

## Verification gates used throughout

- **Service unit tests:** `go test ./internal/app/...`
- **Linux compile (stub tray):** `go build -tags "wails webkit2_41" ./...`
- **Windows cross-compile (real tray):** `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -tags wails ./...`

GUI behavior (tray, modal, installer language) is not unit-testable on this Linux host (CGO + webview2/webkit, no display). Functional checks are manual QA in Task 7 on Windows; macOS QA is deferred to a Mac.

---

### Task 1: `Service.SubscribeConn` connection-state fan-out

Adds a way for the tray to learn the current connection state and every change, mirroring the existing log `Subscribe`.

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/connect.go:69-73` (`setConn`)
- Test: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/app/app_test.go`:

```go
func TestSubscribeConnDeliversCurrentStateImmediately(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	var got []ConnState
	cancel := svc.SubscribeConn(func(c ConnState) { got = append(got, c) })
	defer cancel()
	if len(got) != 1 || got[0] != ConnDisconnected {
		t.Fatalf("want [disconnected] on subscribe, got %v", got)
	}
}

func TestSubscribeConnDeliversChanges(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	var got []ConnState
	cancel := svc.SubscribeConn(func(c ConnState) { got = append(got, c) })
	defer cancel()
	svc.mu.Lock()
	svc.setConn(ConnConnecting, "")
	svc.setConn(ConnConnected, "")
	svc.mu.Unlock()
	want := []ConnState{ConnDisconnected, ConnConnecting, ConnConnected}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at %d want %s got %s", i, want[i], got[i])
		}
	}
}

func TestSubscribeConnCancelStopsDelivery(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	var n int
	cancel := svc.SubscribeConn(func(ConnState) { n++ })
	cancel()
	svc.mu.Lock()
	svc.setConn(ConnConnecting, "")
	svc.mu.Unlock()
	if n != 1 { // only the immediate on-subscribe delivery
		t.Fatalf("want 1 delivery before cancel, got %d", n)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run TestSubscribeConn -v`
Expected: FAIL — `svc.SubscribeConn undefined`.

- [ ] **Step 3: Add fields and init**

In `internal/app/app.go`, add to the `Service` struct (after `tailStop`):

```go
	connSubs   map[int]func(ConnState) // conn-state fan-out (tray)
	connNextID int
```

In `New`, set the field in the returned `&Service{...}` literal (add after `conn: ConnDisconnected,`):

```go
		connSubs: map[int]func(ConnState){},
```

- [ ] **Step 4: Add `SubscribeConn` and notify helper**

In `internal/app/app.go`, add:

```go
// SubscribeConn registers fn to receive connection-state changes. fn is called
// once immediately with the current state, then on every subsequent change. The
// returned function unsubscribes. fn runs while s.mu is held during change
// delivery, so it must not block or call back into the Service.
func (s *Service) SubscribeConn(fn func(ConnState)) (cancel func()) {
	s.mu.Lock()
	id := s.connNextID
	s.connNextID++
	s.connSubs[id] = fn
	cur := s.conn
	s.mu.Unlock()
	fn(cur)
	return func() {
		s.mu.Lock()
		delete(s.connSubs, id)
		s.mu.Unlock()
	}
}

// notifyConn delivers the current state to conn subscribers. Caller must hold s.mu.
func (s *Service) notifyConn() {
	for _, fn := range s.connSubs {
		fn(s.conn)
	}
}
```

- [ ] **Step 5: Call `notifyConn` from `setConn`**

In `internal/app/connect.go`, `setConn` currently is:

```go
func (s *Service) setConn(c ConnState, errMsg string) {
	s.conn = c
	s.lastError = errMsg
	s.emitState()
}
```

Change to:

```go
func (s *Service) setConn(c ConnState, errMsg string) {
	s.conn = c
	s.lastError = errMsg
	s.emitState()
	s.notifyConn()
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/app/ -run TestSubscribeConn -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Run the full app suite (no regressions)**

Run: `go test ./internal/app/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/app/app.go internal/app/connect.go internal/app/app_test.go
git commit -m "feat(app): add SubscribeConn connection-state fan-out for tray"
```

---

### Task 2: Russian NSIS installer

`project.nsi` is committed and is not regenerated by `wails build` (only `wails_tools.nsh` is), so this edit persists across builds. MUI2 ships the Russian translation for all standard pages used here.

**Files:**
- Modify: `build/windows/installer/project.nsi:67`

- [ ] **Step 1: Switch the MUI language**

In `build/windows/installer/project.nsi`, change line 67 from:

```
!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer
```

to:

```
!insertmacro MUI_LANGUAGE "Russian" # Set the Language of the installer
```

- [ ] **Step 2: Sanity-check (no MUI_LANGUAGE duplication / typo)**

Run: `grep -n "MUI_LANGUAGE" build/windows/installer/project.nsi`
Expected: exactly one line, showing `"Russian"`.

- [ ] **Step 3: Commit**

```bash
git add build/windows/installer/project.nsi
git commit -m "feat(installer): localize NSIS installer to Russian"
```

> Functional check (installer pages render in Russian) happens in Task 7 manual QA via `make windows` (requires `nsis`).

---

### Task 3: System tray backend (real impl + Linux stub)

Adds the tray as a small package-`main` unit with a stable internal API, so `gui_app.go` wires callbacks without touching energye/systray directly. Real impl builds only on Windows/macOS; Linux gets a no-op stub so the dev build (`wails dev` on Linux) and the Linux compile gate don't pull systray's C dependencies.

**Files:**
- Create: `tray.go` (`//go:build wails && (windows || darwin)`)
- Create: `tray_stub.go` (`//go:build wails && !windows && !darwin`)
- Modify: `go.mod` / `go.sum` (add dependency)

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/energye/systray@v1.0.3`
Expected: `go.mod` gains `github.com/energye/systray v1.0.3`.

- [ ] **Step 2: Create the real tray implementation**

Create `tray.go`:

```go
//go:build wails && (windows || darwin)

package main

import (
	"sync"

	"github.com/energye/systray"
)

// trayCallbacks are the actions the tray triggers. All run on the tray's own
// thread; each implementation hops back to the Wails runtime / Service as needed.
type trayCallbacks struct {
	onShow       func() // bring the window to the foreground
	onConnect    func() // start the connection
	onDisconnect func() // stop the connection
	onQuit       func() // quit the whole app
}

var (
	trayMu        sync.Mutex
	trayToggle    *systray.MenuItem // single Connect/Disconnect item
	trayConnected bool              // last known connection state
	trayCB        trayCallbacks
	trayEnd       func() // systray external-loop teardown
)

// startTray launches the system tray using the external-loop entry point so it
// coexists with the Wails main loop. icon is the platform-appropriate image
// bytes (.ico on Windows, .png on macOS). Returns a stop function for shutdown.
func startTray(icon []byte, cb trayCallbacks) (stop func()) {
	trayMu.Lock()
	trayCB = cb
	trayMu.Unlock()

	onReady := func() {
		systray.SetIcon(icon)
		systray.SetTitle("VLESS Client")
		systray.SetTooltip("VLESS Client")
		// Left-click shows the window.
		systray.SetOnClick(func(systray.IMenu) { trayCB.onShow() })

		mShow := systray.AddMenuItem("Показать", "Показать окно")
		mShow.Click(func() { trayCB.onShow() })

		trayMu.Lock()
		trayToggle = systray.AddMenuItem("Подключить", "Подключить / отключить")
		trayMu.Unlock()
		trayToggle.Click(onToggleClicked)

		mQuit := systray.AddMenuItem("Выход", "Выйти из приложения")
		mQuit.Click(func() { trayCB.onQuit() })

		// Reflect any state that arrived before the menu existed.
		trayMu.Lock()
		connected := trayConnected
		trayMu.Unlock()
		updateTrayConn(connected)
	}

	start, end := systray.RunWithExternalLoop(onReady, func() {})
	trayMu.Lock()
	trayEnd = end
	trayMu.Unlock()
	start()
	return func() {
		trayMu.Lock()
		e := trayEnd
		trayMu.Unlock()
		if e != nil {
			e()
		}
	}
}

// onToggleClicked dispatches the single toggle item based on last known state.
func onToggleClicked() {
	trayMu.Lock()
	connected := trayConnected
	cb := trayCB
	trayMu.Unlock()
	if connected {
		cb.onDisconnect()
	} else {
		cb.onConnect()
	}
}

// updateTrayConn updates the toggle item's label to match connection state.
// Safe to call before the menu is built (state is stored and applied in onReady).
func updateTrayConn(connected bool) {
	trayMu.Lock()
	trayConnected = connected
	item := trayToggle
	trayMu.Unlock()
	if item == nil {
		return
	}
	if connected {
		item.SetTitle("Отключить")
	} else {
		item.SetTitle("Подключить")
	}
}
```

> Note: `tray.go` does not import `runtime` — the platform icon choice lives in `gui_app.go::trayIcon()`. Keep this file free of platform branching.

- [ ] **Step 3: Create the Linux stub**

Create `tray_stub.go`:

```go
//go:build wails && !windows && !darwin

package main

// trayCallbacks mirrors the real implementation so gui_app.go compiles on Linux.
type trayCallbacks struct {
	onShow       func()
	onConnect    func()
	onDisconnect func()
	onQuit       func()
}

// startTray is a no-op on Linux (dev builds only; release targets Windows/macOS).
func startTray(icon []byte, cb trayCallbacks) (stop func()) { return func() {} }

// updateTrayConn is a no-op on Linux.
func updateTrayConn(connected bool) {}
```

- [ ] **Step 4: Linux compile gate (stub path)**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success (compiles the stub; no systray C deps pulled).

- [ ] **Step 5: Windows cross-compile gate (real path)**

Run: `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -tags wails ./...`
Expected: success (real `tray.go` compiles against the Windows systray backend).

> If this fails on a missing symbol/import, fix `tray.go` before proceeding — do NOT continue with a broken tray. The likely culprit is an API drift in energye/systray (re-check signatures with `go doc github.com/energye/systray`).

- [ ] **Step 6: Commit**

```bash
git add tray.go tray_stub.go go.mod go.sum
git commit -m "feat(gui): add system tray backend (energye/systray) with Linux stub"
```

---

### Task 4: Wire tray, close-prompt, and lifecycle into the Wails app

Connects Task 1 (conn fan-out) and Task 3 (tray) into the app, adds the close prompt, and the bound `HideToTray`/`QuitApp` methods the frontend calls.

**Files:**
- Modify: `gui_app.go`
- Modify: `main.go`

- [ ] **Step 1: Embed tray icons in `gui_app.go`**

At the top of `gui_app.go`, after the existing imports, add an embed block. Insert these embed directives just below the import group (package-level):

```go
// trayIconICO / trayIconPNG are the tray images. Windows uses the .ico; macOS
// uses the .png (energye/systray renders PNG on darwin).
//
//go:embed build/windows/icon.ico
var trayIconICO []byte

//go:embed build/appicon.png
var trayIconPNG []byte
```

Add `"embed"` to the import list in `gui_app.go` (it is not currently imported there).

- [ ] **Step 2: Add a tray-icon selector helper in `gui_app.go`**

```go
// trayIcon returns the platform-appropriate tray image bytes.
func trayIcon() []byte {
	if runtime.GOOS == "darwin" {
		return trayIconPNG
	}
	return trayIconICO
}
```

(`runtime` is already imported in `gui_app.go`.)

- [ ] **Step 3: Add lifecycle + bound methods in `gui_app.go`**

Add a `trayStop` field to the `App` struct:

```go
type App struct {
	ctx      context.Context
	svc      *app.Service
	trayStop func() // tears down the tray on shutdown
}
```

At the end of `startup` (after `a.svc.MaybeAutoConnect()`), add tray startup and conn subscription:

```go
	// Tray: show window, toggle connection, quit. Subscribe to connection
	// state so the toggle label stays in sync (fires once immediately).
	a.svc.SubscribeConn(func(c app.ConnState) {
		updateTrayConn(c == app.ConnConnected)
	})
	a.trayStop = startTray(trayIcon(), trayCallbacks{
		onShow: func() {
			wruntime.WindowShow(a.ctx)
			wruntime.WindowUnminimise(a.ctx)
		},
		onConnect:    func() { _ = a.svc.Connect() },
		onDisconnect: func() { _ = a.svc.Disconnect() },
		onQuit:       func() { wruntime.Quit(a.ctx) },
	})
```

Replace the existing `shutdown` method body so it also stops the tray:

```go
func (a *App) shutdown(ctx context.Context) {
	if a.trayStop != nil {
		a.trayStop()
	}
	if a.svc == nil {
		return
	}
	if err := a.svc.Disconnect(); err != nil {
		log.Printf("shutdown disconnect: %v", err)
	}
}
```

Add the close-prompt hook and the two bound methods (place after `shutdown`):

```go
// beforeClose runs when the user clicks the window close button. It always
// cancels the native close (returns true) and asks the frontend to show the
// minimize-or-quit choice. The frontend then calls HideToTray or QuitApp.
func (a *App) beforeClose(ctx context.Context) (preventClose bool) {
	wruntime.EventsEmit(a.ctx, "close-requested")
	return true
}

// HideToTray hides the window; the app keeps running and is reachable via the
// tray. Bound to the frontend.
func (a *App) HideToTray() { wruntime.WindowHide(a.ctx) }

// QuitApp quits the whole app (runs shutdown → Disconnect → proxy cleanup).
// Bound to the frontend.
func (a *App) QuitApp() { wruntime.Quit(a.ctx) }
```

- [ ] **Step 4: Wire `OnBeforeClose` in `main.go`**

In `main.go`, the options literal currently has:

```go
		OnStartup:  a.startup,
		OnShutdown: a.shutdown,
		Bind:       []any{a},
```

Change to:

```go
		OnStartup:    a.startup,
		OnShutdown:   a.shutdown,
		OnBeforeClose: a.beforeClose,
		Bind:         []any{a},
```

- [ ] **Step 5: Linux compile gate**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success.

- [ ] **Step 6: Windows cross-compile gate**

Run: `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -tags wails ./...`
Expected: success.

- [ ] **Step 7: Commit**

```bash
git add gui_app.go main.go
git commit -m "feat(gui): close-to-tray prompt, tray actions, and lifecycle wiring"
```

---

### Task 5: Regenerate Wails bindings

Exposes `HideToTray` and `QuitApp` to the frontend.

**Files:**
- Regenerate: `frontend/wailsjs/go/main/App.js`, `App.d.ts`

- [ ] **Step 1: Generate the module**

Run: `wails generate module`
Expected: regenerates `frontend/wailsjs/go/...`.

- [ ] **Step 2: Confirm the new bindings exist**

Run: `grep -E "HideToTray|QuitApp" frontend/wailsjs/go/main/App.d.ts`
Expected: both function declarations present.

- [ ] **Step 3: Fix any spurious file-mode changes**

`wails generate module` has previously flipped runtime files to mode 755. Restore:

Run: `git diff --stat` — if any `runtime/*.js`/`.d.ts` show only a mode change (`100644 → 100755`), run `chmod 644` on them.
Expected: only content changes remain (the App.* binding additions).

- [ ] **Step 4: Commit**

```bash
git add frontend/wailsjs
git commit -m "chore(frontend): regenerate wails bindings (HideToTray, QuitApp)"
```

---

### Task 6: Close-choice modal in the frontend

The modal shown when `close-requested` fires.

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/style.css`
- Modify: `frontend/src/main.ts`

- [ ] **Step 1: Add modal markup**

In `frontend/index.html`, immediately before `<script type="module" src="/src/main.ts"></script>` (after the closing `</div>` of `#app`), add:

```html
    <div id="close-modal" class="modal-overlay hidden">
      <div class="modal">
        <p class="modal-title">Закрыть приложение?</p>
        <p class="modal-text">Свернуть в трей — приложение продолжит работать в фоне.</p>
        <div class="modal-actions">
          <button id="modal-hide" class="modal-btn primary">Свернуть в трей</button>
          <button id="modal-quit" class="modal-btn danger">Выйти</button>
          <button id="modal-cancel" class="modal-btn">Отмена</button>
        </div>
      </div>
    </div>
```

- [ ] **Step 2: Add modal styling**

Append to `frontend/src/style.css`:

```css
/* Close-choice modal */
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,.6);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.modal-overlay.hidden { display: none; }
.modal {
  background: var(--surface); border: 1px solid var(--border); border-radius: 10px;
  padding: 1.4rem; width: 320px; max-width: 90vw; box-shadow: 0 12px 40px rgba(0,0,0,.5);
}
.modal-title { font-size: 1.05rem; margin: 0 0 0.5rem; }
.modal-text { font-size: 0.85rem; color: var(--muted); margin: 0 0 1.2rem; }
.modal-actions { display: flex; flex-direction: column; gap: 0.5rem; }
.modal-btn { width: 100%; padding: 0.55rem; }
.modal-btn.primary { background: var(--accent-dim); border-color: var(--accent-dim); color: #fff; }
.modal-btn.primary:hover { background: var(--accent); }
.modal-btn.danger { color: var(--error); }
```

- [ ] **Step 3: Wire the modal in `main.ts`**

In `frontend/src/main.ts`, add `HideToTray` and `QuitApp` to the import from `../wailsjs/go/main/App`:

```ts
import {
  GetState,
  AddServer,
  RemoveServer,
  SetActiveServer,
  UpdateProfile,
  UpdateSettings,
  Connect,
  Disconnect,
  Logs,
  HideToTray,
  QuitApp,
} from "../wailsjs/go/main/App";
```

Inside `wire()`, before the `EventsOn("state", ...)` line, add modal wiring:

```ts
  // Close-choice modal.
  const closeModal = $("close-modal");
  $("modal-hide").addEventListener("click", () => {
    closeModal.classList.add("hidden");
    HideToTray();
  });
  $("modal-quit").addEventListener("click", () => {
    QuitApp();
  });
  $("modal-cancel").addEventListener("click", () => {
    closeModal.classList.add("hidden");
  });
  EventsOn("close-requested", () => {
    closeModal.classList.remove("hidden");
  });
```

- [ ] **Step 4: Build the frontend**

Run: `cd frontend && npm run build`
Expected: TypeScript compiles, `dist/` updated, no errors. (Then `cd ..`.)

- [ ] **Step 5: Full GUI build (embeds the new dist)**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html frontend/src/style.css frontend/src/main.ts frontend/dist
git commit -m "feat(frontend): add close-to-tray choice modal"
```

---

### Task 7: Manual QA + release artifact (manual, cannot be automated on Linux)

GUI behavior requires running on the target OS. This task is a checklist, not automated steps.

**Files:** none (verification only)

- [ ] **Step 1: Build the Windows installer**

Run: `make windows` (requires `nsis`: `sudo apt install nsis` if missing).
Expected: `build/release/vless-client-<VER>-windows-amd64-installer.exe`.

- [ ] **Step 2: Windows install + language QA**

On a Windows machine, run the installer. Confirm: all installer pages (Welcome, install dir, progress, finish, the abort-warning prompt) are in Russian.

- [ ] **Step 3: Windows runtime QA (proxy mode)**

- Launch app, connect in proxy mode, confirm browsing works.
- Click the window close (X): the modal appears with «Свернуть в трей / Выйти / Отмена».
  - «Отмена» → modal closes, window stays.
  - «Свернуть в трей» → window hides; tray icon present.
- From the tray: left-click → window reappears; menu shows «Показать / Отключить (when connected) / Выход».
- Tray «Отключить» → disconnects (label flips to «Подключить»); «Подключить» → reconnects.
- Tray «Выход» (while connected) → app exits AND the system proxy is cleared (open Internet Options → Connections → LAN settings: proxy unchecked; browser works without ERR_PROXY_CONNECTION_FAILED).

- [ ] **Step 4: macOS QA (deferred to a Mac)**

On a Mac build (`make macos`), repeat Step 3. Pay attention to the documented risks: tray on macOS main-thread behavior and the PNG tray icon rendering. If the tray icon click-with-menu doesn't fire on macOS, fall back to relying on the menu's «Показать» item.

- [ ] **Step 5: Update CHANGELOG**

Add entries under the unreleased/next section of `CHANGELOG.md`:

```
- Русский установщик (Windows).
- При закрытии окна — выбор: свернуть в трей или выйти.
- Работа в фоне: иконка в системном трее с пунктами Показать / Подключить-Отключить / Выход.
```

Commit: `git add CHANGELOG.md && git commit -m "docs: changelog for tray + ru installer"`

---

## Self-Review notes

- **Spec coverage:** RU installer → Task 2; close prompt (Свернуть/Выйти/Отмена) → Tasks 4+6; background tray with Подключить/Отключить → Tasks 3+4; proxy cleanup on quit reuses the existing `OnShutdown` path (Task 4 routes Quit through it). All spec sections covered.
- **Risks carried from spec:** energye/systray macOS main-thread (Task 3 uses `RunWithExternalLoop`, the documented external-loop entry; verified at compile only — runtime check is Task 7 Step 4); macOS icon format (Task 4 selects PNG on darwin); no Mac on this host (Task 7 Step 4 deferred).
- **Type consistency:** `trayCallbacks` fields (`onShow/onConnect/onDisconnect/onQuit`), `startTray(icon []byte, cb trayCallbacks) (stop func())`, and `updateTrayConn(connected bool)` are identical across `tray.go` and `tray_stub.go`. `SubscribeConn(func(ConnState)) (cancel func())` matches its caller in Task 4. Bound methods `HideToTray`/`QuitApp` match the frontend import (Task 6) and the regenerated bindings (Task 5).
