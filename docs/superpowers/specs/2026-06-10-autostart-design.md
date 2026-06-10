# Autostart (launch on login) — Design

Date: 2026-06-10
Status: approved
Phase: 5 (#2 of 3 — autostart → ping/test → traffic stats)

## Problem

`Settings.AutoStart` exists in the store/DTO but has no OS implementation and no
UI toggle. `Settings.AutoConnect` likewise has no UI toggle, so it is currently
unreachable from the GUI. Users cannot make the app launch (or connect) on login.

## Goal

Register/unregister the app to launch at user login, per-OS, without requiring
admin to register. Add the missing Settings UI for both `AutoStart` (launch on
login) and `AutoConnect` (connect on launch) — they compose: autostart +
autoconnect = "start the VPN on login".

## Mechanism (per OS)

- **Windows** (`golang.org/x/sys/windows/registry`, already a dependency): a
  string value `KUB Connect` under
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` set to the quoted absolute
  exe path. Per-user, no admin. Disable deletes the value (idempotent).
- **macOS**: a LaunchAgent plist at
  `~/Library/LaunchAgents/pro.qb2b.kub-connect.plist` with `Label`,
  `ProgramArguments=[<exe>]`, `RunAtLoad=true`. Applied immediately with
  `launchctl unload -w` (ignore error) then `launchctl load -w <plist>`
  (best-effort — if launchctl fails the file still takes effect at next login).
  Disable runs `launchctl unload -w` then removes the file (ignore not-exist).
  Fixed label `pro.qb2b.kub-connect` (reverse of the qb2b.pro domain) — does not
  depend on a bundle ID, which the build does not currently set.
- **Linux/other**: stub — `Supported()` false, `Enable` errors, `Disable` no-op.

**Elevation interplay (known caveat, documented not solved):** HKCU Run launches
the app **unelevated**. If the persisted mode is TUN with AutoConnect on, the
restart-with-admin flow (shipped 2026-06-10) fires at login → a UAC prompt every
login. Acceptable for this MVP. The friction-free path (Task Scheduler with
highest privileges, or the helper-service) is future work.

## Architecture

New package `internal/autostart`, per-OS split mirroring `internal/privilege`:

- `autostart.go` (untagged): the `Manager` interface + pure builders (so they are
  unit-testable on any OS).

  ```go
  // Manager controls launch-on-login registration. Implementations are per-OS.
  type Manager interface {
      Supported() bool
      Enable() error  // resolves its own exe path via os.Executable
      Disable() error
  }

  // plistContent builds a macOS LaunchAgent plist for the given label and exe.
  func plistContent(label, execPath string) string

  // runValue builds the Windows Run registry value (quoted exe path).
  func runValue(execPath string) string
  ```

- `autostart_windows.go` (`//go:build windows`): `winManager` using
  `registry.CreateKey`/`SetStringValue`/`DeleteValue`. `Disable` treats a missing
  key/value as success (`registry.ErrNotExist` and open-failure → nil).
- `autostart_darwin.go` (`//go:build darwin`): `macManager` writing the plist
  (mkdir `~/Library/LaunchAgents`, 0644) + launchctl calls.
- `autostart_other.go` (`//go:build !windows && !darwin`): `noopManager`.
- Each OS file exposes `func New() Manager`.

## Settings integration (`internal/app`)

`Deps` gains one field:

```go
// Autostart registers the app to launch on login (autostart.New()). Nil is
// treated as unsupported, so tests need not set it.
Autostart AutostartManager
```

with a local interface (same shape as `autostart.Manager`, redeclared in `app`
to avoid an import cycle concern and to keep the dep mockable):

```go
type AutostartManager interface {
    Supported() bool
    Enable() error
    Disable() error
}
```

Helper:

```go
// autostartSupported reports whether autostart is available on this OS.
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
```

`UpdateSettings` applies the OS change **before** committing the new settings, so
a failure leaves state unchanged:

```go
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

**Startup reconcile** (path drift after update/reinstall) — new method, called
from gui_app startup:

```go
// ReconcileAutostart refreshes the OS login entry to the current exe path when
// AutoStart is enabled. Non-fatal; a failure is logged to the bus.
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

`CapsDTO` gains `AutostartSupported bool`, set from `s.autostartSupported()` in
`snapshot()`.

## gui_app wiring

- `Deps.Autostart = autostart.New()`.
- In `startup`, after service init, call `a.svc.ReconcileAutostart()` (near the
  existing `ResumePendingConnect`/`MaybeAutoConnect` block).

## Frontend

- `Caps` type + `models.ts` gain `autostartSupported`.
- Settings tab (`index.html`): two new checkbox rows near the kill/mux toggles —
  `autostart-toggle` ("Автозапуск при входе") and `autoconnect-toggle`
  ("Автоподключение при старте").
- `render()` populates both checkboxes from `current.settings`; the autostart row
  is hidden (`.hidden`) when `!current.caps.autostartSupported`.
- Handlers mirror the kill/mux pattern, but revert on error:

  ```ts
  $("autostart-toggle").addEventListener("change", () => {
    const cb = <HTMLInputElement>$("autostart-toggle");
    const prev = current.settings.autoStart;
    current.settings.autoStart = cb.checked;
    pushSettings().catch((e) => {
      current.settings.autoStart = prev;
      cb.checked = prev;
      $("error-line").textContent = String(e);
    });
  });
  ```

  (`autoconnect-toggle` is the same without the revert subtlety — but use the
  same revert form for consistency.) This requires `pushSettings` to return the
  `UpdateSettings` promise; if it currently returns void, change it to return the
  promise.

## Testing

- **`internal/autostart`** (`autostart_test.go`, untagged, runs on Linux):
  `plistContent` contains the label, exe path, and `RunAtLoad`; `runValue`
  quotes the path. OS side effects (registry/launchctl) → manual QA (no Win/mac
  on the dev host).
- **`internal/app`** (fake `AutostartManager` with call counters + configurable
  `Supported`/error):
  - `UpdateSettings` calls `Enable` once when AutoStart goes false→true.
  - calls `Disable` once when true→false.
  - no call when AutoStart unchanged.
  - `Enable` error → `UpdateSettings` returns error and does NOT persist the flag
    (reload via `store.Load` shows AutoStart still false).
  - `CapsDTO.AutostartSupported` true with a supported fake, false when dep nil.
  - `ReconcileAutostart` calls `Enable` when enabled+supported, skips otherwise.
- **Frontend:** manual QA on Windows/macOS.

## Verification

`go test ./...`, `wails build -tags "wails webkit2_41"`,
`GOOS=windows GOARCH=amd64 go build -tags wails ./...`,
`cd frontend && npm run build`.

## Out of scope

- Task Scheduler (highest-privileges, silent elevated login) and the
  helper-service path — future. TUN+autostart = UAC-per-login is the documented
  caveat.
- Re-applying other settings (LogLevel etc.) to a running session — unchanged.
