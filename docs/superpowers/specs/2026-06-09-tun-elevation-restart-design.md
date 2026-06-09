# TUN elevation: restart-with-admin on demand (Windows)

Date: 2026-06-09
Status: approved (pending spec review)

## Goal

Stop requiring the user to launch the whole app as Administrator to use TUN mode.
The app starts unprivileged; when the user selects TUN mode without elevation, it
offers to restart itself elevated (one UAC prompt per session). Proxy mode stays
admin-free.

## Scope

- **Windows only.** TUN is supported on Windows (end users) and Linux (dev, run
  via `sudo` manually); macOS TUN is unimplemented. The restart-elevated path is
  implemented for Windows; Linux/macOS get a stub that returns "not supported"
  (the existing Connect-time guard still applies there).
- No privileged helper service (that is the larger future "prod path", out of
  scope here — see the no-admin-tun-options memory).

## Non-goals

- No auto-connect after the elevated restart (the app lands in TUN mode; the user
  presses Connect — or AutoConnect handles it if enabled).
- No persistence of "I dismissed this prompt" — the prompt shows whenever TUN is
  selected while unprivileged.

## Background (verified)

- `internal/privilege` already has `IsElevated()` (Windows: token.IsElevated;
  unix: euid==0) and `RequireElevated(purpose)`.
- `internal/app/connect.go:88` already blocks Connect in TUN mode when
  `!s.deps.Elevated()` with an error. This stays as the safety net.
- `app.Deps.Elevated func() bool` is wired in `gui_app.go` from
  `privilege.IsElevated`. `CapsDTO` currently exposes os/version/tunSupported/
  killSwitchSupported.
- Wails routes `runtime.Quit` through `OnBeforeClose`. The app already uses an
  atomic `quitting` flag (App.quit) so intentional quits aren't vetoed. The
  elevated-restart MUST quit the old instance via `App.quit` (sets the flag),
  otherwise the old instance vetoes its own close and two processes coexist.
- `golang.org/x/sys v0.45.0` is a dependency (has `golang.org/x/sys/windows`).

## Design

### 1. CapsDTO gains `elevated`

`internal/app/types.go` `CapsDTO` adds `Elevated bool` (json `elevated`).
`internal/app/app.go` `snapshot()` populates it from a new helper
`s.elevated()` that returns `s.deps.Elevated == nil || s.deps.Elevated()` (nil =
elevated, so unit tests need not set it — mirrors `killSwitchSupported`).

The frontend `Caps` type gains `elevated: boolean`.

### 2. privilege.RelaunchElevated

New per-OS function in `internal/privilege`:

```go
// RelaunchElevated starts a new elevated instance of the current executable
// (same args) via the OS elevation prompt, and returns nil on success. The
// CALLER is responsible for quitting the current (unprivileged) instance after
// a nil return. Returns an error if elevation was declined or is unsupported.
func RelaunchElevated() error
```

- `privilege_windows.go`: uses `ShellExecuteW` (shell32) with verb `runas` on
  `os.Executable()`, passing the current `os.Args[1:]` joined as the parameter
  string, working dir = exe dir, `SW_SHOWNORMAL`. A return value `<= 32` is a
  ShellExecute error; `SE_ERR_ACCESSDENIED`/`ERROR_CANCELLED` (user declined
  UAC) maps to a sentinel `ErrElevationDeclined`. Implemented via
  `windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")`.
- `privilege_other.go` (`//go:build !windows`): returns an error
  "elevation restart is not supported on this OS".

Security: the executable path comes only from `os.Executable()` (never from
user input/config), so there is no path-injection vector. The UAC dialog shows
the binary's signature/path; since the binary is unsigned, UAC will show
"Publisher: Unknown" — expected for this release.

### 3. App.RelaunchElevated (bound method)

`gui_app.go`:

```go
// RelaunchElevated persists state, launches an elevated instance, and on success
// quits this (unprivileged) one. Bound to the frontend; called when the user
// opts to restart for TUN mode.
func (a *App) RelaunchElevated() error {
	if a.svc != nil {
		a.svc.Persist() // ensure mode=TUN is on disk before the new instance loads it
	}
	if err := privilege.RelaunchElevated(); err != nil {
		return err // UAC declined / unsupported — frontend reverts mode to proxy
	}
	a.quit() // sets the quitting flag, then Quit — old instance exits cleanly
	return nil
}
```

