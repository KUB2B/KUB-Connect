package core

import (
	"runtime"
	"testing"
	"time"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
	"github.com/zki/vless-client/internal/xrayconf"
)

func TestStartStopSmoke(t *testing.T) {
	srv := &vless.ServerConfig{
		Name: "t", Host: "127.0.0.1", Port: 1,
		UUID: "b831381d-6324-4d53-ad4f-8cda48b30811", Flow: "xtls-rprx-vision",
		Security: vless.SecurityReality, Network: vless.NetworkTCP,
		// PublicKey must decode to 32 bytes (xray-core validates length at config
		// load). Dummy 32-byte base64url key, not a real server key.
		SNI: "www.microsoft.com", PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortID: "00", SpiderX: "/",
	}
	cfgJSON, err := xrayconf.Build(srv, routing.Default(), xrayconf.Options{SocksPort: 38080})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	before := runtime.NumGoroutine()

	inst, err := Start(cfgJSON)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+5 {
		t.Errorf("possible goroutine leak: before=%d after=%d", before, after)
	}
}

// TestStartFullCaptureConfig feeds xray-core the config generated for the
// TUN full-capture model: interface-bound outbounds (sockopt.interface),
// a service preset, and direct exceptions. Guards against emitting JSON that
// xray-core rejects at load time.
func TestStartFullCaptureConfig(t *testing.T) {
	srv := &vless.ServerConfig{
		Name: "t", Host: "127.0.0.1", Port: 1,
		UUID: "b831381d-6324-4d53-ad4f-8cda48b30811", Flow: "xtls-rprx-vision",
		Security: vless.SecurityReality, Network: vless.NetworkTCP,
		SNI: "www.microsoft.com", PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortID: "00", SpiderX: "/",
	}
	for _, full := range []bool{false, true} {
		profile := routing.Profile{
			Full:                full,
			Telegram:            true,
			ForceRUDirect:       true,
			ProxyPresets:        []string{"youtube"},
			CustomDirectDomains: []string{"gosuslugi.ru"},
			CustomDirectIPs:     []string{"77.88.8.8"},
		}
		cfgJSON, err := xrayconf.Build(srv, profile, xrayconf.Options{SocksPort: 38081, BindInterface: "lo"})
		if err != nil {
			t.Fatalf("full=%v Build: %v", full, err)
		}
		inst, err := Start(cfgJSON)
		if err != nil {
			t.Fatalf("full=%v Start: %v", full, err)
		}
		if err := inst.Stop(); err != nil {
			t.Fatalf("full=%v Stop: %v", full, err)
		}
	}
}
