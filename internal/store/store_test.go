package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	in := &State{
		Servers: []*vless.ServerConfig{
			{Name: "a", Host: "h1", Port: 443, UUID: "u1", Security: vless.SecurityReality, Network: vless.NetworkTCP, PublicKey: "k"},
		},
		ActiveServer: 0,
		Profile:      routing.Default(),
		Settings:     Settings{Mode: ModeTUN, AutoStart: true, AutoConnect: false, KillSwitch: true},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out.Servers) != 1 || out.Servers[0].Host != "h1" {
		t.Errorf("servers round-trip failed: %+v", out.Servers)
	}
	if out.Profile.Telegram != true {
		t.Errorf("profile round-trip failed: %+v", out.Profile)
	}
	if out.Settings.Mode != ModeTUN || !out.Settings.KillSwitch {
		t.Errorf("settings round-trip failed: %+v", out.Settings)
	}
}

func TestNormalizeLogLevel(t *testing.T) {
	cases := map[string]string{
		"error":   "error",
		"warning": "warning",
		"debug":   "debug",
		"":        "warning",
		"bogus":   "warning",
		"info":    "warning",
	}
	for in, want := range cases {
		if got := NormalizeLogLevel(in); got != want {
			t.Errorf("NormalizeLogLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadMigratesMissingLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(`{"settings":{"mode":"tun"}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Settings.LogLevel != LogNormal {
		t.Errorf("LogLevel = %q, want %q", s.Settings.LogLevel, LogNormal)
	}
}

func TestDefaultStateLogLevel(t *testing.T) {
	if got := DefaultState().Settings.LogLevel; got != LogNormal {
		t.Errorf("default LogLevel = %q, want %q", got, LogNormal)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing should not error, got %v", err)
	}
	if out.Settings.Mode != ModeTUN {
		t.Errorf("default Mode = %q, want tun", out.Settings.Mode)
	}
	if !out.Profile.Telegram {
		t.Errorf("default profile should have Telegram on")
	}
}

func TestSaveLoadRoundTripPendingConnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	in := &State{
		ActiveServer:   -1,
		Profile:        routing.Default(),
		Settings:       Settings{Mode: ModeTUN, LogLevel: LogNormal},
		PendingConnect: true,
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !out.PendingConnect {
		t.Error("PendingConnect should survive round-trip")
	}
}

func TestDefaultStatePendingConnectFalse(t *testing.T) {
	if DefaultState().PendingConnect {
		t.Error("DefaultState PendingConnect should be false")
	}
}
