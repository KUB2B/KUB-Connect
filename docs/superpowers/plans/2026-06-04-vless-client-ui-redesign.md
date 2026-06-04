# UI Redesign + Configurable Log Level Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the Wails GUI into three Russian-language tabs (Главная/Настройки/Логи) with a status-colored connect button, and add a 3-level log verbosity setting wired to xray and tun2socks.

**Architecture:** Backend gains a `LogLevel` field on settings that flows into `xrayconf.Build` (xray `log.loglevel`) and into the tun2socks engine via a package-level setter. The frontend stays vanilla TS: tabs are show/hide of sections, the connect button derives its color/label from `state.conn`.

**Tech Stack:** Go, Wails v2, vanilla TypeScript, xray-core, xjasonlyu/tun2socks, zap.

---

## File Structure

Backend:
- `internal/store/store.go` — add `Settings.LogLevel`, `NormalizeLogLevel`, level constants, default + load migration.
- `internal/store/store_test.go` — tests for normalize + migration.
- `internal/app/types.go` — `SettingsDTO.LogLevel`, `settingsDTO` copy, `ConnConfig.LogLevel`.
- `internal/app/settings.go` — persist + normalize `LogLevel` in `UpdateSettings`.
- `internal/app/settings_test.go` — test LogLevel round-trip + coercion.
- `internal/app/connect.go` — pass `LogLevel` into `xrayconf.Options` and `ConnConfig`.
- `internal/xrayconf/build.go` — `Options.LogLevel` used for `log.loglevel`.
- `internal/xrayconf/build_test.go` — test loglevel from options.
- `internal/tun/tunlog.go` — `SetLogLevel`, `zapLevelFor`, level-aware logger.
- `internal/tun/tun.go` — engine key + logger use the configured level.
- `internal/tun/tunlog_test.go` — test `zapLevelFor` mapping.
- `gui_connector.go` — call `tun.SetLogLevel(c.LogLevel)` for TUN mode.

Frontend (full rewrites):
- `frontend/index.html` — three tabs, Russian labels, log-level select.
- `frontend/src/style.css` — dark + green theme, tab bar, round connect button.
- `frontend/src/main.ts` — tab switching, connect-button state machine, log-level wiring, Russian status labels.

---

