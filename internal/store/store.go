package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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
	// PendingConnect is a one-shot intent set before an elevated restart so the
	// new instance auto-connects once, regardless of the AutoConnect setting.
	// Cleared after it is consumed on startup.
	PendingConnect bool `json:"pendingConnect"`
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

// Save atomically writes state to path as indented JSON, creating parent dirs.
// The temporary file lives beside the destination so Rename cannot cross a
// filesystem boundary; a crash therefore leaves either the old or new JSON.
func Save(path string, s *State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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
		// Preserve the unreadable bytes for diagnosis, but keep the GUI usable.
		// Save's atomic rename prevents new torn files; this also recovers files
		// left by older versions that wrote directly to state.json.
		backup := fmt.Sprintf("%s.corrupt-%s", path, time.Now().Format("20060102T150405.000000000"))
		_ = os.Rename(path, backup)
		return DefaultState(), nil
	}
	if s.Settings.Mode != ModeTUN && s.Settings.Mode != ModeProxy {
		s.Settings.Mode = ModeTUN
	}
	if len(s.Servers) == 0 {
		s.ActiveServer = -1
	} else if s.ActiveServer < 0 || s.ActiveServer >= len(s.Servers) {
		s.ActiveServer = 0
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
	return filepath.Join(dir, "kub-connect", "state.json"), nil
}
