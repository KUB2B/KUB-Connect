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
