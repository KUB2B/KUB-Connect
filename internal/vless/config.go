package vless

// Security is the streamSettings security type.
type Security string

const (
	SecurityNone    Security = "none"
	SecurityTLS     Security = "tls"
	SecurityReality Security = "reality"
)

// Network is the transport type.
type Network string

const (
	NetworkTCP  Network = "tcp"
	NetworkWS   Network = "ws"
	NetworkGRPC Network = "grpc"
)

// ServerConfig is a parsed vless:// link.
type ServerConfig struct {
	Name     string
	Host     string
	Port     int
	UUID     string
	Flow     string
	Security Security
	Network  Network

	// TLS / Reality
	SNI         string
	Fingerprint string
	ALPN        []string

	// Reality only
	PublicKey string
	ShortID   string
	SpiderX   string

	// ws
	Path   string
	WsHost string

	// grpc
	ServiceName string
}
