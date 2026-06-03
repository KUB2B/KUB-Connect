# VLESS Client — Phase 3: Wails GUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wrap the existing headless core in a cross-platform Wails desktop GUI: a testable Go service layer (connection state machine, server/profile/settings management on top of `internal/store`, vless validation), live log streaming, and a vanilla-TS frontend with status/servers/routing/logs panels — shipping a usable proxy-mode VPN client on Windows, macOS, and Linux.

**Architecture:** A new pure-Go `internal/app` service owns application state and the Disconnected→Connecting→Connected→Disconnecting/Error state machine. It depends only on small injected interfaces — an `Emitter` (abstracts Wails `runtime.EventsEmit`) and a `ConnectorFactory` (builds something with `Start()/Stop()`) — so the whole service is unit-tested with fakes and never imports platform code. A new `internal/logbus` provides a thread-safe ring buffer with subscribers plus a poll-based file tailer that streams xray's error log into the buffer. The Wails layer (one binding struct + `main`, both behind a `//go:build wails` tag so default `go build ./...` stays green on machines without webkit) wires the real proxy-mode connector (`tunnel` + `sysproxy`) and a `runtime.EventsEmit` emitter. The frontend is vanilla TypeScript calling generated Wails bindings and listening to `state`/`log` events.

**Tech Stack:** Go 1.26.1, existing `internal/{vless,routing,xrayconf,core,store,tunnel,sysproxy,privilege}`, [Wails v2](https://wails.io) (`github.com/wailsapp/wails/v2`), Node/npm (present), vanilla-TS frontend template. No new runtime networking deps.

**Phasing note:** Phase 3 of (now) 5. Delivers the GUI over **proxy mode**, which builds and runs on all three OSes. Deliberately deferred to **Phase 4 (networking)**: `netcfg` for darwin/windows, full default-route TUN with loop avoidance, wiring TUN into the GUI connector, kill switch, and supervising the tun2socks `engine.Start/Stop` `log.Fatalf` behavior (irrelevant in proxy mode). **Phase 5:** autostart, ping/latency test, traffic stats (xray StatsService). The `Settings` struct already carries `AutoStart`/`KillSwitch`; this phase surfaces them in the UI as disabled "Phase 4/5" controls.

**Why proxy-only this phase:** `internal/netcfg` has a Linux-only implementation. The GUI targets Windows + macOS. A proxy-mode connector imports only `tunnel` + `sysproxy` + `core` (all cross-platform), so `cmd`/root builds everywhere. Pulling in `netcfg`/`tun` would break Win/mac builds — that integration is exactly Phase 4's job.

---

### Task 1: logbus — ring buffer with subscribers

**Files:**
- Create: `internal/logbus/logbus.go`
- Test: `internal/logbus/logbus_test.go`

A thread-safe capped line buffer. Callers `Append` lines; subscribers receive each new line; `Lines` returns a snapshot for initial UI paint.

- [ ] **Step 1: Write the failing test**

`internal/logbus/logbus_test.go`:
```go
package logbus

import (
	"reflect"
	"testing"
)

func TestAppendCapsAtCapacity(t *testing.T) {
	b := New(3)
	for _, l := range []string{"a", "b", "c", "d"} {
		b.Append(l)
	}
	got := b.Lines()
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lines = %v, want %v", got, want)
	}
}

func TestSubscriberReceivesNewLines(t *testing.T) {
	b := New(10)
	var got []string
	cancel := b.Subscribe(func(line string) { got = append(got, line) })
	b.Append("one")
	b.Append("two")
	cancel()
	b.Append("three") // after cancel: must not be delivered
	want := []string{"one", "two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subscriber got %v, want %v", got, want)
	}
}

func TestLinesReturnsCopy(t *testing.T) {
	b := New(10)
	b.Append("x")
	snap := b.Lines()
	snap[0] = "mutated"
	if b.Lines()[0] != "x" {
		t.Error("Lines() must return a copy; internal state was mutated")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logbus/ -v`
Expected: FAIL — package does not compile (`undefined: New`).

- [ ] **Step 3: Implement the buffer**

`internal/logbus/logbus.go`:
```go
// Package logbus is a thread-safe, capped, fan-out line buffer used to stream
// log output to the GUI. Producers call Append; the GUI subscribes for live
// lines and calls Lines once for the initial backlog.
package logbus

import "sync"

// Bus is a capped ring buffer with fan-out subscribers.
type Bus struct {
	mu     sync.Mutex
	cap    int
	lines  []string
	nextID int
	subs   map[int]func(string)
}

// New returns a Bus retaining at most capacity lines (default 1000 if <= 0).
func New(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Bus{cap: capacity, subs: map[int]func(string){}}
}

// Append records a line and delivers it to all current subscribers.
func (b *Bus) Append(line string) {
	b.mu.Lock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.cap {
		b.lines = b.lines[len(b.lines)-b.cap:]
	}
	subs := make([]func(string), 0, len(b.subs))
	for _, fn := range b.subs {
		subs = append(subs, fn)
	}
	b.mu.Unlock()
	for _, fn := range subs {
		fn(line)
	}
}

// Lines returns a copy of the retained lines.
func (b *Bus) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// Subscribe registers fn to receive every subsequent Append. The returned
// function unsubscribes.
func (b *Bus) Subscribe(fn func(string)) (cancel func()) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[id] = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logbus/ -v && gofmt -l internal/logbus/ && go vet ./internal/logbus/`
Expected: all PASS, no fmt/vet output.

- [ ] **Step 5: Commit**

```bash
git add internal/logbus/logbus.go internal/logbus/logbus_test.go
git commit -m "feat(logbus): thread-safe capped line buffer with subscribers"
```

---

### Task 2: logbus — poll-based file tailer

**Files:**
- Create: `internal/logbus/tail.go`
- Test: `internal/logbus/tail_test.go`

Streams an appended-to log file (xray's error log) into a `Bus`. Poll-based so it needs no fsnotify dependency and behaves identically on every OS.

- [ ] **Step 1: Write the failing test**

`internal/logbus/tail_test.go`:
```go
package logbus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForLine polls until bus contains want or the deadline passes.
func waitForLine(t *testing.T, bus *Bus, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, l := range bus.Lines() {
			if l == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for line %q; got %v", want, bus.Lines())
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestTailFileStreamsAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray.log")
	if err := os.WriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	bus := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TailFile(ctx, path, bus, 10*time.Millisecond)

	waitForLine(t, bus, "first")
	appendLine(t, path, "second")
	waitForLine(t, bus, "second")
}

func TestTailFileMissingFileDoesNotPanic(t *testing.T) {
	bus := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	go TailFile(ctx, filepath.Join(t.TempDir(), "nope.log"), bus, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond) // a few poll cycles against a missing file
	cancel()
	if len(bus.Lines()) != 0 {
		t.Errorf("expected no lines from missing file, got %v", bus.Lines())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logbus/ -run TestTailFile -v`
Expected: FAIL — `undefined: TailFile`.

- [ ] **Step 3: Implement the tailer**

`internal/logbus/tail.go`:
```go
package logbus

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"
)

// TailFile polls path every interval, appending any newly written lines to
// bus, until ctx is cancelled. A missing or unreadable file is skipped (the
// xray log may not exist until the first connection). The current read offset
// is tracked across polls; a truncated file is handled by resetting to start.
func TailFile(ctx context.Context, path string, bus *Bus, interval time.Duration) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		offset = readFrom(path, offset, bus)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// readFrom opens path, reads complete lines from offset to EOF into bus, and
// returns the new offset. On any error it returns offset unchanged.
func readFrom(path string, offset int64, bus *Bus) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return offset
	}
	if offset > size { // file was truncated/rotated; start over
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		bus.Append(sc.Text())
	}
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return offset
	}
	return pos
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logbus/ -v && go vet ./internal/logbus/`
Expected: all PASS (both tailer tests + Task 1 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/logbus/tail.go internal/logbus/tail_test.go
git commit -m "feat(logbus): poll-based log file tailer"
```

---

### Task 3: xrayconf — optional error-log file

**Files:**
- Modify: `internal/xrayconf/build.go:11-14` (the `Options` struct), `internal/xrayconf/build.go:24-26` (`logConf`), `internal/xrayconf/build.go:117-118` (log assignment)
- Test: `internal/xrayconf/build_test.go`

The GUI needs xray to write its error log to a file the tailer can follow. Add an optional `LogFile`; when empty, output is unchanged (the golden test for the default config stays valid because the new field is `omitempty`).

- [ ] **Step 1: Write the failing test**

Append to `internal/xrayconf/build_test.go`:
```go
func TestBuildSetsErrorLogFile(t *testing.T) {
	s := &vless.ServerConfig{
		Name: "x", Host: "h", Port: 443, UUID: "u",
		Security: vless.SecurityReality, Network: vless.NetworkTCP,
		SNI: "www.example.com", PublicKey: "pbk", ShortID: "sid",
	}
	out, err := Build(s, routing.Default(), Options{SocksPort: 10808, LogFile: "/tmp/xray.log"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var parsed struct {
		Log struct {
			LogLevel string `json:"loglevel"`
			Error    string `json:"error"`
		} `json:"log"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Log.Error != "/tmp/xray.log" {
		t.Errorf("log.error = %q, want /tmp/xray.log", parsed.Log.Error)
	}
}

func TestBuildOmitsErrorLogWhenUnset(t *testing.T) {
	s := &vless.ServerConfig{
		Name: "x", Host: "h", Port: 443, UUID: "u",
		Security: vless.SecurityReality, Network: vless.NetworkTCP,
		SNI: "www.example.com", PublicKey: "pbk", ShortID: "sid",
	}
	out, err := Build(s, routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(string(out), `"error"`) {
		t.Errorf("expected no log.error key when LogFile unset; got %s", out)
	}
}
```

Ensure the test file imports `encoding/json` and `strings` (add to its import block if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/xrayconf/ -run TestBuild -v`
Expected: FAIL — `Options` has no field `LogFile` (compile error).

- [ ] **Step 3: Add the field and wire it**

In `internal/xrayconf/build.go`, change the `Options` struct:
```go
// Options holds runtime knobs for the generated config.
type Options struct {
	SocksPort int    // local SOCKS inbound port (e.g. 10808)
	LogFile   string // optional path for xray's error log (empty = stdout/default)
}
```

Change `logConf`:
```go
type logConf struct {
	LogLevel string `json:"loglevel"`
	Error    string `json:"error,omitempty"`
}
```

Change the log line inside `Build` (currently `Log: logConf{LogLevel: "warning"},`):
```go
		Log: logConf{LogLevel: "warning", Error: opts.LogFile},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/xrayconf/ -v && gofmt -l internal/xrayconf/ && go vet ./internal/xrayconf/`
Expected: all PASS, including the existing golden test (`reality_default.json` is built with `LogFile` unset, so output is byte-identical). If the golden test fails, do NOT pass `UPDATE_GOLDEN=1` — investigate, because the default output must not have changed.

- [ ] **Step 5: Commit**

```bash
git add internal/xrayconf/build.go internal/xrayconf/build_test.go
git commit -m "feat(xrayconf): optional error-log file path"
```

---

### Task 4: app — types, dependencies, and GetState

**Files:**
- Create: `internal/app/types.go`
- Create: `internal/app/app.go`
- Test: `internal/app/app_test.go`

Establishes the service skeleton: injected dependencies, the JSON DTOs the frontend consumes, construction (loads persisted state), and a `GetState` snapshot. No connection logic yet.

- [ ] **Step 1: Write the failing test**

`internal/app/app_test.go`:
```go
package app

import (
	"path/filepath"
	"testing"
)

// fakeEmitter records emitted events for assertions.
type fakeEmitter struct{ events []struct {
	name string
	data any
} }

func (e *fakeEmitter) Emit(name string, data any) {
	e.events = append(e.events, struct {
		name string
		data any
	}{name, data})
}

// fakeConnector satisfies Connector.
type fakeConnector struct{ started, stopped bool }

func (f *fakeConnector) Start() error { f.started = true; return nil }
func (f *fakeConnector) Stop() error  { f.stopped = true; return nil }

// testDeps builds a Service backed by a temp dir, an always-elevated stub, and
// a factory returning fc (recording the ConnConfig it was given).
func testDeps(t *testing.T) (*Service, *fakeEmitter, *fakeConnector, *ConnConfig) {
	t.Helper()
	dir := t.TempDir()
	em := &fakeEmitter{}
	fc := &fakeConnector{}
	var captured ConnConfig
	deps := Deps{
		StatePath: filepath.Join(dir, "state.json"),
		LogDir:    dir,
		Emitter:   em,
		Elevated:  func() bool { return true },
		Factory: func(c ConnConfig) (Connector, error) {
			captured = c
			return fc, nil
		},
	}
	svc, err := New(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc, em, fc, &captured
}

func TestNewLoadsDefaultStateWhenNoFile(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	st := svc.GetState()
	if st.Conn != string(ConnDisconnected) {
		t.Errorf("Conn = %q, want disconnected", st.Conn)
	}
	if len(st.Servers) != 0 {
		t.Errorf("expected 0 servers on fresh state, got %d", len(st.Servers))
	}
	if !st.Profile.Telegram || !st.Profile.ForceRUDirect {
		t.Errorf("default profile should have Telegram + ForceRUDirect on: %+v", st.Profile)
	}
	if st.Settings.Mode != string(store.ModeTUN) {
		// DefaultState ships Mode=tun; GUI surfaces it but Connect (Task 7) gates it.
		t.Errorf("default Mode = %q, want tun", st.Settings.Mode)
	}
}
```

Add the import for `store` to the test's import block:
```go
	"github.com/zki/vless-client/internal/store"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -v`
Expected: FAIL — package does not compile (`undefined: New`, `Deps`, `ConnConfig`, etc.).

- [ ] **Step 3: Write the DTOs and dependency types**

`internal/app/types.go`:
```go
// Package app is the GUI-facing service layer: it owns application state, the
// connection state machine, and persistence, exposing methods the Wails
// frontend binds to. It depends only on injected interfaces (Emitter,
// ConnectorFactory) so it is fully unit-testable and free of platform code.
package app

import (
	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/vless"
)

// ConnState is the connection state-machine state.
type ConnState string

const (
	ConnDisconnected  ConnState = "disconnected"
	ConnConnecting    ConnState = "connecting"
	ConnConnected     ConnState = "connected"
	ConnDisconnecting ConnState = "disconnecting"
	ConnError         ConnState = "error"
)

// socksPort is the fixed local SOCKS inbound port for this version.
const socksPort = 10808

// Emitter delivers named events with a JSON-serializable payload to the
// frontend. The Wails implementation wraps runtime.EventsEmit.
type Emitter interface {
	Emit(event string, data any)
}

// Connector is a started/stopped capture session (satisfied by *tunnel.Tunnel).
type Connector interface {
	Start() error
	Stop() error
}

// ConnConfig is what the service hands the factory to build a Connector.
type ConnConfig struct {
	XrayJSON  []byte
	SocksHost string
	SocksPort int
	Mode      store.Mode
}

// ConnectorFactory builds a Connector for a session. The Wails layer supplies
// the real (proxy-mode) implementation; tests supply a fake.
type ConnectorFactory func(ConnConfig) (Connector, error)

// Deps are the service's injected dependencies.
type Deps struct {
	StatePath string           // path to state.json
	LogDir    string           // directory for xray.log
	Emitter   Emitter          // event sink for the frontend
	Factory   ConnectorFactory // builds the capture session
	Elevated  func() bool      // reports OS privilege (privilege.IsElevated)
}

// ServerDTO is a server entry as shown in the UI.
type ServerDTO struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Network  string `json:"network"`
}

// ProfileDTO mirrors routing.Profile for the frontend.
type ProfileDTO struct {
	Telegram           bool     `json:"telegram"`
	ForceRUDirect      bool     `json:"forceRUDirect"`
	CustomProxyDomains []string `json:"customProxyDomains"`
	CustomProxyIPs     []string `json:"customProxyIPs"`
}

// SettingsDTO mirrors store.Settings for the frontend.
type SettingsDTO struct {
	Mode        string `json:"mode"`
	AutoConnect bool   `json:"autoConnect"`
	AutoStart   bool   `json:"autoStart"`
	KillSwitch  bool   `json:"killSwitch"`
}

// StateDTO is the full snapshot the frontend renders.
type StateDTO struct {
	Servers      []ServerDTO `json:"servers"`
	ActiveServer int         `json:"activeServer"`
	Profile      ProfileDTO  `json:"profile"`
	Settings     SettingsDTO `json:"settings"`
	Conn         string      `json:"conn"`
	LastError    string      `json:"lastError"`
}

func serverDTO(s *vless.ServerConfig) ServerDTO {
	return ServerDTO{
		Name:     s.Name,
		Host:     s.Host,
		Port:     s.Port,
		Security: string(s.Security),
		Network:  string(s.Network),
	}
}

func profileDTO(p routing.Profile) ProfileDTO {
	return ProfileDTO{
		Telegram:           p.Telegram,
		ForceRUDirect:      p.ForceRUDirect,
		CustomProxyDomains: p.CustomProxyDomains,
		CustomProxyIPs:     p.CustomProxyIPs,
	}
}

func settingsDTO(s store.Settings) SettingsDTO {
	return SettingsDTO{
		Mode:        string(s.Mode),
		AutoConnect: s.AutoConnect,
		AutoStart:   s.AutoStart,
		KillSwitch:  s.KillSwitch,
	}
}
```

- [ ] **Step 4: Write the service core**

`internal/app/app.go`:
```go
package app

import (
	"sync"

	"github.com/zki/vless-client/internal/logbus"
	"github.com/zki/vless-client/internal/store"
)

// Service is the GUI-facing application service.
type Service struct {
	deps Deps
	bus  *logbus.Bus

	mu        sync.Mutex
	state     *store.State
	conn      ConnState
	lastError string
	connector Connector
	tailStop  func() // cancels the log tailer goroutine; nil when not tailing
}

// New constructs the service, loading persisted state (or defaults).
func New(d Deps) (*Service, error) {
	st, err := store.Load(d.StatePath)
	if err != nil {
		return nil, err
	}
	return &Service{
		deps:  d,
		bus:   logbus.New(2000),
		state: st,
		conn:  ConnDisconnected,
	}, nil
}

// snapshot builds a StateDTO. Caller must hold s.mu.
func (s *Service) snapshot() StateDTO {
	servers := make([]ServerDTO, 0, len(s.state.Servers))
	for _, sv := range s.state.Servers {
		servers = append(servers, serverDTO(sv))
	}
	return StateDTO{
		Servers:      servers,
		ActiveServer: s.state.ActiveServer,
		Profile:      profileDTO(s.state.Profile),
		Settings:     settingsDTO(s.state.Settings),
		Conn:         string(s.conn),
		LastError:    s.lastError,
	}
}

// GetState returns the current state snapshot.
func (s *Service) GetState() StateDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

// emitState pushes the current snapshot to the frontend. Caller must hold s.mu.
func (s *Service) emitState() {
	if s.deps.Emitter != nil {
		s.deps.Emitter.Emit("state", s.snapshot())
	}
}

// persist writes state to disk. Caller must hold s.mu.
func (s *Service) persist() error {
	return store.Save(s.deps.StatePath, s.state)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/app/ -v && gofmt -l internal/app/ && go vet ./internal/app/`
Expected: `TestNewLoadsDefaultStateWhenNoFile` PASS, no fmt/vet output.

- [ ] **Step 6: Commit**

```bash
git add internal/app/types.go internal/app/app.go internal/app/app_test.go
git commit -m "feat(app): service skeleton, DTOs, GetState"
```

---

### Task 5: app — server CRUD with validation and persistence

**Files:**
- Create: `internal/app/servers.go`
- Test: `internal/app/servers_test.go`

Add/remove servers (parsing & validating `vless://`), select the active server, persist on every change, emit a fresh state snapshot.

- [ ] **Step 1: Write the failing test**

`internal/app/servers_test.go`:
```go
package app

import "testing"

const sampleLink = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@example.com:443" +
	"?type=tcp&security=reality&pbk=ABCpublicKey&fp=chrome&sni=www.microsoft.com" +
	"&sid=0123abcd&spx=%2F&flow=xtls-rprx-vision#my-server"

func TestAddServerParsesAndSelectsFirst(t *testing.T) {
	svc, em, _, _ := testDeps(t)
	if err := svc.AddServer(sampleLink); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	st := svc.GetState()
	if len(st.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(st.Servers))
	}
	if st.Servers[0].Host != "example.com" || st.Servers[0].Name != "my-server" {
		t.Errorf("server DTO wrong: %+v", st.Servers[0])
	}
	if st.ActiveServer != 0 {
		t.Errorf("first added server should become active, got %d", st.ActiveServer)
	}
	// A "state" event must have been emitted.
	if len(em.events) == 0 || em.events[len(em.events)-1].name != "state" {
		t.Error("AddServer should emit a state event")
	}
}

func TestAddServerRejectsBadLink(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.AddServer("not-a-vless-link"); err == nil {
		t.Error("expected error for malformed link")
	}
	if len(svc.GetState().Servers) != 0 {
		t.Error("bad link must not be stored")
	}
}

func TestAddServerPersists(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.AddServer(sampleLink); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	// A new service reading the same path must see the server.
	svc2, err := New(svc.deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(svc2.GetState().Servers) != 1 {
		t.Error("server was not persisted")
	}
}

func TestRemoveServerAdjustsActive(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	mustAdd(t, svc, sampleLink) // index 0
	mustAdd(t, svc, sampleLink) // index 1
	if err := svc.SetActiveServer(1); err != nil {
		t.Fatalf("SetActiveServer: %v", err)
	}
	if err := svc.RemoveServer(0); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	st := svc.GetState()
	if len(st.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(st.Servers))
	}
	if st.ActiveServer != 0 {
		t.Errorf("active index should shift to 0 after removing index 0, got %d", st.ActiveServer)
	}
}

func TestRemoveLastServerClearsActive(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	mustAdd(t, svc, sampleLink)
	if err := svc.RemoveServer(0); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	if svc.GetState().ActiveServer != -1 {
		t.Error("removing the only server should reset ActiveServer to -1")
	}
}

func TestRemoveServerOutOfRange(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.RemoveServer(0); err == nil {
		t.Error("expected error removing from empty list")
	}
}

func mustAdd(t *testing.T, svc *Service, link string) {
	t.Helper()
	if err := svc.AddServer(link); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run "Server" -v`
Expected: FAIL — `undefined: (*Service).AddServer` etc.

- [ ] **Step 3: Implement server CRUD**

`internal/app/servers.go`:
```go
package app

import (
	"fmt"

	"github.com/zki/vless-client/internal/vless"
)

// AddServer parses link, appends it, and (if it is the first) selects it.
func (s *Service) AddServer(link string) error {
	cfg, err := vless.Parse(link)
	if err != nil {
		return fmt.Errorf("parse link: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Servers = append(s.state.Servers, cfg)
	if s.state.ActiveServer < 0 {
		s.state.ActiveServer = 0
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}

// RemoveServer deletes the server at index and keeps ActiveServer valid.
func (s *Service) RemoveServer(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.state.Servers) {
		return fmt.Errorf("server index %d out of range", index)
	}
	s.state.Servers = append(s.state.Servers[:index], s.state.Servers[index+1:]...)
	switch {
	case len(s.state.Servers) == 0:
		s.state.ActiveServer = -1
	case s.state.ActiveServer > index:
		s.state.ActiveServer--
	case s.state.ActiveServer == index:
		s.state.ActiveServer = 0
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}

// SetActiveServer selects the active server by index.
func (s *Service) SetActiveServer(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.state.Servers) {
		return fmt.Errorf("server index %d out of range", index)
	}
	s.state.ActiveServer = index
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app/ -v && gofmt -l internal/app/ && go vet ./internal/app/`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/servers.go internal/app/servers_test.go
git commit -m "feat(app): server add/remove/select with validation and persistence"
```

---

### Task 6: app — profile and settings updates

**Files:**
- Create: `internal/app/settings.go`
- Test: `internal/app/settings_test.go`

Apply routing-profile and settings edits from the UI, persisting and emitting.

- [ ] **Step 1: Write the failing test**

`internal/app/settings_test.go`:
```go
package app

import (
	"reflect"
	"testing"
)

func TestUpdateProfilePersistsAndEmits(t *testing.T) {
	svc, em, _, _ := testDeps(t)
	p := ProfileDTO{
		Telegram:           false,
		ForceRUDirect:      true,
		CustomProxyDomains: []string{"youtube.com"},
		CustomProxyIPs:     []string{"1.2.3.4/32"},
	}
	if err := svc.UpdateProfile(p); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got := svc.GetState().Profile
	if !reflect.DeepEqual(got, p) {
		t.Errorf("profile = %+v, want %+v", got, p)
	}
	if em.events[len(em.events)-1].name != "state" {
		t.Error("UpdateProfile should emit a state event")
	}

	svc2, _ := New(svc.deps)
	if !reflect.DeepEqual(svc2.GetState().Profile, p) {
		t.Error("profile was not persisted")
	}
}

func TestUpdateSettingsPersists(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	in := SettingsDTO{Mode: "proxy", AutoConnect: true, AutoStart: false, KillSwitch: false}
	if err := svc.UpdateSettings(in); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	got := svc.GetState().Settings
	if got.Mode != "proxy" || !got.AutoConnect {
		t.Errorf("settings = %+v, want proxy + autoConnect", got)
	}
}

func TestUpdateSettingsRejectsBadMode(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "bogus"}); err == nil {
		t.Error("expected error for invalid mode")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run "Update" -v`
Expected: FAIL — `undefined: (*Service).UpdateProfile`.

- [ ] **Step 3: Implement updates**

`internal/app/settings.go`:
```go
package app

import (
	"fmt"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
)

// UpdateProfile replaces the routing profile.
func (s *Service) UpdateProfile(p ProfileDTO) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Profile = routing.Profile{
		Telegram:           p.Telegram,
		ForceRUDirect:      p.ForceRUDirect,
		CustomProxyDomains: p.CustomProxyDomains,
		CustomProxyIPs:     p.CustomProxyIPs,
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}

// UpdateSettings replaces app settings after validating the capture mode.
func (s *Service) UpdateSettings(in SettingsDTO) error {
	mode := store.Mode(in.Mode)
	if mode != store.ModeProxy && mode != store.ModeTUN {
		return fmt.Errorf("invalid mode %q", in.Mode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Settings = store.Settings{
		Mode:        mode,
		AutoConnect: in.AutoConnect,
		AutoStart:   in.AutoStart,
		KillSwitch:  in.KillSwitch,
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app/ -v && gofmt -l internal/app/ && go vet ./internal/app/`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/settings.go internal/app/settings_test.go
git commit -m "feat(app): profile and settings updates"
```

---

### Task 7: app — connection state machine (Connect/Disconnect)

**Files:**
- Create: `internal/app/connect.go`
- Test: `internal/app/connect_test.go`

The heart of the GUI: build the xray config from the active server + profile, drive Disconnected→Connecting→Connected (or →Error), and the reverse on Disconnect. Proxy mode only this phase; TUN is rejected with a clear message. Each transition emits a `state` event. On connect, the xray log tailer starts; on disconnect it stops.

- [ ] **Step 1: Write the failing test**

`internal/app/connect_test.go`:
```go
package app

import (
	"errors"
	"testing"

	"github.com/zki/vless-client/internal/store"
)

func connectReadyService(t *testing.T) (*Service, *fakeEmitter, *fakeConnector, *ConnConfig) {
	t.Helper()
	svc, em, fc, captured := testDeps(t)
	mustAdd(t, svc, sampleLink)
	// Default state ships Mode=tun; switch to proxy for the happy path.
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	return svc, em, fc, captured
}

func TestConnectProxyHappyPath(t *testing.T) {
	svc, em, fc, captured := connectReadyService(t)
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !fc.started {
		t.Error("connector should have been started")
	}
	if svc.GetState().Conn != string(ConnConnected) {
		t.Errorf("Conn = %q, want connected", svc.GetState().Conn)
	}
	if captured.Mode != store.ModeProxy || captured.SocksPort != socksPort {
		t.Errorf("ConnConfig wrong: %+v", *captured)
	}
	if len(captured.XrayJSON) == 0 {
		t.Error("ConnConfig.XrayJSON should be populated")
	}
	// The last emitted state event should reflect connected.
	last := em.events[len(em.events)-1]
	if last.name != "state" {
		t.Fatalf("last event = %q, want state", last.name)
	}
	if dto, ok := last.data.(StateDTO); !ok || dto.Conn != string(ConnConnected) {
		t.Errorf("emitted state Conn wrong: %+v", last.data)
	}
}

func TestConnectWithoutActiveServerErrors(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.Connect(); err == nil {
		t.Error("Connect with no active server should error")
	}
	if svc.GetState().Conn != string(ConnDisconnected) {
		t.Error("failed Connect must remain disconnected")
	}
}

func TestConnectFactoryFailureGoesToError(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	mustAdd(t, svc, sampleLink)
	_ = svc.UpdateSettings(SettingsDTO{Mode: "proxy"})
	svc.deps.Factory = func(ConnConfig) (Connector, error) {
		return nil, errors.New("boom")
	}
	if err := svc.Connect(); err == nil {
		t.Fatal("expected Connect to fail")
	}
	st := svc.GetState()
	if st.Conn != string(ConnError) {
		t.Errorf("Conn = %q, want error", st.Conn)
	}
	if st.LastError == "" {
		t.Error("LastError should be set on failure")
	}
}

func TestConnectTUNModeRejectedThisPhase(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	mustAdd(t, svc, sampleLink)
	// state default Mode is tun; do not switch.
	if err := svc.Connect(); err == nil {
		t.Error("TUN mode should be rejected in Phase 3")
	}
}

func TestDisconnectStopsConnector(t *testing.T) {
	svc, _, fc, _ := connectReadyService(t)
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := svc.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if !fc.stopped {
		t.Error("connector should have been stopped")
	}
	if svc.GetState().Conn != string(ConnDisconnected) {
		t.Errorf("Conn = %q, want disconnected", svc.GetState().Conn)
	}
}

func TestConnectWhenAlreadyConnectedErrors(t *testing.T) {
	svc, _, _, _ := connectReadyService(t)
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := svc.Connect(); err == nil {
		t.Error("second Connect while connected should error")
	}
}

func TestLogsReturnsBufferedLines(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	svc.bus.Append("hello")
	got := svc.Logs()
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("Logs = %v, want [hello]", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run "Connect|Disconnect|Logs" -v`
Expected: FAIL — `undefined: (*Service).Connect`.

- [ ] **Step 3: Implement connect/disconnect/logs**

`internal/app/connect.go`:
```go
package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zki/vless-client/internal/logbus"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/xrayconf"
)

// xrayLogPath is where xray writes its error log, tailed into the log bus.
func (s *Service) xrayLogPath() string {
	return filepath.Join(s.deps.LogDir, "xray.log")
}

// setConn updates state + lastError and emits. Caller must hold s.mu.
func (s *Service) setConn(c ConnState, errMsg string) {
	s.conn = c
	s.lastError = errMsg
	s.emitState()
}

// Connect builds the config from the active server + profile and starts the
// capture session. Proxy mode only this phase.
func (s *Service) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == ConnConnected || s.conn == ConnConnecting {
		return fmt.Errorf("already %s", s.conn)
	}
	if s.state.ActiveServer < 0 || s.state.ActiveServer >= len(s.state.Servers) {
		return fmt.Errorf("no active server selected")
	}
	mode := s.state.Settings.Mode
	if mode != store.ModeProxy {
		return fmt.Errorf("mode %q is not available yet (Phase 3 supports proxy mode only)", mode)
	}

	srv := s.state.Servers[s.state.ActiveServer]
	s.bus.Append(fmt.Sprintf("connecting to %s (%s:%d)", srv.Name, srv.Host, srv.Port))
	s.setConn(ConnConnecting, "")

	cfgJSON, err := xrayconf.Build(srv, s.state.Profile, xrayconf.Options{
		SocksPort: socksPort,
		LogFile:   s.xrayLogPath(),
	})
	if err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: build config: " + err.Error())
		return fmt.Errorf("build config: %w", err)
	}

	conn, err := s.deps.Factory(ConnConfig{
		XrayJSON:  cfgJSON,
		SocksHost: "127.0.0.1",
		SocksPort: socksPort,
		Mode:      mode,
	})
	if err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: " + err.Error())
		return err
	}
	if err := conn.Start(); err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: start: " + err.Error())
		return err
	}

	s.connector = conn
	s.startTailing()
	s.bus.Append("connected")
	s.setConn(ConnConnected, "")
	return nil
}

// Disconnect stops the capture session.
func (s *Service) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connector == nil {
		s.setConn(ConnDisconnected, "")
		return nil
	}
	s.setConn(ConnDisconnecting, "")
	err := s.connector.Stop()
	s.connector = nil
	s.stopTailing()
	if err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: stop: " + err.Error())
		return err
	}
	s.bus.Append("disconnected")
	s.setConn(ConnDisconnected, "")
	return nil
}

// startTailing launches the xray log tailer. Caller must hold s.mu.
func (s *Service) startTailing() {
	if s.tailStop != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.tailStop = cancel
	go logbus.TailFile(ctx, s.xrayLogPath(), s.bus, 500*time.Millisecond)
}

// stopTailing stops the tailer. Caller must hold s.mu.
func (s *Service) stopTailing() {
	if s.tailStop != nil {
		s.tailStop()
		s.tailStop = nil
	}
}

// Logs returns the buffered log lines for the initial UI paint.
func (s *Service) Logs() []string {
	return s.bus.Lines()
}

// SubscribeLogs forwards every new log line to fn until the returned cancel is
// called. The Wails layer uses this to emit "log" events.
func (s *Service) SubscribeLogs(fn func(string)) (cancel func()) {
	return s.bus.Subscribe(fn)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app/ -v && gofmt -l internal/app/ && go vet ./internal/app/`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/connect.go internal/app/connect_test.go
git commit -m "feat(app): connection state machine, connect/disconnect, log streaming"
```

---

### Task 8: app — auto-connect on startup

**Files:**
- Create: `internal/app/autoconnect.go`
- Test: `internal/app/autoconnect_test.go`

If `AutoConnect` is set and an active server exists, connect on launch. Errors are swallowed (logged to the bus, surfaced via state) so a failed auto-connect never blocks startup.

- [ ] **Step 1: Write the failing test**

`internal/app/autoconnect_test.go`:
```go
package app

import "testing"

func TestMaybeAutoConnectConnectsWhenEnabled(t *testing.T) {
	svc, _, fc, _ := connectReadyService(t)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoConnect: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	svc.MaybeAutoConnect()
	if !fc.started {
		t.Error("auto-connect should have started the connector")
	}
	if svc.GetState().Conn != string(ConnConnected) {
		t.Error("auto-connect should reach connected")
	}
}

func TestMaybeAutoConnectSkipsWhenDisabled(t *testing.T) {
	svc, _, fc, _ := connectReadyService(t)
	// AutoConnect defaults to false.
	svc.MaybeAutoConnect()
	if fc.started {
		t.Error("auto-connect should not run when disabled")
	}
}

func TestMaybeAutoConnectSkipsWithoutActiveServer(t *testing.T) {
	svc, _, fc, _ := testDeps(t)
	_ = svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoConnect: true})
	svc.MaybeAutoConnect() // no servers added
	if fc.started {
		t.Error("auto-connect should not run without an active server")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run AutoConnect -v`
Expected: FAIL — `undefined: (*Service).MaybeAutoConnect`.

- [ ] **Step 3: Implement auto-connect**

`internal/app/autoconnect.go`:
```go
package app

// MaybeAutoConnect connects on startup when AutoConnect is enabled and an
// active server is configured. A connection failure is non-fatal: it is
// recorded in state/logs (via Connect) and otherwise ignored.
func (s *Service) MaybeAutoConnect() {
	s.mu.Lock()
	enabled := s.state.Settings.AutoConnect
	hasActive := s.state.ActiveServer >= 0 && s.state.ActiveServer < len(s.state.Servers)
	s.mu.Unlock()

	if !enabled || !hasActive {
		return
	}
	_ = s.Connect()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app/ -v && gofmt -l internal/app/ && go vet ./internal/app/`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/autoconnect.go internal/app/autoconnect_test.go
git commit -m "feat(app): auto-connect on startup"
```

---

### Task 9: Wails scaffold + build-tag isolation

**Files:**
- Create: `wails.json`
- Create: `main_stub.go`
- Create: `frontend/` (via `wails init`)
- Modify: `go.mod`/`go.sum`

Initialize the Wails project structure in the repo root and isolate all GUI Go files behind a `wails` build tag so the default `go build ./...` / `go test ./...` (used on this CI/dev box, which lacks webkit) stays green. The real GUI is built with `wails build -tags wails`.

- [ ] **Step 1: Install the Wails CLI and scaffold a throwaway template**

Run:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
export PATH="$PATH:$(go env GOPATH)/bin"
wails version
cd /tmp && wails init -n vlessgui -t vanilla-ts && cd -
```
Expected: `wails version` prints a v2.x version. The throwaway scaffold in `/tmp/vlessgui` is a reference for the files we copy in (we do NOT init in-repo because that would overwrite `go.mod`/main). If `@latest` fails on Go 1.26, install a pinned recent v2 tag and record which.

- [ ] **Step 2: Copy the frontend skeleton into the repo**

```bash
mkdir -p /home/zki/projects/vless-client/frontend
cp -r /tmp/vlessgui/frontend/* /home/zki/projects/vless-client/frontend/
```
This brings in `frontend/package.json`, `frontend/index.html`, `frontend/src/`, `frontend/vite.config.ts`, `frontend/tsconfig.json`. (We replace `index.html` and `src/main.ts` content in Task 11.) Confirm `frontend/package.json` exists.

- [ ] **Step 3: Write wails.json**

`wails.json` (repo root):
```json
{
  "$schema": "https://wails.io/schemas/config.v2.json",
  "name": "vless-client",
  "outputfilename": "vless-client",
  "frontend:install": "npm install",
  "frontend:build": "npm run build",
  "frontend:dev:watcher": "npm run dev",
  "frontend:dev:serverUrl": "auto",
  "author": {
    "name": "zki"
  }
}
```

- [ ] **Step 4: Write the non-GUI stub main**

The repo root must contain a buildable `package main` for the default (no-tag) build, since the real GUI `main` is tag-gated.

`main_stub.go` (repo root):
```go
//go:build !wails

package main

import "fmt"

// This binary is the GUI entrypoint, but the GUI is only compiled with the
// "wails" build tag (which pulls in webkit/cgo). Build it with:
//
//	wails build -tags wails     # production bundle
//	wails dev   -tags wails     # live-reload dev
//
// The headless CLI lives in ./cmd/headless and needs no GUI toolchain.
func main() {
	fmt.Println("Build the GUI with: wails build -tags wails (or wails dev -tags wails)")
	fmt.Println("Headless CLI: go run ./cmd/headless -link 'vless://...'")
}
```

- [ ] **Step 5: Add the Wails dependency to go.mod**

```bash
go get github.com/wailsapp/wails/v2@latest
go mod tidy
```
Expected: `github.com/wailsapp/wails/v2` added to `go.mod`. (`go mod tidy` keeps it even though it is only referenced from tag-gated files, because the tag-off build still has the import graph computed lazily — if tidy drops it, add a tools-style blank import under a `//go:build wails` file, which Tasks 10 provide anyway.)

- [ ] **Step 6: Verify default build is still green**

Run:
```bash
go build ./...
go test ./...
gofmt -l . && go vet ./...
```
Expected: builds clean (root `main` = stub only), all tests pass, no fmt/vet output. The `frontend/` dir contains no Go files so it does not affect `go build ./...`.

- [ ] **Step 7: Commit**

```bash
git add wails.json main_stub.go frontend/ go.mod go.sum .gitignore
git commit -m "chore(gui): scaffold Wails project, isolate GUI behind wails build tag"
```
Note: before committing, add `frontend/node_modules/` and `build/bin/` to `.gitignore` if not already present.

---

### Task 10: Wails binding — App struct, emitter, proxy connector

**Files:**
- Create: `main.go` (`//go:build wails`)
- Create: `gui_app.go` (`//go:build wails`)
- Create: `gui_connector.go` (`//go:build wails`)

The thin platform layer: builds the real proxy-mode connector (`tunnel` + `sysproxy` + `core`), wraps `runtime.EventsEmit` as an `app.Emitter`, constructs the service on startup, forwards log lines as `log` events, and exposes the service methods Wails binds for the frontend.

- [ ] **Step 1: Write the connector factory**

`gui_connector.go`:
```go
//go:build wails

package main

import (
	"fmt"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/core"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
	"github.com/zki/vless-client/internal/tunnel"
)

// coreAdapter adapts core.Start to the tunnel.Core interface.
type coreAdapter struct{}

func (coreAdapter) Start(jsonConfig []byte) (tunnel.Stopper, error) {
	return core.Start(jsonConfig)
}

// newConnector builds a proxy-mode capture session. TUN is rejected here
// (Phase 4 wires netcfg/tun); the app layer also guards against it.
func newConnector(c app.ConnConfig) (app.Connector, error) {
	if c.Mode != store.ModeProxy {
		return nil, fmt.Errorf("mode %q not supported in this build (proxy only)", c.Mode)
	}
	return tunnel.New(tunnel.Config{
		XrayJSON:  c.XrayJSON,
		SocksHost: c.SocksHost,
		SocksPort: c.SocksPort,
		Mode:      store.ModeProxy,
	}, tunnel.Deps{
		Core:  coreAdapter{},
		Proxy: sysproxy.New(),
		// Tun/Router unused in proxy mode (Phase 4).
	}), nil
}
```

- [ ] **Step 2: Write the App binding**

`gui_app.go`:
```go
//go:build wails

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/store"
)

// wailsEmitter implements app.Emitter via the Wails runtime.
type wailsEmitter struct{ ctx context.Context }

func (e wailsEmitter) Emit(event string, data any) {
	wruntime.EventsEmit(e.ctx, event, data)
}

// App is the Wails-bound application object.
type App struct {
	ctx context.Context
	svc *app.Service
}

func NewApp() *App { return &App{} }

// startup runs once the Wails runtime context is ready. It builds the service,
// wires log streaming to "log" events, and performs auto-connect.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	statePath, err := store.DefaultPath()
	if err != nil {
		log.Printf("config path: %v", err)
		statePath = "state.json"
	}
	logDir := filepath.Dir(statePath)
	_ = os.MkdirAll(logDir, 0o755)

	svc, err := app.New(app.Deps{
		StatePath: statePath,
		LogDir:    logDir,
		Emitter:   wailsEmitter{ctx},
		Factory:   newConnector,
		Elevated:  privilege.IsElevated,
	})
	if err != nil {
		log.Printf("init service: %v", err)
		return
	}
	a.svc = svc
	a.svc.SubscribeLogs(func(line string) {
		wruntime.EventsEmit(a.ctx, "log", line)
	})
	a.svc.MaybeAutoConnect()
}

// --- Methods bound to the frontend ---

func (a *App) GetState() app.StateDTO            { return a.svc.GetState() }
func (a *App) AddServer(link string) error       { return a.svc.AddServer(link) }
func (a *App) RemoveServer(index int) error      { return a.svc.RemoveServer(index) }
func (a *App) SetActiveServer(index int) error   { return a.svc.SetActiveServer(index) }
func (a *App) UpdateProfile(p app.ProfileDTO) error   { return a.svc.UpdateProfile(p) }
func (a *App) UpdateSettings(s app.SettingsDTO) error { return a.svc.UpdateSettings(s) }
func (a *App) Connect() error                    { return a.svc.Connect() }
func (a *App) Disconnect() error                 { return a.svc.Disconnect() }
func (a *App) Logs() []string                    { return a.svc.Logs() }
```

- [ ] **Step 3: Write main.go**

`main.go`:
```go
//go:build wails

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := NewApp()
	err := wails.Run(&options.App{
		Title:  "VLESS Client",
		Width:  900,
		Height: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: a.startup,
		Bind:      []any{a},
	})
	if err != nil {
		panic(err)
	}
}
```

- [ ] **Step 4: Verify the tagged build compiles (best effort on this box)**

Run:
```bash
go vet -tags wails ./... 2>&1 | head -40
```
Expected on a machine **with** webkit2gtk + gcc: clean. On WSL2 **without** webkit, expect cgo/link errors from the Wails import only — that is an environment limitation, not a code error. To still type-check our code against the Wails API, confirm the import paths/signatures with:
```bash
go doc github.com/wailsapp/wails/v2/pkg/runtime.EventsEmit
go doc github.com/wailsapp/wails/v2/pkg/options.App
go doc github.com/wailsapp/wails/v2.Run
```
Adapt `options.App` field names (`Bind`, `OnStartup`, `AssetServer`) to the resolved Wails version if they differ, and note any change. The default (untagged) build/test MUST remain green:
```bash
go build ./... && go test ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go gui_app.go gui_connector.go
git commit -m "feat(gui): Wails binding, emitter, proxy-mode connector"
```

---

### Task 11: Frontend — vanilla-TS UI

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.ts`
- Modify: `frontend/src/style.css`

A single-window UI with four sections (Status, Servers, Routing, Logs) driven by the bound `App` methods and the `state`/`log` events. No framework — direct DOM updates keep the code fully inspectable.

> The Wails JS bindings (`frontend/wailsjs/go/main/App.js` and `frontend/wailsjs/runtime`) are **generated** by `wails dev`/`wails build`/`wails generate module`; they do not exist until then. The import paths below match Wails' generated layout.

- [ ] **Step 1: Replace index.html**

`frontend/index.html`:
```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>VLESS Client</title>
  </head>
  <body>
    <div id="app">
      <header>
        <h1>VLESS Client</h1>
        <div id="status-pill" class="pill disconnected">disconnected</div>
      </header>

      <section id="status">
        <p>Active server: <span id="active-server">—</span></p>
        <button id="connect-btn">Connect</button>
        <button id="disconnect-btn">Disconnect</button>
        <p id="error-line" class="error"></p>
      </section>

      <section id="servers">
        <h2>Servers</h2>
        <div class="row">
          <input id="link-input" type="text" placeholder="vless://..." />
          <button id="add-server-btn">Add</button>
        </div>
        <p id="link-error" class="error"></p>
        <ul id="server-list"></ul>
      </section>

      <section id="routing">
        <h2>Routing</h2>
        <label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
        <label><input type="checkbox" id="ru-toggle" /> Force RU direct</label>
        <div class="row">
          <label>Mode:
            <select id="mode-select">
              <option value="proxy">Proxy (system SOCKS)</option>
              <option value="tun">TUN (Phase 4 — disabled)</option>
            </select>
          </label>
        </div>
      </section>

      <section id="logs">
        <h2>Logs</h2>
        <button id="clear-logs-btn">Clear</button>
        <pre id="log-view"></pre>
      </section>
    </div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 2: Replace main.ts**

`frontend/src/main.ts`:
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

function render(st: State) {
  const pill = $("status-pill");
  pill.textContent = st.conn;
  pill.className = "pill " + st.conn;

  $("active-server").textContent =
    st.activeServer >= 0 && st.servers[st.activeServer]
      ? st.servers[st.activeServer].name
      : "—";
  $("error-line").textContent = st.lastError || "";

  const list = $("server-list");
  list.innerHTML = "";
  st.servers.forEach((s, i) => {
    const li = document.createElement("li");
    li.className = i === st.activeServer ? "active" : "";
    li.innerHTML = `<span>${s.name} (${s.host}:${s.port})</span>`;
    const sel = document.createElement("button");
    sel.textContent = "Select";
    sel.onclick = () => SetActiveServer(i);
    const del = document.createElement("button");
    del.textContent = "Delete";
    del.onclick = () => RemoveServer(i);
    li.append(sel, del);
    list.append(li);
  });

  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  (<HTMLSelectElement>$("mode-select")).value = st.settings.mode;
}

let current: State;

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

function wire() {
  $("connect-btn").addEventListener("click", () => {
    Connect().catch((e) => ($("error-line").textContent = String(e)));
  });
  $("disconnect-btn").addEventListener("click", () => {
    Disconnect().catch((e) => ($("error-line").textContent = String(e)));
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
    UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
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

- [ ] **Step 3: Replace style.css**

`frontend/src/style.css`:
```css
:root { font-family: system-ui, sans-serif; color: #eee; background: #1e1e22; }
body { margin: 0; }
#app { padding: 1rem 1.25rem; max-width: 820px; margin: 0 auto; }
header { display: flex; align-items: center; gap: 1rem; }
h1 { font-size: 1.2rem; }
h2 { font-size: 1rem; margin: 1.2rem 0 0.4rem; border-bottom: 1px solid #444; }
.pill { padding: 0.15rem 0.6rem; border-radius: 1rem; font-size: 0.8rem; text-transform: uppercase; }
.pill.connected { background: #1f7a3d; }
.pill.connecting, .pill.disconnecting { background: #8a6d00; }
.pill.disconnected { background: #555; }
.pill.error { background: #9a2222; }
.row { display: flex; gap: 0.5rem; align-items: center; margin: 0.4rem 0; }
input[type="text"] { flex: 1; padding: 0.35rem; background: #2a2a30; border: 1px solid #444; color: #eee; }
button { padding: 0.35rem 0.7rem; background: #33333a; color: #eee; border: 1px solid #555; cursor: pointer; }
button:hover { background: #44444c; }
ul { list-style: none; padding: 0; }
li { display: flex; align-items: center; gap: 0.5rem; padding: 0.25rem 0; }
li.active span { font-weight: bold; color: #6fcf97; }
li span { flex: 1; }
.error { color: #ff8080; min-height: 1em; font-size: 0.85rem; }
label { display: block; margin: 0.3rem 0; }
#log-view { background: #111; padding: 0.5rem; height: 220px; overflow-y: auto; font-size: 0.78rem; white-space: pre-wrap; }
```

- [ ] **Step 4: Generate bindings and type-check the frontend**

Run:
```bash
export PATH="$PATH:$(go env GOPATH)/bin"
cd /home/zki/projects/vless-client
wails generate module -tags wails    # generates frontend/wailsjs from bound App
cd frontend && npm install && npx tsc --noEmit && cd ..
```
Expected: `wails generate module` writes `frontend/wailsjs/go/main/App.{js,d.ts}` and `frontend/wailsjs/runtime/`. `tsc --noEmit` reports no type errors. If `wails generate module` requires the cgo toolchain and fails on this box, this step is deferred to a dev machine with webkit; record that, and at minimum run `npx tsc --noEmit` after the bindings exist. The generated DTO names may be namespaced (e.g. `app.StateDTO`); if so, adjust the `import` in `main.ts` to the generated path and note it.

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html frontend/src/main.ts frontend/src/style.css
git commit -m "feat(gui): vanilla-TS frontend (status/servers/routing/logs)"
```

---

### Task 12: Manual run, build notes, and Definition of Done

**Files:**
- Create: `docs/superpowers/phase3-build-notes.md`

- [ ] **Step 1: Manual GUI run (dev machine with webkit2gtk + gcc, or Win/mac)**

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
cd /home/zki/projects/vless-client
wails dev -tags wails
```
Expected: a window opens. Verify, with a real disposable `vless://` link:
1. Paste link → Add → server appears and becomes active.
2. Mode = Proxy → Connect → status pill goes `connecting` → `connected`; the Logs pane shows "connecting…", "connected", and xray's own log lines (tailed from `xray.log`).
3. Telegram reachable while connected (check Telegram Desktop / `curl` through the system SOCKS proxy); RU/other traffic unaffected.
4. Disconnect → pill returns to `disconnected`; system proxy cleared.
5. Selecting Mode = TUN then Connect → status `error` with the "proxy mode only" message (TUN is Phase 4).
6. Toggle Telegram off → Connect again → Telegram no longer routed (rebuild reflects profile).

(On WSL2 without webkit, `wails dev` will not run — perform this on Windows/macOS or a Linux desktop. This mirrors Phase 2's manual platform verification.)

- [ ] **Step 2: Production build sanity (per target OS)**

On each target dev machine:
```bash
wails build -tags wails
```
Expected: a bundle under `build/bin/` (`.app` on macOS, `.exe` on Windows, ELF on Linux). Record which OSes were actually built.

- [ ] **Step 3: Write the build notes**

`docs/superpowers/phase3-build-notes.md`:
```markdown
# Phase 3 build/verification notes

## What shipped
- `internal/logbus`: capped fan-out line buffer + poll-based file tailer (unit-tested).
- `internal/app`: GUI service layer — state machine (disconnected/connecting/
  connected/disconnecting/error), server CRUD, profile/settings updates,
  connect/disconnect, log streaming, auto-connect. Fully unit-tested with fakes.
- `internal/xrayconf`: optional `LogFile` (xray error log → tailable file).
- Wails GUI (`main.go`, `gui_app.go`, `gui_connector.go`, `frontend/`), all Go
  GUI files behind `//go:build wails`. Frontend: vanilla TS.

## Build model
- Default build/test (no toolchain beyond Go): `go build ./... && go test ./...`
  is green; root `main` is the stub (`main_stub.go`).
- GUI build: `wails build -tags wails` / `wails dev -tags wails` (needs Node +
  webkit2gtk on Linux, WebView2 on Windows, native WebKit on macOS).
- Connector is **proxy mode only** this phase (cross-platform). TUN selection is
  rejected at Connect with a clear message.

## Verified
| Item | Linux dev (this box) | Win/mac dev |
|---|---|---|
| `go test ./...` (logbus, app, xrayconf, prior) | PASS | — |
| `go build ./...` default | PASS | — |
| `wails build -tags wails` | needs webkit (note env) | <fill on run> |
| Manual GUI flow (add/connect/route/logs/disconnect) | <fill> | <fill> |

## Deferred to Phase 4 (networking)
- netcfg for darwin/windows; full default-route TUN with loop avoidance.
- Wire TUN mode into the GUI connector (remove the proxy-only guard).
- Supervise tun2socks `engine.Start/Stop` `log.Fatalf` (GUI must not be killed).
- Kill switch.

## Deferred to Phase 5
- Autostart (Win Run key / macOS LaunchAgent) — `Settings.AutoStart` UI is present but inert.
- Ping/latency test per server.
- Traffic stats (xray StatsService) — up/down speed indicator.
```

- [ ] **Step 4: Reconcile the table and commit**

Fill the table from actual runs, then:
```bash
git add docs/superpowers/phase3-build-notes.md
git commit -m "docs: phase 3 build matrix, manual verification, deferred work"
```

---

## Phase 3 Done — Definition of Done

- `go test ./...` green on the default build (logbus, app, xrayconf, and all prior packages).
- `go build ./...` green with the stub `main` (no GUI toolchain required).
- `internal/app` state machine, server/profile/settings management, connect/disconnect, log streaming, and auto-connect are unit-tested with fakes (no platform deps imported by `internal/app`).
- The Wails GUI builds with `wails build -tags wails` on at least one target OS and runs the full flow: add server → connect (proxy) → Telegram routed → live logs → disconnect.
- TUN selection is cleanly rejected with a Phase-4 message (no crash, state → error).
- Build matrix and manual verification recorded in `docs/superpowers/phase3-build-notes.md`.

## What Phase 3 deliberately leaves out

- **Phase 4 (networking):** netcfg darwin/windows, full default-route TUN with loop avoidance, TUN wired into the GUI, tun2socks `log.Fatalf` supervision, kill switch.
- **Phase 5:** autostart, ping/latency test, traffic stats (up/down speed).
- Installers, code-signing/notarization (manual install in v1, per spec).
