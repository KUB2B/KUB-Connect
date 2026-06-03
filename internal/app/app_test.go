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
