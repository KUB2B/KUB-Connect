# Close-to-tray, background operation, and Russian installer

Date: 2026-06-09
Status: approved (pending spec review)

## Goal

Three independent improvements to the VLESS client GUI (Windows + macOS, Wails v2):

1. Localize the Windows NSIS installer to Russian.
2. On window close, prompt the user: minimize to tray, quit, or cancel.
3. Keep the app running in the background when minimized, reachable via a system tray icon.

## Non-goals

- No "remember my choice" persistence — the close dialog asks every time.
- No single-instance enforcement (the tray icon is the way back to the window).
- No new connection features; the tray only triggers existing Connect/Disconnect.

## 1. Russian installer

`build/windows/installer/project.nsi` is committed and is **not** overwritten by
`wails build` (only `wails_tools.nsh` is regenerated), so edits persist.

Change: `build/windows/installer/project.nsi:67`

```
!insertmacro MUI_LANGUAGE "English"   ->   !insertmacro MUI_LANGUAGE "Russian"
```

MUI2 ships built-in Russian translations for all standard pages used here
(Welcome, Directory, InstFiles, Finish, Uninstall) and the `MUI_ABORTWARNING`
prompt. Product/company names stay as configured in `wails.json`.

Verification: build the installer with `makensis` and confirm pages render in
Russian. (Requires `nsis` installed; cross-build path is `make windows`.)

## 2. Close dialog

A native `runtime.MessageDialog` is **not** used: on Windows its custom button
labels are unreliable, so a Russian three-way choice cannot be guaranteed.
Instead use an HTML modal in the frontend, giving full control over labels and
styling on every platform.

### Flow

1. Wails `OnBeforeClose(ctx) bool` hook fires when the user clicks the window
   close button.
2. The hook **always returns `true`** (cancels the native close) and emits a
   `close-requested` event to the frontend.
3. The frontend shows a modal with three buttons:
   - **Свернуть в трей** -> calls bound `HideToTray()` -> `runtime.WindowHide(ctx)`.
   - **Выйти** -> calls bound `QuitApp()` -> `runtime.Quit(ctx)`.
   - **Отмена** -> closes the modal, no backend call.
4. `runtime.Quit` triggers the existing `OnShutdown` handler -> `svc.Disconnect()`
   -> system-proxy cleanup (already implemented). So quitting from the dialog
   tears the connection down cleanly.

### Components

- `gui_app.go`:
  - `beforeClose(ctx) bool` — emits `close-requested`, returns `true`.
  - `HideToTray()` — `wruntime.WindowHide(a.ctx)`.
  - `QuitApp()` — `wruntime.Quit(a.ctx)`.
- `main.go`: wire `OnBeforeClose: a.beforeClose`.
- `frontend/index.html`: modal markup (hidden by default).
- `frontend/src/main.ts`: `EventsOn("close-requested", ...)` shows modal; button
  handlers call `HideToTray` / `QuitApp` / hide modal.
- `frontend/src/style.css`: modal overlay styling.
- Regenerate `frontend/wailsjs/go/main/App.*` bindings after adding methods.

## 3. System tray (background)

Wails v2 core has no system tray. Use `github.com/energye/systray` (the
Wails-v2-compatible fork).

### Integration

- Start the tray in `App.startup` after the service is built.
- Tray lifecycle runs alongside the Wails event loop. On Windows the tray can run
  from a goroutine; on macOS the tray must cooperate with the main thread.
  **Planning task: confirm the exact energye/systray + Wails v2 start pattern
  (`systray.Run` vs `systray.Register`/external-loop) on both OSes before
  implementing.** This is the main technical risk.
- Tray icon bytes embedded from `build/windows/icon.ico` via `go:embed`
  (macOS may need a PNG/template variant — resolve in planning).

### Menu

- **Показать** — `wruntime.WindowShow(ctx)` + `WindowUnminimise`.
- **Подключить / Отключить** — single item whose label and action track the
  connection state. Calls `svc.Connect()` / `svc.Disconnect()`. Label updates
  when the connection state changes (subscribe to the same state the GUI uses,
  or update on a `state` event).
- **Выход** — `wruntime.Quit(ctx)`.
- Left-click on the tray icon — show the window.

### State sync

The tray menu label for Connect/Disconnect must reflect `Service` connection
state. Reuse the existing event/log subscription mechanism (`SubscribeLogs` is
log-only; a connection-state hook may be needed). Planning task: pick the
cleanest way to push conn-state changes to the tray (e.g. a small callback the
service invokes on `setConn`, mirroring `SubscribeLogs`).

### Build constraints

- Tray code is GUI-only: `//go:build wails`.
- energye/systray pulls CGO + platform libs (already required by Wails GUI).
- The headless CLI (`cmd/headless`, no CGO) is unaffected.

## Testing

- Unit tests stay at the `Service`/`tunnel` layer (already cover Connect/
  Disconnect/proxy-clear). Tray + dialog are GUI wiring in `package main`
  (`//go:build wails`, CGO+webkit) — not unit-testable without a Wails harness.
- Manual QA on Windows (cross-built): close -> dialog -> minimize/quit/cancel;
  tray show/connect/disconnect/quit; verify proxy cleared on quit.
- macOS QA deferred to a Mac build.

## Risks

1. **energye/systray macOS main-thread integration** — highest risk; verify the
   start pattern in planning before coding.
2. **macOS tray icon format** — `.ico` may not suffice; may need a template PNG.
3. Cannot build/test macOS on this Linux host — macOS verification is deferred.
