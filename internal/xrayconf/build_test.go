package xrayconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

func sampleServer() *vless.ServerConfig {
	return &vless.ServerConfig{
		Name: "srv", Host: "example.com", Port: 443,
		UUID:     "b831381d-6324-4d53-ad4f-8cda48b30811",
		Flow:     "xtls-rprx-vision",
		Security: vless.SecurityReality,
		Network:  vless.NetworkTCP,
		SNI:      "www.microsoft.com", Fingerprint: "chrome",
		PublicKey: "ABCpublicKey", ShortID: "0123abcd", SpiderX: "/",
	}
}

func TestBuildGolden(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var pretty []byte
	pretty, err = json.MarshalIndent(json.RawMessage(got), "", "  ")
	if err != nil {
		t.Fatalf("indent: %v", err)
	}
	pretty = append(pretty, '\n')

	goldenPath := filepath.Join("testdata", "reality_default.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, pretty, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 first): %v", err)
	}
	if string(pretty) != string(want) {
		t.Errorf("config mismatch.\n--- got ---\n%s\n--- want ---\n%s", pretty, want)
	}
}

func TestBuildLoopGuardBlocksTUNSubnetFirst(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var cfg struct {
		Routing struct {
			Rules []ruleJSON `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(got, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Routing.Rules) == 0 {
		t.Fatal("no routing rules")
	}
	first := cfg.Routing.Rules[0]
	if first.OutboundTag != routing.OutboundBlock {
		t.Errorf("first rule outbound = %q, want %q (loop guard must win)", first.OutboundTag, routing.OutboundBlock)
	}
	found := false
	for _, ip := range first.IP {
		if ip == routing.TUNReservedCIDR {
			found = true
		}
	}
	if !found {
		t.Errorf("first rule IPs = %v, want to include %q", first.IP, routing.TUNReservedCIDR)
	}
}

func TestBuildMuxDropsVisionFlowAndEnablesMux(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808, Mux: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var cfg struct {
		Outbounds []struct {
			Tag      string `json:"tag"`
			Mux      *struct {
				Enabled     bool `json:"enabled"`
				Concurrency int  `json:"concurrency"`
			} `json:"mux"`
			Settings struct {
				Vnext []struct {
					Users []struct {
						Flow string `json:"flow"`
					} `json:"users"`
				} `json:"vnext"`
			} `json:"settings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(got, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var proxy *struct {
		Tag string `json:"tag"`
		Mux *struct {
			Enabled     bool `json:"enabled"`
			Concurrency int  `json:"concurrency"`
		} `json:"mux"`
		Settings struct {
			Vnext []struct {
				Users []struct {
					Flow string `json:"flow"`
				} `json:"users"`
			} `json:"vnext"`
		} `json:"settings"`
	}
	for i := range cfg.Outbounds {
		if cfg.Outbounds[i].Tag == "proxy" {
			proxy = &cfg.Outbounds[i]
		}
	}
	if proxy == nil {
		t.Fatal("no proxy outbound")
	}
	if proxy.Mux == nil || !proxy.Mux.Enabled || proxy.Mux.Concurrency <= 0 {
		t.Errorf("mux not enabled with positive concurrency: %+v", proxy.Mux)
	}
	if got := proxy.Settings.Vnext[0].Users[0].Flow; got != "" {
		t.Errorf("vision flow must be dropped when mux is on, got %q", got)
	}
}

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

func TestBuildLoadsIntoXray(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !json.Valid(got) {
		t.Fatal("Build produced invalid JSON")
	}
}

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
