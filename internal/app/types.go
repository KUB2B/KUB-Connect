// Package app is the GUI-facing service layer: it owns application state, the
// connection state machine, and persistence, exposing methods the Wails
// frontend binds to. It depends only on injected interfaces (Emitter,
// ConnectorFactory) so it is fully unit-testable and free of platform code.
package app

import (
	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/vless"
)

// ConnState is the connection state-machine state.
type ConnState string

const (
	ConnDisconnected  ConnState = "disconnected"
	ConnConnecting    ConnState = "connecting"
	ConnConnected     ConnState = "connected"
	ConnDisconnecting ConnState = "disconnecting"
	ConnError         ConnState = "error"
)

// socksPort is the fixed local SOCKS inbound port for this version.
const socksPort = 10808

// Emitter delivers named events with a JSON-serializable payload to the
// frontend. The Wails implementation wraps runtime.EventsEmit.
type Emitter interface {
	Emit(event string, data any)
}

// Connector is a started/stopped capture session (satisfied by *tunnel.Tunnel).
type Connector interface {
	Start() error
	Stop() error
}

// ConnConfig is what the service hands the factory to build a Connector. The
// Device/TunIP/TunPrefix/RouteCIDRs fields are populated for TUN mode only.
type ConnConfig struct {
	XrayJSON   []byte
	SocksHost  string
	SocksPort  int
	Mode       store.Mode
	Device     string
	TunIP      string
	TunPrefix  int
	RouteCIDRs []string
	KillSwitch bool
}

// TUN device defaults. 198.18.0.0/15 is the benchmarking range (RFC 2544),
// unlikely to collide with real destinations. The adapter address uses a /30 so
// its on-link subnet stays tiny: a wide prefix would route tens of thousands of
// addresses into the TUN and, combined with the catch-all direct rule, form a
// routing loop. Whitelisted CIDRs are added as explicit interface routes
// regardless of this prefix.
const (
	tunDevice = "tun0"
	tunIP     = "198.18.0.1"
	tunPrefix = 30
)

// ConnectorFactory builds a Connector for a session. The Wails layer supplies
// the real (proxy-mode) implementation; tests supply a fake.
type ConnectorFactory func(ConnConfig) (Connector, error)

// Deps are the service's injected dependencies.
type Deps struct {
	StatePath string           // path to state.json
	LogDir    string           // directory for xray.log
	Emitter   Emitter          // event sink for the frontend
	Factory   ConnectorFactory // builds the capture session
	Elevated  func() bool      // reports OS privilege (privilege.IsElevated)
	// KillSwitchSupported reports whether the kill switch is implemented on this
	// OS (firewall.Supported). Nil is treated as supported, so tests need not
	// set it. When false, a requested kill switch is skipped with a warning
	// rather than aborting the TUN connection.
	KillSwitchSupported func() bool
}

// ServerDTO is a server entry as shown in the UI.
type ServerDTO struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Network  string `json:"network"`
}

// ProfileDTO mirrors routing.Profile for the frontend.
type ProfileDTO struct {
	Telegram           bool     `json:"telegram"`
	ForceRUDirect      bool     `json:"forceRUDirect"`
	CustomProxyDomains []string `json:"customProxyDomains"`
	CustomProxyIPs     []string `json:"customProxyIPs"`
}

// SettingsDTO mirrors store.Settings for the frontend.
type SettingsDTO struct {
	Mode        string `json:"mode"`
	AutoConnect bool   `json:"autoConnect"`
	AutoStart   bool   `json:"autoStart"`
	KillSwitch  bool   `json:"killSwitch"`
	Mux         bool   `json:"mux"`
}

// StateDTO is the full snapshot the frontend renders.
type StateDTO struct {
	Servers      []ServerDTO `json:"servers"`
	ActiveServer int         `json:"activeServer"`
	Profile      ProfileDTO  `json:"profile"`
	Settings     SettingsDTO `json:"settings"`
	Conn         string      `json:"conn"`
	LastError    string      `json:"lastError"`
}

func serverDTO(s *vless.ServerConfig) ServerDTO {
	return ServerDTO{
		Name:     s.Name,
		Host:     s.Host,
		Port:     s.Port,
		Security: string(s.Security),
		Network:  string(s.Network),
	}
}

func profileDTO(p routing.Profile) ProfileDTO {
	return ProfileDTO{
		Telegram:           p.Telegram,
		ForceRUDirect:      p.ForceRUDirect,
		CustomProxyDomains: p.CustomProxyDomains,
		CustomProxyIPs:     p.CustomProxyIPs,
	}
}

func settingsDTO(s store.Settings) SettingsDTO {
	return SettingsDTO{
		Mode:        string(s.Mode),
		AutoConnect: s.AutoConnect,
		AutoStart:   s.AutoStart,
		KillSwitch:  s.KillSwitch,
		Mux:         s.Mux,
	}
}
