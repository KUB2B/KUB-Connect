# TUN Elevation on Connect + Auto-Connect After Restart — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pressing Connect in TUN mode while unelevated offers the restart-with-admin modal; after the elevated restart the app auto-connects once.

**Architecture:** A one-shot `PendingConnect` flag on the persisted `State` carries the connect intent across the elevation restart. `gui_app.RelaunchElevated` sets+persists it (clearing on UAC failure); startup consumes it before normal auto-connect. The frontend power button opens the existing elevate modal instead of erroring when TUN+unelevated.

**Tech Stack:** Go (internal/store, internal/app, gui_app.go), Wails v2 bindings, vanilla TypeScript frontend.

**Spec:** `docs/superpowers/specs/2026-06-10-tun-elevation-on-connect-design.md`

---

## File Structure

- `internal/store/store.go` — add `PendingConnect` field to `State`.
- `internal/store/store_test.go` — round-trip + default tests.
- `internal/app/connect.go` — add `SetPendingConnect` + `ResumePendingConnect` methods (lives with `Connect`/`MaybeAutoConnect` neighbors).
- `internal/app/resume_test.go` — new test file for the two methods.
- `gui_app.go` — `RelaunchElevated(connectAfter bool)` + startup wiring.
- `frontend/src/main.ts` — power-button guard + context-aware modal.
- `frontend/wailsjs/go/main/App.{d.ts,js}` — regenerated bindings.

---

## Task 1: `PendingConnect` field on `State`

**Files:**
- Modify: `internal/store/store.go` (State struct ~line 60)
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/store/store_test.go`:

```go
func TestSaveLoadRoundTripPendingConnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	in := &State{
		ActiveServer:   -1,
		Profile:        routing.Default(),
		Settings:       Settings{Mode: ModeTUN, LogLevel: LogNormal},
		PendingConnect: true,
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !out.PendingConnect {
		t.Error("PendingConnect should survive round-trip")
	}
}

