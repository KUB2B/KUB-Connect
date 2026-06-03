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
