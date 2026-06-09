package app

import (
	"path/filepath"
	"testing"

	"github.com/zki/vless-client/internal/store"
)

// fakeEmitter records emitted events for assertions.
type fakeEmitter struct {
	events []struct {
		name string
		data any
	}
}

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
	return testDepsElevation(t, true)
}

// testDepsElevation is testDeps with a configurable privilege stub.
func testDepsElevation(t *testing.T, elevated bool) (*Service, *fakeEmitter, *fakeConnector, *ConnConfig) {
	t.Helper()
	dir := t.TempDir()
	em := &fakeEmitter{}
	fc := &fakeConnector{}
	var captured ConnConfig
	deps := Deps{
		StatePath: filepath.Join(dir, "state.json"),
		LogDir:    dir,
		Emitter:   em,
		Elevated:  func() bool { return elevated },
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

func TestCapsAndTUNFallback(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	// Persist a state whose mode is TUN.
	st := store.DefaultState()
	st.Settings.Mode = store.ModeTUN
	if err := store.Save(statePath, st); err != nil {
		t.Fatal(err)
	}

	svc, err := New(Deps{
		StatePath:    statePath,
		TUNSupported: func() bool { return false },
		OS:           "darwin",
		Version:      "v9.9.9",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := svc.GetState()
	if got.Caps.TUNSupported {
		t.Error("Caps.TUNSupported = true, want false")
	}
	if got.Caps.OS != "darwin" {
		t.Errorf("Caps.OS = %q, want darwin", got.Caps.OS)
	}
	if got.Caps.Version != "v9.9.9" {
		t.Errorf("Caps.Version = %q, want v9.9.9", got.Caps.Version)
	}
	if got.Settings.Mode != string(store.ModeProxy) {
		t.Errorf("Settings.Mode = %q, want proxy (TUN should fall back)", got.Settings.Mode)
	}
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