## Task 1: store — LogLevel field, constants, normalize, migration

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/store/store_test.go`:

```go
func TestNormalizeLogLevel(t *testing.T) {
	cases := map[string]string{
		"error":   "error",
		"warning": "warning",
		"debug":   "debug",
		"":        "warning",
		"bogus":   "warning",
		"info":    "warning",
	}
	for in, want := range cases {
		if got := NormalizeLogLevel(in); got != want {
			t.Errorf("NormalizeLogLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadMigratesMissingLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// state.json written before the LogLevel field existed.
	if err := os.WriteFile(path, []byte(`{"settings":{"mode":"tun"}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Settings.LogLevel != LogNormal {
		t.Errorf("LogLevel = %q, want %q", s.Settings.LogLevel, LogNormal)
	}
}

func TestDefaultStateLogLevel(t *testing.T) {
	if got := DefaultState().Settings.LogLevel; got != LogNormal {
		t.Errorf("default LogLevel = %q, want %q", got, LogNormal)
	}
}
```

Ensure `store_test.go` imports `os`, `path/filepath` (add if missing).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run 'LogLevel|MigratesMissing' -v`
Expected: FAIL — `NormalizeLogLevel`/`LogNormal` undefined.

- [ ] **Step 3: Implement**

In `internal/store/store.go`, after the `Mode` consts block, add:

```go
// Log verbosity levels. Values are xray-native (also valid tun2socks engine
// levels) so they map straight through with no translation.
const (
	LogQuiet   = "error"   // UI: Тихо
	LogNormal  = "warning" // UI: Обычный (default)
	LogVerbose = "debug"   // UI: Подробно
)

// NormalizeLogLevel returns level if supported, otherwise LogNormal. Empty or
// unknown values (including state files written before this field existed) fall
// back to warning.
func NormalizeLogLevel(level string) string {
	switch level {
	case LogQuiet, LogNormal, LogVerbose:
		return level
	default:
		return LogNormal
	}
}
```

Add the field to `Settings`:

```go
type Settings struct {
	Mode        Mode   `json:"mode"`
	AutoStart   bool   `json:"autoStart"`
	AutoConnect bool   `json:"autoConnect"`
	KillSwitch  bool   `json:"killSwitch"`
	Mux         bool   `json:"mux"`
	// LogLevel is the xray-native log verbosity (error/warning/debug). See the
	// Log* constants. Drives both xray's log.loglevel and the tun2socks engine.
	LogLevel string `json:"logLevel"`
}
```

Set the default in `DefaultState`:

```go
		Settings:     Settings{Mode: ModeTUN, LogLevel: LogNormal},
```

Normalize on load — in `Load`, just before `return &s, nil`:

```go
	s.Settings.LogLevel = NormalizeLogLevel(s.Settings.LogLevel)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS (all store tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): add configurable log level setting

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: app — SettingsDTO + UpdateSettings plumbing

**Files:**
- Modify: `internal/app/types.go`, `internal/app/settings.go`
- Test: `internal/app/settings_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/app/settings_test.go`:

```go
func TestUpdateSettingsPersistsLogLevel(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", LogLevel: "debug"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if got := svc.GetState().Settings.LogLevel; got != "debug" {
		t.Errorf("LogLevel = %q, want debug", got)
	}
}

func TestUpdateSettingsCoercesInvalidLogLevel(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", LogLevel: "screaming"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if got := svc.GetState().Settings.LogLevel; got != "warning" {
		t.Errorf("LogLevel = %q, want warning (coerced)", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run UpdateSettings -v`
Expected: FAIL — `SettingsDTO` has no `LogLevel` field.

- [ ] **Step 3: Implement**

In `internal/app/types.go`, add to `SettingsDTO`:

```go
	Mux         bool   `json:"mux"`
	LogLevel    string `json:"logLevel"`
```

In `settingsDTO`, copy it:

```go
		Mux:         s.Mux,
		LogLevel:    s.LogLevel,
```

Add to `ConnConfig` (used in Task 3):

```go
	KillSwitch bool
	LogLevel   string
```

In `internal/app/settings.go`, inside `UpdateSettings` `store.Settings{...}`:

```go
		Mux:         in.Mux,
		LogLevel:    store.NormalizeLogLevel(in.LogLevel),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -run UpdateSettings -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/types.go internal/app/settings.go internal/app/settings_test.go
git commit -m "feat(app): persist log level through settings DTO

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: xrayconf — Options.LogLevel drives log.loglevel

**Files:**
- Modify: `internal/xrayconf/build.go`
- Test: `internal/xrayconf/build_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/xrayconf/build_test.go`:

```go
func TestBuildUsesLogLevel(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808, LogLevel: "debug"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var cfg struct {
		Log struct {
			LogLevel string `json:"loglevel"`
		} `json:"log"`
	}
	if err := json.Unmarshal(got, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Log.LogLevel != "debug" {
		t.Errorf("loglevel = %q, want debug", cfg.Log.LogLevel)
	}
}

func TestBuildDefaultsLogLevelToWarning(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var cfg struct {
		Log struct {
			LogLevel string `json:"loglevel"`
		} `json:"log"`
	}
	if err := json.Unmarshal(got, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Log.LogLevel != "warning" {
		t.Errorf("loglevel = %q, want warning", cfg.Log.LogLevel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/xrayconf/ -run LogLevel -v`
Expected: FAIL — `Options` has no `LogLevel` field.

- [ ] **Step 3: Implement**

In `internal/xrayconf/build.go`, add to `Options`:

```go
	// LogLevel is xray's log.loglevel (error/warning/debug). Empty defaults to
	// warning.
	LogLevel string
```

In `Build`, replace the `Log:` line. Currently:

```go
		Log: logConf{LogLevel: "warning", Error: opts.LogFile},
```

with:

```go
		Log: logConf{LogLevel: orDefault(opts.LogLevel, "warning"), Error: opts.LogFile},
```

(`orDefault` already exists in this file.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/xrayconf/ -v`
Expected: PASS. The golden test is unaffected — `TestBuildGolden` calls `Build` with no `LogLevel`, so it still emits `warning`.

- [ ] **Step 5: Commit**

```bash
git add internal/xrayconf/build.go internal/xrayconf/build_test.go
git commit -m "feat(xray): set log.loglevel from Options.LogLevel

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: connect — pass LogLevel to xray and ConnConfig

**Files:**
- Modify: `internal/app/connect.go`
- Test: `internal/app/connect_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/app/connect_test.go`:

```go
func TestConnectPassesLogLevel(t *testing.T) {
	svc, _, _, captured := testDeps(t) // elevated, default Mode=tun
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", LogLevel: "debug"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if captured.LogLevel != "debug" {
		t.Errorf("ConnConfig.LogLevel = %q, want debug", captured.LogLevel)
	}
	if !strings.Contains(string(captured.XrayJSON), `"loglevel": "debug"`) &&
		!strings.Contains(string(captured.XrayJSON), `"loglevel":"debug"`) {
		t.Errorf("xray JSON missing debug loglevel: %s", captured.XrayJSON)
	}
}
```

(`strings` is already imported in `connect_test.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run ConnectPassesLogLevel -v`
Expected: FAIL — `captured.LogLevel` empty.

- [ ] **Step 3: Implement**

In `internal/app/connect.go`, add `LogLevel` to the `xrayconf.Build` options:

```go
	cfgJSON, err := xrayconf.Build(srv, s.state.Profile, xrayconf.Options{
		SocksPort: socksPort,
		LogFile:   s.xrayLogPath(),
		LogLevel:  s.state.Settings.LogLevel,
		// Mux tames the Telegram connection storm. It drops the xtls-rprx-vision
		// flow, so it only works when the server's client is configured with no
		// flow. User-gated to avoid breaking vision-only servers.
		Mux: s.state.Settings.Mux,
	})
```

And set it on `ConnConfig` (do this right after `cc` is constructed, so it applies to both modes). Find:

```go
	cc := ConnConfig{
		XrayJSON:  cfgJSON,
		SocksHost: "127.0.0.1",
		SocksPort: socksPort,
		Mode:      mode,
	}
```

Add the field:

```go
	cc := ConnConfig{
		XrayJSON:  cfgJSON,
		SocksHost: "127.0.0.1",
		SocksPort: socksPort,
		Mode:      mode,
		LogLevel:  s.state.Settings.LogLevel,
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -v`
Expected: PASS (all app tests).

- [ ] **Step 5: Commit**

```bash
git add internal/app/connect.go internal/app/connect_test.go
git commit -m "feat(app): thread log level into xray config and ConnConfig

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: tun — level-aware engine logger

**Files:**
- Modify: `internal/tun/tunlog.go`, `internal/tun/tun.go`
- Test: `internal/tun/tunlog_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/tun/tunlog_test.go`:

```go
package tun

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestZapLevelFor(t *testing.T) {
	cases := map[string]zapcore.Level{
		"debug":   zapcore.DebugLevel,
		"warning": zapcore.WarnLevel,
		"error":   zapcore.ErrorLevel,
		"":        zapcore.WarnLevel,
		"bogus":   zapcore.WarnLevel,
	}
	for in, want := range cases {
		if got := zapLevelFor(in); got != want {
			t.Errorf("zapLevelFor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetLogLevelUpdatesEngineLevel(t *testing.T) {
	t.Cleanup(func() { engineLevel = "warning" })
	SetLogLevel("debug")
	if engineLevel != "debug" {
		t.Errorf("engineLevel = %q, want debug", engineLevel)
	}
	SetLogLevel("nonsense")
	if engineLevel != "warning" {
		t.Errorf("engineLevel = %q, want warning (coerced)", engineLevel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tun/ -run 'ZapLevelFor|SetLogLevel' -v`
Expected: FAIL — `zapLevelFor`, `engineLevel`, `SetLogLevel` undefined.

- [ ] **Step 3: Implement**

Rewrite `internal/tun/tunlog.go`:

```go
package tun

import (
	"io"

	t2log "github.com/xjasonlyu/tun2socks/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// engineLogWriter, when set, receives the tun2socks engine's log output.
var engineLogWriter io.Writer

// engineLevel is the xray-native verbosity (error/warning/debug) used for both
// the engine key and the captured logger. Defaults to warning.
var engineLevel = "warning"

// SetLogWriter routes the tun2socks engine's logs to w. It must be called
// before Start. The GUI process has no usable stderr, so the engine's
// per-connection lines and device read errors are otherwise lost.
func SetLogWriter(w io.Writer) { engineLogWriter = w }

// SetLogLevel sets the engine verbosity. Unknown values fall back to warning.
// Must be called before Start to take effect for a session.
func SetLogLevel(level string) { engineLevel = normalizeLevel(level) }

func normalizeLevel(level string) string {
	switch level {
	case "error", "warning", "debug":
		return level
	default:
		return "warning"
	}
}

func zapLevelFor(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.WarnLevel
	}
}

// installEngineLogger points the tun2socks global logger at w at the configured
// level. engine.Start installs its own logger during startup, so this must run
// after engine.Start to take effect.
func installEngineLogger(w io.Writer) {
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(w), zapLevelFor(engineLevel))
	t2log.SetLogger(zap.New(core))
}
```

In `internal/tun/tun.go`, change the engine key to use the configured level. Replace:

```go
	key := &engine.Key{
		Device:   "tun://" + device,
		Proxy:    socksURL,
		LogLevel: "warning",
	}
```

with:

```go
	key := &engine.Key{
		Device:   "tun://" + device,
		Proxy:    socksURL,
		LogLevel: engineLevel,
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tun/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tun/tunlog.go internal/tun/tun.go internal/tun/tunlog_test.go
git commit -m "feat(tun): make engine log level configurable

Replaces the always-on Info instrumentation: tun2socks now logs at the
user-selected level (default warning), so verbose per-connection lines
appear only on Подробно/debug.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: gui_connector — apply log level for TUN sessions

**Files:**
- Modify: `gui_connector.go`

(No Go unit test — `gui_connector.go` is `//go:build wails` glue; covered by the build check in Task 10.)

- [ ] **Step 1: Implement**

In `gui_connector.go`, inside `newConnector`, in the `case store.ModeTUN:` branch, set the engine log level before the connector is built. After the existing `cfg.KillSwitch = c.KillSwitch` line, add:

```go
		tun.SetLogLevel(c.LogLevel)
```

(The `tun` package is already imported.)

- [ ] **Step 2: Verify it compiles (wails tag)**

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags wails ./...`
Expected: builds with no error.

- [ ] **Step 3: Commit**

```bash
git add gui_connector.go
git commit -m "feat(gui): apply selected log level to the tun2socks engine

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 7: frontend — HTML (tabs, Russian, log-level select)

**Files:**
- Modify: `frontend/index.html` (full rewrite)

- [ ] **Step 1: Replace the file**

Write `frontend/index.html`:

```html
<!doctype html>
<html lang="ru">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>VLESS Client</title>
  </head>
  <body>
    <div id="app">
      <nav id="tabbar">
        <button class="tab active" data-tab="home">Главная</button>
        <button class="tab" data-tab="settings">Настройки</button>
        <button class="tab" data-tab="logs">Логи</button>
      </nav>

      <!-- ГЛАВНАЯ -->
      <section id="tab-home" class="tab-panel">
        <div class="home">
          <button id="power-btn" class="power disconnected" title="Подключить/отключить">
            <span class="power-glyph">⏻</span>
          </button>
          <div id="status-text" class="status-text">Отключено</div>
          <label class="server-pick">
            <span class="label">Сервер</span>
            <select id="server-select"></select>
          </label>
          <p id="error-line" class="error"></p>
        </div>
      </section>

      <!-- НАСТРОЙКИ -->
      <section id="tab-settings" class="tab-panel hidden">
        <h2>Серверы</h2>
        <div class="row">
          <input id="link-input" type="text" placeholder="vless://..." />
          <button id="add-server-btn">Добавить</button>
        </div>
        <p id="link-error" class="error"></p>
        <ul id="server-list"></ul>

        <h2>Режим</h2>
        <div class="row">
          <select id="mode-select">
            <option value="proxy">Proxy (системный SOCKS)</option>
            <option value="tun">TUN (нужны права администратора)</option>
          </select>
        </div>

        <h2>Маршрутизация</h2>
        <label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
        <label><input type="checkbox" id="ru-toggle" /> RU напрямую</label>
        <label><input type="checkbox" id="kill-toggle" /> Kill switch (TUN)</label>
        <label><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>

        <h2>Логи</h2>
        <label class="row">
          <span class="label">Уровень</span>
          <select id="loglevel-select">
            <option value="error">Тихо</option>
            <option value="warning">Обычный</option>
            <option value="debug">Подробно</option>
          </select>
        </label>
      </section>

      <!-- ЛОГИ -->
      <section id="tab-logs" class="tab-panel hidden">
        <div class="row">
          <button id="clear-logs-btn">Очистить</button>
        </div>
        <pre id="log-view"></pre>
      </section>
    </div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/index.html
git commit -m "feat(ui): three Russian tabs with status button and log-level select

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 8: frontend — CSS (dark + green theme)

**Files:**
- Modify: `frontend/src/style.css` (full rewrite)

- [ ] **Step 1: Replace the file**

Write `frontend/src/style.css`:

```css
:root {
  --bg: #0d1117;
  --surface: #161b22;
  --surface-2: #1c2128;
  --text: #e6e9ef;
  --muted: #6b7280;
  --border: #1e2630;
  --accent: #10b981;
  --accent-dim: #059669;
  --warn: #d4a017;
  --error: #f87171;
  font-family: system-ui, sans-serif;
  color: var(--text);
  background: var(--bg);
}

body { margin: 0; }
#app { max-width: 720px; margin: 0 auto; min-height: 100vh; display: flex; flex-direction: column; }

/* Tab bar */
#tabbar { display: flex; gap: 0.25rem; padding: 0 0.75rem; border-bottom: 1px solid var(--border); background: var(--surface); }
.tab {
  background: none; border: none; color: var(--muted);
  padding: 0.85rem 1rem; font-size: 0.95rem; cursor: pointer;
  border-bottom: 2px solid transparent;
}
.tab:hover { color: var(--text); }
.tab.active { color: var(--accent); border-bottom-color: var(--accent); }

.tab-panel { padding: 1.25rem; }
.tab-panel.hidden { display: none; }

/* Home */
.home { display: flex; flex-direction: column; align-items: center; gap: 1rem; padding-top: 2.5rem; }
.power {
  width: 132px; height: 132px; border-radius: 50%; border: none; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  color: #fff; transition: background 0.25s, box-shadow 0.25s;
}
.power-glyph { font-size: 3.2rem; line-height: 1; }
.power.disconnected, .power.error { background: var(--muted); box-shadow: 0 8px 24px rgba(0,0,0,.4); }
.power.connected { background: radial-gradient(circle at 30% 30%, var(--accent), var(--accent-dim)); box-shadow: 0 8px 28px rgba(16,185,129,.45); }
.power.connecting, .power.disconnecting { background: var(--warn); box-shadow: 0 8px 24px rgba(212,160,23,.4); animation: pulse 1.1s ease-in-out infinite; }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.55; } }

.status-text { font-size: 1rem; color: var(--text); }
.server-pick { display: flex; flex-direction: column; align-items: center; gap: 0.3rem; }

/* Controls */
h2 { font-size: 0.95rem; margin: 1.4rem 0 0.5rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; }
.row { display: flex; gap: 0.5rem; align-items: center; margin: 0.5rem 0; }
.label { font-size: 0.85rem; color: var(--muted); }
input[type="text"], select {
  padding: 0.45rem 0.55rem; background: var(--surface-2); border: 1px solid var(--border);
  color: var(--text); border-radius: 6px;
}
input[type="text"] { flex: 1; }
button {
  padding: 0.45rem 0.8rem; background: var(--surface-2); color: var(--text);
  border: 1px solid var(--border); border-radius: 6px; cursor: pointer;
}
button:hover { background: #28303a; }
ul { list-style: none; padding: 0; }
li { display: flex; align-items: center; gap: 0.5rem; padding: 0.3rem 0; }
li.active span { font-weight: 600; color: var(--accent); }
li span { flex: 1; }
label { display: block; margin: 0.4rem 0; }
.error { color: var(--error); min-height: 1em; font-size: 0.85rem; }

#log-view {
  background: #0a0d11; padding: 0.6rem; height: 60vh; overflow-y: auto;
  font-size: 0.78rem; white-space: pre-wrap; border-radius: 6px; border: 1px solid var(--border);
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/style.css
git commit -m "feat(ui): dark green theme with round status button

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 9: frontend — main.ts (tabs, button state machine, log level)

**Files:**
- Modify: `frontend/src/main.ts` (full rewrite)

- [ ] **Step 1: Replace the file**

Write `frontend/src/main.ts`:

```ts
import "./style.css";
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
} from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime";

type Profile = {
  telegram: boolean;
  forceRUDirect: boolean;
  customProxyDomains: string[];
  customProxyIPs: string[];
};
type Settings = {
  mode: string;
  autoConnect: boolean;
  autoStart: boolean;
  killSwitch: boolean;
  mux: boolean;
  logLevel: string;
};
type Server = { name: string; host: string; port: number; security: string; network: string };
type State = {
  servers: Server[];
  activeServer: number;
  profile: Profile;
  settings: Settings;
  conn: string;
  lastError: string;
};

const $ = (id: string) => document.getElementById(id)!;

const STATUS: Record<string, string> = {
  connected: "Подключено",
  connecting: "Подключение…",
  disconnecting: "Отключение…",
  disconnected: "Отключено",
  error: "Ошибка",
};

let current: State;

function setTab(name: string) {
  document.querySelectorAll<HTMLElement>(".tab").forEach((b) => {
    b.classList.toggle("active", b.dataset.tab === name);
  });
  document.querySelectorAll<HTMLElement>(".tab-panel").forEach((p) => {
    p.classList.toggle("hidden", p.id !== "tab-" + name);
  });
}

function render(st: State) {
  // Power button: color from conn state.
  const btn = $("power-btn");
  btn.className = "power " + st.conn;
  $("status-text").textContent = STATUS[st.conn] ?? st.conn;
  $("error-line").textContent = st.lastError || "";

  // Active-server selector on Главная.
  const sel = <HTMLSelectElement>$("server-select");
  sel.innerHTML = "";
  if (st.servers.length === 0) {
    const opt = document.createElement("option");
    opt.textContent = "Нет серверов — добавьте в Настройках";
    opt.value = "-1";
    sel.append(opt);
    sel.disabled = true;
  } else {
    sel.disabled = false;
    st.servers.forEach((s, i) => {
      const opt = document.createElement("option");
      opt.value = String(i);
      opt.textContent = `${s.name} (${s.host}:${s.port})`;
      sel.append(opt);
    });
    sel.value = String(st.activeServer);
  }

  // Server list on Настройки.
  const list = $("server-list");
  list.innerHTML = "";
  st.servers.forEach((s, i) => {
    const li = document.createElement("li");
    li.className = i === st.activeServer ? "active" : "";
    li.innerHTML = `<span>${s.name} (${s.host}:${s.port})</span>`;
    const pick = document.createElement("button");
    pick.textContent = "Выбрать";
    pick.onclick = () => SetActiveServer(i);
    const del = document.createElement("button");
    del.textContent = "Удалить";
    del.onclick = () => RemoveServer(i);
    li.append(pick, del);
    list.append(li);
  });

  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  (<HTMLSelectElement>$("mode-select")).value = st.settings.mode;
  (<HTMLInputElement>$("kill-toggle")).checked = st.settings.killSwitch;
  (<HTMLInputElement>$("mux-toggle")).checked = st.settings.mux;
  (<HTMLSelectElement>$("loglevel-select")).value = st.settings.logLevel || "warning";
}

function refresh() {
  GetState().then((st) => {
    current = st as State;
    render(current);
  });
}

function appendLog(line: string) {
  const view = $("log-view");
  view.textContent += line + "\n";
  view.scrollTop = view.scrollHeight;
}

function pushSettings() {
  UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
}

function wire() {
  // Tabs
  document.querySelectorAll<HTMLElement>(".tab").forEach((b) => {
    b.addEventListener("click", () => setTab(b.dataset.tab!));
  });

  // Power button: toggle based on current state.
  $("power-btn").addEventListener("click", () => {
    const c = current?.conn;
    if (c === "connected") {
      Disconnect().catch((e) => ($("error-line").textContent = String(e)));
    } else if (c === "disconnected" || c === "error") {
      Connect().catch((e) => ($("error-line").textContent = String(e)));
    }
    // connecting/disconnecting: ignore.
  });

  $("server-select").addEventListener("change", () => {
    const v = parseInt((<HTMLSelectElement>$("server-select")).value, 10);
    if (v >= 0) SetActiveServer(v);
  });

  $("add-server-btn").addEventListener("click", () => {
    const input = <HTMLInputElement>$("link-input");
    AddServer(input.value)
      .then(() => {
        input.value = "";
        $("link-error").textContent = "";
      })
      .catch((e) => ($("link-error").textContent = String(e)));
  });

  $("tg-toggle").addEventListener("change", () => {
    current.profile.telegram = (<HTMLInputElement>$("tg-toggle")).checked;
    UpdateProfile(current.profile);
  });
  $("ru-toggle").addEventListener("change", () => {
    current.profile.forceRUDirect = (<HTMLInputElement>$("ru-toggle")).checked;
    UpdateProfile(current.profile);
  });
  $("mode-select").addEventListener("change", () => {
    current.settings.mode = (<HTMLSelectElement>$("mode-select")).value;
    pushSettings();
  });
  $("kill-toggle").addEventListener("change", () => {
    current.settings.killSwitch = (<HTMLInputElement>$("kill-toggle")).checked;
    pushSettings();
  });
  $("mux-toggle").addEventListener("change", () => {
    current.settings.mux = (<HTMLInputElement>$("mux-toggle")).checked;
    pushSettings();
  });
  $("loglevel-select").addEventListener("change", () => {
    current.settings.logLevel = (<HTMLSelectElement>$("loglevel-select")).value;
    pushSettings();
  });
  $("clear-logs-btn").addEventListener("click", () => {
    $("log-view").textContent = "";
  });

  EventsOn("state", (st: State) => {
    current = st;
    render(st);
  });
  EventsOn("log", (line: string) => appendLog(line));
}

document.addEventListener("DOMContentLoaded", () => {
  wire();
  refresh();
  Logs().then((lines) => (lines as string[]).forEach(appendLog));
});
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/main.ts
git commit -m "feat(ui): tab switching, status-driven power button, log-level wiring

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 10: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Go tests**

Run: `go test ./...`
Expected: all packages `ok`.

- [ ] **Step 2: Linux build**

Run: `go build ./...`
Expected: no error.

- [ ] **Step 3: Windows cross-build (wails tag)**

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags wails ./...`
Expected: no error.

- [ ] **Step 4: Frontend build**

Run: `cd frontend && npm install && npm run build`
Expected: Vite build succeeds (TypeScript compiles, no type errors). Return to repo root afterwards.

- [ ] **Step 5: Manual smoke (user, on Windows)**

Build the app (`wails build -platform windows/amd64 -tags wails`) and confirm:
- three tabs switch correctly,
- power button is grey when disconnected, yellow+pulsing while connecting, green when connected,
- changing Уровень логов to Подробно makes xray.log/tun.log verbose, Тихо makes them quiet,
- all other settings persist across restart.

(No commit — verification step.)

---

## Self-Review Notes

- **Spec coverage:** tabs (T7/T9), dark+green theme (T8), status button (T8/T9), server selector on home (T7/T9), all controls on settings tab (T7), logs tab (T7), 3-level log setting wired to xray (T3/T4) and tun2socks (T5/T6), Russian text (T7/T9), state migration (T1), tests (T1–T5). All covered.
- **Defaults:** warning default enforced in store default + load migration + xrayconf fallback + tun normalize — consistent.
- **Type consistency:** stored values `error`/`warning`/`debug` used identically across store, app, xrayconf, tun, and the frontend `<option value>`s.
