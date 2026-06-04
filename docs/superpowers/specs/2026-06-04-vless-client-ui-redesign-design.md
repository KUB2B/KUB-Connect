# VLESS Client — UI redesign + configurable log level

Date: 2026-06-04
Status: approved (design)

## Goal

Make the desktop GUI simpler and more attractive, translate it to Russian, and
move all configuration onto a dedicated tab so the home screen shows little more
than a connect button. Add a user-facing setting for the log verbosity level.

## Scope

In scope:

- Three in-page tabs: **Главная**, **Настройки**, **Логи**.
- Dark theme with a green accent, driven by CSS variables.
- Home screen reduced to a status-colored connect/disconnect button, a status
  label, an active-server selector, and an error line.
- All controls (servers, mode, routing toggles, kill switch, mux, log level)
  moved to the Настройки tab.
- Logs moved to their own tab.
- A 3-level log verbosity setting (Тихо / Обычный / Подробно) wired to both
  xray and tun2socks.
- Full Russian UI text.

Out of scope (YAGNI):

- i18n / multi-language framework — Russian is hardcoded.
- Frontend test suite (none exists today; keep it that way).
- Light theme / theme switching (variables make it possible later, not now).
- Any change to connection/routing behavior beyond log verbosity.

## Architecture

### Frontend (vanilla TS, no framework)

The current single-page `index.html` + `main.ts` stays vanilla TS to match the
existing codebase. Tabs are implemented as show/hide of `<section>` blocks driven
by a small tab bar; no router or framework is introduced.

- **Tab bar**: three buttons (`Главная`, `Настройки`, `Логи`). Clicking sets an
  `active` class on the bar button and toggles a `hidden` attribute/class on the
  corresponding section. Default active tab: Главная.
- **Theme**: a `<style>`/`style.css` using CSS custom properties on `:root`
  (`--bg`, `--surface`, `--text`, `--muted`, `--accent`, `--accent-dim`,
  `--warn`, `--border`). Dark values, green accent.
- **System font stack**, no web fonts.

### Главная tab

- **Connect button**: a large round button whose color reflects connection
  state, derived from `state.conn`:
  - disconnected / error → grey (`--muted`)
  - connecting / disconnecting → yellow, pulsing (`--warn`)
  - connected → green (`--accent`)
  - Click behavior: if disconnected/error → `Connect()`; if connected →
    `Disconnect()`; ignored (no-op) while connecting/disconnecting.
  - Contains a power glyph (⏻ or an inline SVG; final icon TBD during impl, not
    a design blocker).
- **Status label** below the button: Russian text mapped from `state.conn`
  (`Отключено`, `Подключение…`, `Подключено`, `Отключение…`, `Ошибка`).
- **Active-server selector**: a `<select>` listing servers by name; changing it
  calls `SetActiveServer(index)`. Disabled/placeholder when no servers exist
  (prompts the user to add one in Настройки).
- **Error line**: shows `state.lastError` when present.

### Настройки tab

Grouped controls:

1. **Серверы** — list of servers (each row: name + host:port, a "выбрать"
   action, a "удалить" action), plus a `vless://` input and "Добавить" button.
   Mirrors current behavior; only styling/labels change.
2. **Режим** — Proxy / TUN selector.
3. **Маршрутизация / опции** — checkboxes: `Telegram → VPN`, `RU напрямую`,
   `Kill switch (TUN)`, `Mux`.
4. **Уровень логов** — a `<select>` with Тихо / Обычный / Подробно.

### Логи tab

A `<pre>` log view bound to the `log` event stream, plus an "Очистить" button.
Same data source as today.

## Log level feature

### Storage

`store.Settings` gains `LogLevel string` (JSON `logLevel`). Stored values are
xray-native to keep mapping trivial: `"error"`, `"warning"`, `"debug"`.

UI labels map to stored values:

| UI label  | stored value |
|-----------|--------------|
| Тихо      | `error`      |
| Обычный   | `warning`    |
| Подробно  | `debug`      |

Default: `"warning"`. State migration: an empty/missing `LogLevel` on load is
treated as `"warning"` (handled where settings are loaded / in `settingsDTO` and
when building config — see below — so old `state.json` files keep working).

### Plumbing

- `SettingsDTO` gains `LogLevel string`; `settingsDTO()` copies it;
  `UpdateSettings` persists it (validating it is one of the three values, else
  defaults to `warning`).
- `xrayconf.Options` gains `LogLevel string`; `Build` uses it for
  `logConf.LogLevel` instead of the hardcoded `"warning"` (empty → `"warning"`).
- `internal/app` passes `s.state.Settings.LogLevel` into `xrayconf.Options`.
- **tun2socks**: the TUN connector maps the level to the engine log level
  (`engine.Key.LogLevel`) and to whether verbose engine logs are emitted:
  - Тихо (`error`) → engine `error`
  - Обычный (`warning`) → engine `warning`
  - Подробно (`debug`) → engine `debug`
- The always-on `Info` tun.log instrumentation added during the Windows TUN
  debugging is **replaced** by this: tun.log is written at the mapped level, so
  verbose per-connection lines appear only on Подробно. (Addresses the
  release-prep item to lower/gate that instrumentation.)

## Data flow

`render(state)`:
- sets tab visibility (unchanged across renders; tab is local UI state),
- sets connect-button color/label from `state.conn`,
- populates the active-server `<select>` and Настройки server list,
- reflects all checkboxes/selectors including the new log-level select.

`UpdateSettings(settings)` is the single write path for mode, toggles, mux, and
log level; the frontend sends the whole `Settings` object as it does now.

## Error handling

- Connect/Disconnect failures surface in the Главная error line (existing
  pattern).
- Add-server errors surface near the input on the Настройки tab.
- Invalid `LogLevel` from the frontend is coerced to `warning` server-side.

## Testing

Go unit tests:

- `UpdateSettings` persists `LogLevel`; invalid value coerces to `warning`.
- `settingsDTO` round-trips `LogLevel`; empty stored value maps to `warning`.
- `xrayconf.Build` writes `log.loglevel` from `Options.LogLevel`; empty →
  `warning` (extend the existing golden / log-level tests).
- The TUN connector maps each level to the expected `engine.Key.LogLevel`
  (where unit-testable; otherwise a small pure mapping function with its own
  test).

Frontend: no automated tests (consistent with current project); manual smoke
via `wails build` + run.

## Open implementation details (not design blockers)

- Final connect-button icon (glyph vs inline SVG).
- Exact CSS variable values / spacing.
