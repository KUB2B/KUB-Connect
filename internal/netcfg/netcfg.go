// Package netcfg configures the TUN device IP address and the routes that
// steer whitelisted destinations into the tunnel.
package netcfg

// Config describes the TUN device and which CIDRs to route into it.
type Config struct {
	Device     string   // TUN device name, e.g. "tun0"
	TunIP      string   // device IP, e.g. "198.18.0.1"
	Prefix     int      // device IP prefix length, e.g. 15
	RouteCIDRs []string // whitelist mode: selective CIDRs into the TUN
	FullTunnel bool     // full mode: split-default + server bypass + ipv6 block
	ServerIPs  []string // full mode: server IPv4s to bypass via the physical gateway
	BlockIPv6  bool     // full mode: capture IPv6 into the TUN (xray blackholes it)
}

// Router applies and reverts TUN IP + routing configuration.
type Router interface {
	Up(c Config) error
	Down(c Config) error
}
