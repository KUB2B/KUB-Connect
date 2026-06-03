package xrayconf

import (
	"encoding/json"
	"os"
	"path/filepath"
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