`Service` needs an exported `Persist()` (currently `persist()` is unexported and
holds-mu-required). Add `func (s *Service) Persist() error` that locks s.mu and
calls the internal persist. The frontend has already pushed mode=TUN via
`UpdateSettings` before calling RelaunchElevated, so state is current; Persist is
belt-and-suspenders.

### 4. Frontend: elevate modal + mode-select handling

`frontend/index.html`: a second modal `#elevate-modal` (same styling as the
close modal) with title "Режим TUN требует прав администратора", text explaining
a restart with admin rights is needed, and buttons **Перезапустить** (primary) /
**Отмена**.

`frontend/src/main.ts`:
- Import `RelaunchElevated` from the bindings.
- In the existing `mode-select` change handler: when the chosen value is `tun`
  and `current.caps.elevated` is false (and `caps.tunSupported`), show
  `#elevate-modal` instead of (or in addition to) pushing settings. The mode IS
  pushed (so the elevated instance loads TUN), then the modal is shown.
- `#elevate-restart` click → `RelaunchElevated().catch(...)`. On error (UAC
  declined), hide modal, revert the select to `proxy`, push settings, show the
  error line.
- `#elevate-cancel` (and Escape) → hide modal, revert select to `proxy`, push
  settings.

On the elevated instance, `caps.elevated` is true, so selecting TUN never
prompts — no elevation loop.

### 5. Bindings

Regenerate `frontend/wailsjs/go/main/App.*` (with `-tags "wails webkit2_41"`) to
expose `RelaunchElevated`. Restore any 100644→100755 mode churn on runtime files.

## Components / files

- Modify: `internal/app/types.go` (CapsDTO.Elevated), `internal/app/app.go`
  (snapshot + `elevated()` helper + exported `Persist`).
- Create: `internal/privilege/privilege.go` gains `RelaunchElevated` declaration
  doc + `ErrElevationDeclined`; `privilege_windows.go` real impl;
  `privilege_other.go` (`!windows`) stub. (Current unix file is
  `//go:build linux || darwin` for IsElevated; the RelaunchElevated stub should
  cover all non-windows, so put it in a new `privilege_other.go` with
  `//go:build !windows`.)
- Modify: `gui_app.go` (App.RelaunchElevated bound method).
- Modify: `frontend/index.html`, `frontend/src/main.ts` (+style reuse).
- Regenerate: `frontend/wailsjs/go/main/App.*`.

## Error handling

- UAC declined → `RelaunchElevated` returns `ErrElevationDeclined`; frontend
  reverts to proxy and shows a short notice. App keeps running unprivileged.
- Non-Windows → stub error; never reached from the UI because `caps.elevated`
  reflects euid and the prompt only triggers on Windows where TUN+unelevated is
  the real case (on Linux dev the user runs sudo, so elevated=true; on macOS
  tunSupported=false so TUN isn't offered).

## Testing

- `internal/privilege`: unit test that `RelaunchElevated` on the non-windows
  build returns a non-nil error (the stub). The Windows ShellExecute path is not
  unit-testable (spawns UAC) — manual QA.
- `internal/app`: test that `CapsDTO.Elevated` reflects the injected
  `Deps.Elevated` (true when nil, follows the stub when set).
- Compile gates: Linux GUI (`-tags "wails webkit2_41"`) and Windows cross
  (`CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc -tags wails`).
- Manual QA (Windows, deferred): launch unprivileged → select TUN → modal →
  Перезапустить → UAC → app relaunches elevated, old instance gone, TUN
  connects; Отмена/decline → stays on proxy.

## Risks

1. `ShellExecuteW` argument quoting — args with spaces must be quoted in the
   parameter string. This release passes no runtime args, so the parameter
   string is typically empty; quoting is implemented defensively but lightly
   exercised.
2. Cannot test the real UAC/relaunch on this Linux host — Windows QA deferred.
3. Two-instance risk if the old instance fails to quit — mitigated by routing
   the old-instance shutdown through `App.quit` (the quitting flag), consistent
   with the close/quit fix.