func TestDefaultStatePendingConnectFalse(t *testing.T) {
	if DefaultState().PendingConnect {
		t.Error("DefaultState PendingConnect should be false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'PendingConnect' -v`
Expected: FAIL — `in.PendingConnect undefined (type *State has no field PendingConnect)` (compile error).

- [ ] **Step 3: Add the field**

In `internal/store/store.go`, the `State` struct becomes:

```go
// State is the full persisted application state.
type State struct {
	Servers      []*vless.ServerConfig `json:"servers"`
	ActiveServer int                   `json:"activeServer"`
	Profile      routing.Profile       `json:"profile"`
	Settings     Settings              `json:"settings"`
	// PendingConnect is a one-shot intent set before an elevated restart so the
	// new instance auto-connects once, regardless of the AutoConnect setting.
	// Cleared after it is consumed on startup.
	PendingConnect bool `json:"pendingConnect"`
}
```

`DefaultState` is unchanged (zero value is `false`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'PendingConnect' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): add one-shot PendingConnect flag to State"
```

---

## Task 2: `SetPendingConnect` + `ResumePendingConnect` on Service

**Files:**
- Modify: `internal/app/connect.go` (append methods near `MaybeAutoConnect`)
- Test: `internal/app/resume_test.go` (create)

Note on the test harness (from `internal/app/app_test.go` / `connect_test.go`):
`testDepsElevation(t, elevated bool)` returns `(*Service, *fakeEmitter, *fakeConnector, *ConnConfig)`. `mustAdd(t, svc, sampleLink)` adds a server. Tests are in package `app`, so `svc.state` is directly accessible.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/resume_test.go`:

```go
package app

import "testing"

func TestResumePendingConnectConnectsAndClears(t *testing.T) {
	svc, _, fc, _ := testDepsElevation(t, true)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	svc.SetPendingConnect(true)

	if !svc.ResumePendingConnect() {
		t.Fatal("ResumePendingConnect should report it ran")
	}
	if !fc.started {
		t.Error("ResumePendingConnect should have started the connector")
	}
	if svc.state.PendingConnect {
		t.Error("flag should be cleared in memory")
	}
}

func TestResumePendingConnectSkipsWhenUnset(t *testing.T) {
	svc, _, fc, _ := testDepsElevation(t, true)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	// PendingConnect defaults false.
	if svc.ResumePendingConnect() {
		t.Error("ResumePendingConnect should report false when unset")
	}
	if fc.started {
		t.Error("connector should not start when flag unset")
	}
}

func TestSetPendingConnectWritesField(t *testing.T) {
	svc, _, _, _ := testDepsElevation(t, true)
	svc.SetPendingConnect(true)
	if !svc.state.PendingConnect {
		t.Error("SetPendingConnect(true) should set the field")
	}
	svc.SetPendingConnect(false)
	if svc.state.PendingConnect {
		t.Error("SetPendingConnect(false) should clear the field")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run 'PendingConnect' -v`
Expected: FAIL — `svc.SetPendingConnect undefined` (compile error).

- [ ] **Step 3: Implement the methods**

Append to `internal/app/connect.go`:

```go
// SetPendingConnect sets the one-shot connect-after-restart intent. Called by
// the GUI before an elevated restart so the new instance connects once.
func (s *Service) SetPendingConnect(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.PendingConnect = v
}

// ResumePendingConnect connects once if a pending-connect intent was persisted
// (set before an elevated restart). The flag is cleared on disk regardless of
// the connection outcome. Returns true if the intent was present (so the caller
// can skip the normal AutoConnect path). A connection failure is non-fatal:
// it is recorded in state/logs via Connect.
func (s *Service) ResumePendingConnect() bool {
	s.mu.Lock()
	pending := s.state.PendingConnect
	if pending {
		s.state.PendingConnect = false
		if err := s.persist(); err != nil {
			s.bus.Append("error: clear pending-connect: " + err.Error())
		}
	}
	s.mu.Unlock()

	if !pending {
		return false
	}
	_ = s.Connect()
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -run 'PendingConnect' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/app/connect.go internal/app/resume_test.go
git commit -m "feat(app): SetPendingConnect + ResumePendingConnect for elevation restart"
```

---

## Task 3: Wire `RelaunchElevated(connectAfter)` + startup consume

**Files:**
- Modify: `gui_app.go` (`RelaunchElevated` ~line 188; `startup` ~line 115)

This task changes a bound method signature; bindings are regenerated in Task 4.
No Go unit test (GUI glue); covered by the app-level tests in Task 2 and manual QA.

- [ ] **Step 1: Update `RelaunchElevated`**

Replace the `RelaunchElevated` method in `gui_app.go` with:

```go
// RelaunchElevated persists state, launches an elevated instance of the app, and
// on success quits this (unprivileged) one via quit (so the close is not vetoed).
// Bound to the frontend; called when the user opts to restart for TUN mode.
// When connectAfter is true a one-shot intent is persisted so the elevated
// instance auto-connects once on startup. Returns the error (e.g.
// privilege.ErrElevationDeclined) so the frontend can revert.
func (a *App) RelaunchElevated(connectAfter bool) error {
	if a.svc != nil {
		a.svc.SetPendingConnect(connectAfter)
		if err := a.svc.Persist(); err != nil {
			log.Printf("persist before elevate: %v", err)
		}
	}
	if err := privilege.RelaunchElevated(); err != nil {
		if a.svc != nil {
			// Relaunch failed (e.g. UAC declined); this process keeps running, so
			// clear the intent to avoid a spurious auto-connect on the next start.
			a.svc.SetPendingConnect(false)
			_ = a.svc.Persist()
		}
		return err
	}
	a.quit()
	return nil
}
```

- [ ] **Step 2: Update startup to consume the flag first**

In `gui_app.go` `startup`, replace the line `a.svc.MaybeAutoConnect()` (~line 115) with:

```go
	// A pending-connect intent (set before an elevated restart) takes priority
	// over the normal AutoConnect setting; consume it once, else auto-connect.
	if !a.svc.ResumePendingConnect() {
		a.svc.MaybeAutoConnect()
	}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build -tags "wails webkit2_41" ./...`
Expected: success (no output). The Go side compiles; the stale JS bindings still
call `RelaunchElevated()` with no arg but that is JS, regenerated next task.

- [ ] **Step 4: Commit**

```bash
git add gui_app.go
git commit -m "feat(gui): RelaunchElevated(connectAfter) + consume pending-connect on startup"
```

---

## Task 4: Regenerate Wails bindings

**Files:**
- Modify: `frontend/wailsjs/go/main/App.d.ts`, `frontend/wailsjs/go/main/App.js`

- [ ] **Step 1: Regenerate bindings**

Run: `wails generate module` (from repo root). If that is unavailable, a full
`wails build -tags wails` also regenerates `frontend/wailsjs/`.

- [ ] **Step 2: Verify the signature changed**

Run: `grep -n RelaunchElevated frontend/wailsjs/go/main/App.d.ts`
Expected: `export function RelaunchElevated(arg1:boolean):Promise<void>;`

- [ ] **Step 3: Commit**

```bash
git add frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js
git commit -m "chore(frontend): regenerate bindings for RelaunchElevated(connectAfter)"
```

---

## Task 5: Frontend — power-button guard + context-aware modal

**Files:**
- Modify: `frontend/src/main.ts` (power button ~161; mode-select ~194; elevate modal ~243-263)

- [ ] **Step 1: Add the context flag**

Near the top of the `setup`/init scope where other module-local state lives (above the power-button handler, ~line 159), add:

```ts
  // Distinguishes how the elevate modal was opened: from the power button
  // (connect intent — auto-connect after restart, cancel leaves TUN as-is) vs
  // from the mode dropdown (cancel reverts to proxy).
  let elevateForConnect = false;
```

- [ ] **Step 2: Guard the power button**

Replace the power-button handler (lines 161-169) with:

```ts
  // Power button: toggle based on current state.
  $("power-btn").addEventListener("click", () => {
    const c = current?.conn;
    if (c === "connected") {
      Disconnect().catch((e) => ($("error-line").textContent = String(e)));
    } else if (c === "disconnected" || c === "error") {
      // TUN needs admin. If unelevated, offer restart-with-admin instead of a
      // doomed Connect that the backend would reject.
      if (
        current.settings.mode === "tun" &&
        current.caps.tunSupported &&
        !current.caps.elevated
      ) {
        elevateForConnect = true;
        $("elevate-modal").classList.remove("hidden");
        return;
      }
      Connect().catch((e) => ($("error-line").textContent = String(e)));
    }
    // connecting/disconnecting: ignore.
  });
```

- [ ] **Step 3: Mark the mode-select path as non-connect**

In the `mode-select` change handler (lines 194-202), set the flag false when opening the modal. Replace the handler with:

```ts
  $("mode-select").addEventListener("change", () => {
    const val = (<HTMLSelectElement>$("mode-select")).value;
    current.settings.mode = val;
    pushSettings();
    // TUN needs admin. If we are not elevated, offer a restart-with-admin.
    if (val === "tun" && current.caps.tunSupported && !current.caps.elevated) {
      elevateForConnect = false;
      $("elevate-modal").classList.remove("hidden");
    }
  });
```

- [ ] **Step 4: Make modal close context-aware + pass the flag**

Replace the elevate-modal block (lines 243-263) with:

```ts
  // Elevate (restart-with-admin) modal.
  const elevateModal = $("elevate-modal");
  // Close the modal. When opened from the mode dropdown (revert=true) we fall
  // back to proxy; when opened from the power button we leave the mode as TUN.
  const closeElevate = (revert: boolean) => {
    elevateModal.classList.add("hidden");
    if (revert) {
      const sel = <HTMLSelectElement>$("mode-select");
      sel.value = "proxy";
      current.settings.mode = "proxy";
      pushSettings();
    }
    elevateForConnect = false;
  };
  $("elevate-restart").addEventListener("click", () => {
    RelaunchElevated(elevateForConnect).catch((e) => {
      closeElevate(!elevateForConnect);
      $("error-line").textContent = String(e);
    });
  });
  $("elevate-cancel").addEventListener("click", () => closeElevate(!elevateForConnect));
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !elevateModal.classList.contains("hidden")) {
      closeElevate(!elevateForConnect);
    }
  });
```

Rationale for `closeElevate(!elevateForConnect)`: power-button entry
(`elevateForConnect=true`) → `revert=false` (keep TUN); mode-select entry
(`elevateForConnect=false`) → `revert=true` (back to proxy), preserving the
original behavior.

- [ ] **Step 5: Type-check / build the frontend**

Run: `cd frontend && npm run build`
Expected: build succeeds, no TypeScript errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/main.ts
git commit -m "feat(frontend): offer elevate modal from power button in TUN mode"
```

---

## Task 6: Full verification

- [ ] **Step 1: Run all Go tests**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 2: Wails build (Linux dev tag)**

Run: `wails build -tags "wails webkit2_41"`
Expected: build succeeds.

- [ ] **Step 3: Windows cross-compile sanity**

Run: `GOOS=windows GOARCH=amd64 go build -tags wails ./...`
Expected: success (no output).

- [ ] **Step 4: Frontend build**

Run: `cd frontend && npm run build`
Expected: success.

- [ ] **Step 5: Note manual QA (Windows, deferred — no display on Linux host)**

Verify on a Windows host:
- Persisted `mode=tun`, unelevated, press power → elevate modal appears (not an error).
- Confirm restart → UAC → app relaunches elevated and auto-connects once.
- Cancel on the modal (power entry) → modal closes, mode stays TUN, no connect.
- Decline UAC → frontend shows error, mode unchanged; next manual start does NOT auto-connect (flag cleared).
- Mode-dropdown → TUN while unelevated → cancel still reverts to proxy (unchanged behavior).

---

## Self-Review Notes

- **Spec coverage:** State flag (Task 1) ✓; app methods (Task 2) ✓;
  RelaunchElevated signature + failure-clear + startup priority (Task 3) ✓;
  bindings regen (Task 4) ✓; power-button guard + context-aware cancel (Task 5) ✓;
  tests store+app ✓, manual QA listed (Task 6) ✓.
- **Naming consistency:** `PendingConnect` (Go field), `pendingConnect` (JSON),
  `SetPendingConnect`/`ResumePendingConnect` (methods), `elevateForConnect` (TS),
  `closeElevate` (TS) used consistently across tasks.
- **No placeholders:** all steps contain concrete code/commands.
