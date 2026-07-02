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
	LogLevel   string
	// Full-tunnel (TUN mode) fields, set only when Profile.Full is enabled.
	FullTunnel bool     // route everything into the TUN (split-default + bypass)
	ServerIPs  []string // server IPv4s to bypass via the physical gateway
	BlockIPv6  bool     // capture + blackhole IPv6 while connected
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

// AutostartManager registers the app to launch at user login. Satisfied by
// *autostart.Manager values; nil is treated as unsupported.
type AutostartManager interface {
	Supported() bool
	Enable() error
	Disable() error
}

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
	// TUNSupported reports whether TUN routing is implemented on this OS
	// (netcfg.Supported). Nil is treated as supported, so tests need not set it.
	// When false and the persisted mode is TUN, the mode is coerced to proxy at
	// startup (see New).
	TUNSupported func() bool
	// DefaultInterface returns the physical default-route interface name
	// (netcfg.DefaultInterfaceName), used in TUN mode to bind xray's outbound
	// sockets so direct traffic exits via the NIC instead of looping into the
	// TUN. Nil or an error selects the legacy selective-route capture model.
	DefaultInterface func() (string, error)
	// Autostart registers launch-on-login (autostart.New()). Nil is treated as
	// unsupported, so tests need not set it.
	Autostart AutostartManager
	// OS is the runtime GOOS, surfaced to the frontend via Caps.
	OS string
	// Version is the build version string, surfaced to the frontend via Caps.
	Version string
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
	Full                bool     `json:"full"`
	Telegram            bool     `json:"telegram"`
	ForceRUDirect       bool     `json:"forceRUDirect"`
	CustomProxyDomains  []string `json:"customProxyDomains"`
	CustomProxyIPs      []string `json:"customProxyIPs"`
	ProxyPresets        []string `json:"proxyPresets"`
	CustomDirectDomains []string `json:"customDirectDomains"`
	CustomDirectIPs     []string `json:"customDirectIPs"`
}

// PresetDTO is one selectable service preset.
type PresetDTO struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// SettingsDTO mirrors store.Settings for the frontend.
type SettingsDTO struct {
	Mode        string `json:"mode"`
	AutoConnect bool   `json:"autoConnect"`
	AutoStart   bool   `json:"autoStart"`
	KillSwitch  bool   `json:"killSwitch"`
	Mux         bool   `json:"mux"`
	LogLevel    string `json:"logLevel"`
}

// PingResultDTO is the outcome of a server reachability test.
type PingResultDTO struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latencyMs"`
	Error     string `json:"error"`
}

// CapsDTO tells the frontend what this build/platform supports.
type CapsDTO struct {
	OS                  string `json:"os"`
	Version             string `json:"version"`
	TUNSupported        bool   `json:"tunSupported"`
	KillSwitchSupported bool   `json:"killSwitchSupported"`
	Elevated            bool   `json:"elevated"`
	AutostartSupported  bool   `json:"autostartSupported"`
}

// StateDTO is the full snapshot the frontend renders.
type StateDTO struct {
	Servers      []ServerDTO `json:"servers"`
	ActiveServer int         `json:"activeServer"`
	Profile      ProfileDTO  `json:"profile"`
	Settings     SettingsDTO `json:"settings"`
	Conn         string      `json:"conn"`
	LastError    string      `json:"lastError"`
	Caps         CapsDTO     `json:"caps"`
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
		Full:                p.Full,
		Telegram:            p.Telegram,
		ForceRUDirect:       p.ForceRUDirect,
		CustomProxyDomains:  p.CustomProxyDomains,
		CustomProxyIPs:      p.CustomProxyIPs,
		ProxyPresets:        p.ProxyPresets,
		CustomDirectDomains: p.CustomDirectDomains,
		CustomDirectIPs:     p.CustomDirectIPs,
	}
}

func settingsDTO(s store.Settings) SettingsDTO {
	return SettingsDTO{
		Mode:        string(s.Mode),
		AutoConnect: s.AutoConnect,
		AutoStart:   s.AutoStart,
		KillSwitch:  s.KillSwitch,
		Mux:         s.Mux,
		LogLevel:    s.LogLevel,
	}
}
