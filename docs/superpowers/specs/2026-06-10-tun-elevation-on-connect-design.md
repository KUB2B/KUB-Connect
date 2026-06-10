# TUN elevation on Connect + auto-connect after restart

Date: 2026-06-10
Status: approved

## Problem

The restart-with-admin flow (shipped 2026-06-09, commit `d680ef9` lineage)
triggers the elevate modal **only** when the user changes the mode dropdown to
TUN (`frontend/src/main.ts:194`). When `mode=tun` is already persisted from a
prior session and the user simply presses the power button:

- backend `Service.Connect()` returns the error `"TUN mode requires
  administrator/root privileges"` (`internal/app/connect.go:95`);
- the frontend shows that raw error in the error line — no restart prompt.

The user is stuck: the app remembers TUN but offers no way to elevate without
re-toggling the dropdown.

## Goal

Pressing Connect while in TUN mode and unelevated must offer the same
restart-with-admin modal. After the user confirms and the app relaunches
elevated, it must auto-connect **once** — regardless of the `AutoConnect`
setting — honoring the intent that triggered the restart.

## Flow

1. **Power-button click**: if `mode === 'tun' && caps.tunSupported &&
   !caps.elevated` → show the existing elevate modal; do **not** call
   `Connect()`.
2. **Restart confirm**: `RelaunchElevated(connectAfter=true)` — backend sets a
   one-shot `PendingConnect` flag, persists state, relaunches elevated, the old
   (unprivileged) instance quits.
3. **Elevated startup**: the new instance sees `PendingConnect`, connects once,
   clears the flag. This is independent of the `AutoConnect` user setting.

The existing mode-select trigger is unchanged; the power-button check is
additive.

## Backend

### `internal/store/store.go`

Add a top-level field to `State` (not `Settings` — this is transient app state,
not a user toggle):

```go
type State struct {
    Servers        []*vless.ServerConfig `json:"servers"`
    ActiveServer   int                   `json:"activeServer"`
    Profile        routing.Profile       `json:"profile"`
    Settings       Settings              `json:"settings"`
    PendingConnect bool                  `json:"pendingConnect"`
}
```

`DefaultState` leaves it `false` (zero value).

### `internal/app`

Two methods on `Service`:

- `SetPendingConnect(v bool)` — acquires `s.mu`, sets `s.state.PendingConnect`.
- `ResumePendingConnect() bool` — under `s.mu`: read the flag, set it to
  `false`, and if it was set, call `s.persist()` to clear it on disk; release
  `s.mu`; if it was set, call `s.Connect()` (which takes its own lock) and
  return `true`; otherwise return `false`.

`Connect()` already guards against double-connect (`connect.go:88`), so a
spurious overlap with auto-connect is harmless.

### `gui_app.go`

`RelaunchElevated` gains a parameter and clears the flag on failure:

```go
func (a *App) RelaunchElevated(connectAfter bool) error {
    if a.svc != nil {
        a.svc.SetPendingConnect(connectAfter)
        if err := a.svc.Persist(); err != nil {
            log.Printf("persist before elevate: %v", err)
        }
    }
    if err := privilege.RelaunchElevated(); err != nil {
        if a.svc != nil {
            a.svc.SetPendingConnect(false)
            _ = a.svc.Persist() // avoid a spurious auto-connect on next manual start
        }
        return err
    }
    a.quit()
    return nil
}
```

`startup()` (~line 115) gives the pending intent priority over normal
auto-connect:

```go
if !a.svc.ResumePendingConnect() {
    a.svc.MaybeAutoConnect()
}
```

Regenerate the Wails bindings (`RelaunchElevated` signature changed).

## Frontend (`frontend/src/main.ts`)

- New module-scoped flag `elevateForConnect = false` distinguishes the two ways
  the modal opens.
- Power-button handler (line 161): before calling `Connect()`, if
  `current.settings.mode === 'tun' && current.caps.tunSupported &&
  !current.caps.elevated` → set `elevateForConnect = true`, show the modal,
  return.
- Mode-select handler (line 194): unchanged behavior; ensure
  `elevateForConnect = false` for that path.
- `elevate-restart` click (line 252): call `RelaunchElevated(elevateForConnect)`.
- Cancel / Escape: if `elevateForConnect` (power-button entry) → just close the
  modal (mode stays TUN, no connect). Otherwise the existing `revertToProxy`
  (mode-select entry). Reset `elevateForConnect = false` on close.

## Testing

- **store**: `PendingConnect` survives a Save/Load round-trip; `DefaultState`
  has it `false`.
- **app**: `ResumePendingConnect` connects and clears the flag when set; returns
  `false` and does not connect when unset; `SetPendingConnect` writes the field.
- **frontend**: manual QA on Windows (no display on the Linux host) — persisted
  TUN + power → modal → restart → auto-connects once; cancel leaves TUN
  disconnected; declined UAC reverts cleanly with no later spurious connect.

## Out of scope

- Linux/macOS: `tunSupported` is false there (caps-gating), so the power-button
  guard never fires — this is a Windows-only path. The Unix `RelaunchElevated`
  stub is untouched.
- No change to the `AutoConnect` or `AutoStart` settings (the latter is Phase 5,
  separate spec).
