package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

// Mode is the traffic capture mode.
type Mode string

const (
	ModeTUN   Mode = "tun"
	ModeProxy Mode = "proxy"
)

// Log verbosity levels. Values are xray-native (also valid tun2socks engine
// levels) so they map straight through with no translation.
const (
	LogQuiet   = "error"   // UI: Тихо
	LogNormal  = "warning" // UI: Обычный (default)
	LogVerbose = "debug"   // UI: Подробно
)

// NormalizeLogLevel returns level if supported, otherwise LogNormal. Empty or
// unknown values (including state files written before this field existed) fall
// back to warning.
func NormalizeLogLevel(level string) string {
	switch level {
	case LogQuiet, LogNormal, LogVerbose:
		return level
	default:
		return LogNormal
	}
}

// Settings holds app-level toggles.
type Settings struct {
	Mode        Mode `json:"mode"`
	AutoStart   bool `json:"autoStart"`
	AutoConnect bool `json:"autoConnect"`
	KillSwitch  bool `json:"killSwitch"`
	// Mux multiplexes proxied streams over few real connections to the server.
	// Tames the Telegram connection storm (dozens of parallel sockets). Requires
	// the server's client to be configured with no flow (it drops xtls-rprx-vision,
	// which is incompatible with mux). Off by default; safe only when the server
	// side matches.
	Mux bool `json:"mux"`
	// LogLevel is the xray-native log verbosity (error/warning/debug). See the
	// Log* constants. Drives both xray's log.loglevel and the tun2socks engine.
	LogLevel string `json:"logLevel"`
}

// State is the full persisted application state.
type State struct {
	Servers      []*vless.ServerConfig `json:"servers"`
	ActiveServer int                   `json:"activeServer"`
	Profile      routing.Profile       `json:"profile"`
	Settings     Settings              `json:"settings"`
}

// DefaultState returns the initial state for a fresh install.
func DefaultState() *State {
	return &State{
		Servers:      nil,
		ActiveServer: -1,
		Profile:      routing.Default(),
		Settings:     Settings{Mode: ModeTUN, LogLevel: LogNormal},
	}
}

// Save writes state to path as indented JSON, creating parent dirs.
func Save(path string, s *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads state from path. A missing file yields DefaultState (no error).
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultState(), nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.Settings.LogLevel = NormalizeLogLevel(s.Settings.LogLevel)
	return &s, nil
}

// DefaultPath returns the OS-appropriate state file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vless-client", "state.json"), nil
}
