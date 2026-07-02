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

// fullSampleLink uses a literal IP so resolveServerIPv4 is deterministic without DNS.
const fullSampleLink = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@203.0.113.7:443?security=reality&pbk=x&sni=example.com&type=tcp&fp=chrome#full"

func TestConnectTUNFullPopulatesConfig(t *testing.T) {
	svc, _, fc, captured := testDeps(t) // testDeps is elevated
	mustAdd(t, svc, fullSampleLink)
	if err := svc.UpdateProfile(ProfileDTO{Full: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !fc.started {
		t.Fatal("connector should have started")
	}
	if !captured.FullTunnel {
		t.Error("ConnConfig.FullTunnel should be true in full TUN mode")
	}
	if !captured.BlockIPv6 {
		t.Error("ConnConfig.BlockIPv6 should be true in full TUN mode")
	}
	if len(captured.ServerIPs) == 0 {
		t.Error("ConnConfig.ServerIPs should be populated from the server host")
	}
	if len(captured.RouteCIDRs) != 0 {
		t.Error("full mode should clear selective RouteCIDRs")
	}
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

// bindDeps is testDeps plus a DefaultInterface stub returning name.
func bindDeps(t *testing.T, name string) (*Service, *ConnConfig) {
	t.Helper()
	svc, _, _, captured := testDeps(t)
	svc.deps.DefaultInterface = func() (string, error) { return name, nil }
	return svc, captured
}

func TestConnectTUNWhitelistFullCaptureWithBind(t *testing.T) {
	svc, captured := bindDeps(t, "eth0")
	mustAdd(t, svc, fullSampleLink)
	if err := svc.UpdateProfile(ProfileDTO{Telegram: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !captured.FullTunnel {
		t.Error("whitelist + bind interface must use full-capture (FullTunnel)")
	}
	if captured.BlockIPv6 {
		t.Error("whitelist mode must not block IPv6")
	}
	if len(captured.ServerIPs) == 0 {
		t.Error("ServerIPs bypass should be populated")
	}
	// Kill-switch CIDR list survives the capture-model switch.
	if !sliceContains(captured.RouteCIDRs, routing.TelegramCIDRs[0]) {
		t.Errorf("RouteCIDRs missing telegram CIDRs: %v", captured.RouteCIDRs)
	}
	if !strings.Contains(string(captured.XrayJSON), `"interface":"eth0"`) {
		t.Errorf("xray JSON missing sockopt interface binding: %s", captured.XrayJSON)
	}
}

func TestConnectTUNFullWithBindBlocksIPv6(t *testing.T) {
	svc, captured := bindDeps(t, "eth0")
	mustAdd(t, svc, fullSampleLink)
	if err := svc.UpdateProfile(ProfileDTO{Full: true}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !captured.FullTunnel || !captured.BlockIPv6 {
		t.Errorf("full + bind: FullTunnel=%v BlockIPv6=%v, want true/true",
			captured.FullTunnel, captured.BlockIPv6)
	}
}

func TestConnectProxyModeSkipsBind(t *testing.T) {
	svc, _, _, captured := connectReadyService(t)
	svc.deps.DefaultInterface = func() (string, error) {
		t.Error("DefaultInterface must not be called in proxy mode")
		return "", nil
	}
	if err := svc.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if strings.Contains(string(captured.XrayJSON), "sockopt") {
		t.Error("proxy mode must not bind outbounds to an interface")
	}
}

func TestUpdateProfileValidation(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.UpdateProfile(ProfileDTO{CustomProxyIPs: []string{"not-an-ip"}}); err == nil {
		t.Error("bad IP must be rejected")
	}
	if err := svc.UpdateProfile(ProfileDTO{CustomDirectDomains: []string{"http://x.com"}}); err == nil {
		t.Error("URL as domain must be rejected")
	}
	if err := svc.UpdateProfile(ProfileDTO{ProxyPresets: []string{"nope"}}); err == nil {
		t.Error("unknown preset must be rejected")
	}
	err := svc.UpdateProfile(ProfileDTO{
		ProxyPresets:        []string{"youtube"},
		CustomProxyDomains:  []string{" geosite:google ", ""},
		CustomProxyIPs:      []string{"1.2.3.4", "10.0.0.0/8", "2a0a:f280::/32"},
		CustomDirectDomains: []string{"sberbank.ru"},
		CustomDirectIPs:     []string{"77.88.8.8"},
	})
	if err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	p := svc.GetState().Profile
	if len(p.CustomProxyDomains) != 1 || p.CustomProxyDomains[0] != "geosite:google" {
		t.Errorf("lists not cleaned: %+v", p.CustomProxyDomains)
	}
}
