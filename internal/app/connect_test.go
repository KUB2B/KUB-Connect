package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zki/vless-client/internal/routing"
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

func TestConnectTUNHappyPath(t *testing.T) {
	svc, _, fc, captured := testDeps(t) // elevated, default Mode=tun
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateProfile(ProfileDTO{Telegram: true, CustomProxyIPs: []string{"203.0.113.0/24"}}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !fc.started {
		t.Error("connector should have been started")
	}
	if captured.Mode != store.ModeTUN {
		t.Errorf("Mode = %q, want tun", captured.Mode)
	}
	if captured.Device == "" || captured.TunIP == "" || captured.TunPrefix == 0 {
		t.Errorf("TUN params unset: %+v", *captured)
	}
	if !sliceContains(captured.RouteCIDRs, routing.TelegramCIDRs[0]) {
		t.Errorf("RouteCIDRs missing telegram CIDRs: %v", captured.RouteCIDRs)
	}
	if !sliceContains(captured.RouteCIDRs, "203.0.113.0/24") {
		t.Errorf("RouteCIDRs missing custom IP: %v", captured.RouteCIDRs)
	}
}

func TestConnectTUNPassesKillSwitch(t *testing.T) {
	svc, _, _, captured := testDeps(t) // elevated, default Mode=tun
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", KillSwitch: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.UpdateProfile(ProfileDTO{Telegram: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !captured.KillSwitch {
		t.Error("ConnConfig.KillSwitch should be true when setting is on in TUN mode")
	}
}

func TestConnectTUNDropsUnsupportedKillSwitch(t *testing.T) {
	dir := t.TempDir()
	var captured ConnConfig
	deps := Deps{
		StatePath:           filepath.Join(dir, "state.json"),
		LogDir:              dir,
		Emitter:             &fakeEmitter{},
		Elevated:            func() bool { return true },
		KillSwitchSupported: func() bool { return false },
		Factory: func(c ConnConfig) (Connector, error) {
			captured = c
			return &fakeConnector{}, nil
		},
	}
	svc, err := New(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", KillSwitch: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	// Connect must succeed (not abort) and silently drop the kill switch.
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if captured.KillSwitch {
		t.Error("ConnConfig.KillSwitch must be false when unsupported on this OS")
	}
}

func TestConnectProxyIgnoresKillSwitch(t *testing.T) {
	svc, _, _, captured := testDeps(t)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", KillSwitch: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if captured.KillSwitch {
		t.Error("ConnConfig.KillSwitch must be false in proxy mode")
	}
}

func TestConnectTUNRequiresElevation(t *testing.T) {
	svc, _, _, _ := testDepsElevation(t, false)
	mustAdd(t, svc, sampleLink)
	// default Mode=tun
	if err := svc.Connect(); err == nil {
		t.Error("TUN mode without elevation should error")
	}
	if svc.GetState().Conn != string(ConnDisconnected) {
		t.Error("failed Connect must remain disconnected")
	}
}

func TestConnectTUNExcludesTelegramWhenDisabled(t *testing.T) {
	svc, _, _, captured := testDeps(t)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateProfile(ProfileDTO{Telegram: false, CustomProxyIPs: []string{"203.0.113.0/24"}}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if sliceContains(captured.RouteCIDRs, routing.TelegramCIDRs[0]) {
		t.Errorf("telegram CIDRs should be absent when Telegram off: %v", captured.RouteCIDRs)
	}
	if !sliceContains(captured.RouteCIDRs, "203.0.113.0/24") {
		t.Errorf("custom IP should still be present: %v", captured.RouteCIDRs)
	}
}

func TestConnectTUNExcludesIPv6Routes(t *testing.T) {
	svc, _, _, captured := testDeps(t)
	mustAdd(t, svc, sampleLink)
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	for _, c := range captured.RouteCIDRs {
		if strings.Contains(c, ":") {
			t.Errorf("IPv6 CIDR %q routed into TUN; IPv6 must be excluded to halve the handshake storm: %v", c, captured.RouteCIDRs)
		}
	}
	// IPv4 telegram ranges must still be present.
	if !sliceContains(captured.RouteCIDRs, routing.TelegramCIDRs[0]) {
		t.Errorf("IPv4 telegram CIDRs missing: %v", captured.RouteCIDRs)
	}
}

func sliceContains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
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
