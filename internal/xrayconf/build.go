package xrayconf

import (
	"encoding/json"
	"fmt"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

// Options holds runtime knobs for the generated config.
type Options struct {
	SocksPort int    // local SOCKS inbound port (e.g. 10808)
	LogFile   string // optional path for xray's error log (empty = stdout/default)
	// LogLevel is xray's log.loglevel (error/warning/debug). Empty defaults to
	// warning.
	LogLevel string
	// Mux multiplexes many proxied streams over a few real connections to the
	// server. Telegram opens dozens of sockets in parallel; without mux each is a
	// separate Reality handshake to the server, and the burst gets dropped (DPI /
	// rate limiting), collapsing throughput. Mux is incompatible with the
	// xtls-rprx-vision flow, so enabling it drops the vision flow from the
	// outbound — the server's client must be configured with no flow (Reality
	// still applies) for the handshake to match.
	Mux bool
	// BindInterface binds the proxy and direct outbounds' sockets to the named
	// physical network interface (xray sockopt.interface: SO_BINDTODEVICE on
	// Linux, IP_UNICAST_IF on Windows). In TUN mode this is what keeps
	// direct-tagged traffic (and the encrypted server connection) from being
	// routed back into the TUN by the split-default routes and looping. Empty
	// leaves the sockets unbound.
	BindInterface string
}

type xrayConfig struct {
	Log       logConf     `json:"log"`
	DNS       dnsConf     `json:"dns"`
	Inbounds  []inbound   `json:"inbounds"`
	Outbounds []outbound  `json:"outbounds"`
	Routing   routingConf `json:"routing"`
}

type logConf struct {
	LogLevel string `json:"loglevel"`
	Error    string `json:"error,omitempty"`
}

type dnsConf struct {
	Servers []any `json:"servers"`
}

type inbound struct {
	Tag      string        `json:"tag"`
	Listen   string        `json:"listen"`
	Port     int           `json:"port"`
	Protocol string        `json:"protocol"`
	Settings socksSettings `json:"settings"`
	Sniffing sniffing      `json:"sniffing"`
}

type socksSettings struct {
	Auth string `json:"auth"`
	UDP  bool   `json:"udp"`
}

type sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
	// RouteOnly uses the sniffed domain for routing decisions only, keeping the
	// original destination address for the outbound connection. Without it,
	// sniffing overrides the destination with the sniffed domain, which breaks
	// Telegram's MTProto (its obfuscated/fake-TLS handshake yields a bogus SNI,
	// so the connection is redialed to garbage). Domain-based geosite rules
	// still work.
	RouteOnly bool `json:"routeOnly,omitempty"`
}

type outbound struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	StreamSettings *streamSettings `json:"streamSettings,omitempty"`
	Mux            *muxConf        `json:"mux,omitempty"`
}

type muxConf struct {
	Enabled     bool `json:"enabled"`
	Concurrency int  `json:"concurrency"`
}

type streamSettings struct {
	Network         string           `json:"network,omitempty"`
	Security        string           `json:"security,omitempty"`
	RealitySettings *realitySettings `json:"realitySettings,omitempty"`
	TLSSettings     *tlsSettings     `json:"tlsSettings,omitempty"`
	WSSettings      *wsSettings      `json:"wsSettings,omitempty"`
	GRPCSettings    *grpcSettings    `json:"grpcSettings,omitempty"`
	Sockopt         *sockoptConf     `json:"sockopt,omitempty"`
}

type sockoptConf struct {
	Interface string `json:"interface,omitempty"`
}

type realitySettings struct {
	ServerName  string `json:"serverName"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"publicKey"`
	ShortID     string `json:"shortId"`
	SpiderX     string `json:"spiderX"`
}

type tlsSettings struct {
	ServerName  string   `json:"serverName"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
}

type wsSettings struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
}

type grpcSettings struct {
	ServiceName string `json:"serviceName"`
}

type routingConf struct {
	DomainStrategy string     `json:"domainStrategy"`
	Rules          []ruleJSON `json:"rules"`
}

type ruleJSON struct {
	Type        string   `json:"type"`
	OutboundTag string   `json:"outboundTag"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Network     string   `json:"network,omitempty"`
}

// Build produces the xray JSON config bytes.
func Build(s *vless.ServerConfig, p routing.Profile, opts Options) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("nil server config")
	}
	if opts.SocksPort <= 0 {
		opts.SocksPort = 10808
	}

	proxyOut, err := buildProxyOutbound(s, opts.Mux)
	if err != nil {
		return nil, err
	}

	directOut := outbound{Tag: "direct", Protocol: "freedom"}
	if opts.BindInterface != "" {
		bind := &sockoptConf{Interface: opts.BindInterface}
		proxyOut.StreamSettings.Sockopt = bind
		directOut.StreamSettings = &streamSettings{Sockopt: bind}
	}

	cfg := xrayConfig{
		Log: logConf{LogLevel: orDefault(opts.LogLevel, "warning"), Error: opts.LogFile},
		DNS: dnsConf{Servers: []any{"1.1.1.1", "localhost"}},
		Inbounds: []inbound{{
			Tag:      "socks-in",
			Listen:   "127.0.0.1",
			Port:     opts.SocksPort,
			Protocol: "socks",
			Settings: socksSettings{Auth: "noauth", UDP: true},
			Sniffing: sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}, RouteOnly: true},
		}},
		Outbounds: []outbound{
			proxyOut,
			directOut,
			{Tag: "block", Protocol: "blackhole"},
		},
		Routing: routingConf{
			DomainStrategy: "IPIfNonMatch",
			Rules:          withLoopGuard(toRuleJSON(p.Rules())),
		},
	}
	return json.Marshal(cfg)
}

func buildProxyOutbound(s *vless.ServerConfig, mux bool) (outbound, error) {
	// Mux is incompatible with the xtls-rprx-vision flow; xray refuses to start if
	// both are set. When mux is on, drop the flow (Reality alone still applies).
	flow := s.Flow
	if mux {
		flow = ""
	}
	settings := map[string]any{
		"vnext": []any{map[string]any{
			"address": s.Host,
			"port":    s.Port,
			"users": []any{map[string]any{
				"id":         s.UUID,
				"encryption": "none",
				"flow":       flow,
			}},
		}},
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return outbound{}, err
	}

	stream := &streamSettings{
		Network:  string(s.Network),
		Security: string(s.Security),
	}
	switch s.Security {
	case vless.SecurityReality:
		stream.RealitySettings = &realitySettings{
			ServerName:  s.SNI,
			Fingerprint: orDefault(s.Fingerprint, "chrome"),
			PublicKey:   s.PublicKey,
			ShortID:     s.ShortID,
			SpiderX:     s.SpiderX,
		}
	case vless.SecurityTLS:
		stream.TLSSettings = &tlsSettings{
			ServerName:  s.SNI,
			Fingerprint: s.Fingerprint,
			ALPN:        s.ALPN,
		}
	}
	switch s.Network {
	case vless.NetworkWS:
		ws := &wsSettings{Path: orDefault(s.Path, "/")}
		if s.WsHost != "" {
			ws.Headers = map[string]string{"Host": s.WsHost}
		}
		stream.WSSettings = ws
	case vless.NetworkGRPC:
		stream.GRPCSettings = &grpcSettings{ServiceName: s.ServiceName}
	}

	out := outbound{
		Tag:            "proxy",
		Protocol:       "vless",
		Settings:       rawSettings,
		StreamSettings: stream,
	}
	if mux {
		out.Mux = &muxConf{Enabled: true, Concurrency: 8}
	}
	return out, nil
}

// withLoopGuard prepends a rule that blackholes the TUN adapter's reserved
// subnet. In TUN mode the OS routes the whole on-link TUN subnet into the TUN
// device; without this guard a packet addressed to that subnet is forwarded to
// xray, matched by the catch-all direct rule, dialed back out by the OS into
// the TUN, and re-forwarded — an amplifying loop that exhausts memory and
// ephemeral ports. Placed first so it wins over the catch-all. Harmless in
// proxy mode: the range carries no real traffic.
func withLoopGuard(rules []ruleJSON) []ruleJSON {
	guard := ruleJSON{
		Type:        "field",
		OutboundTag: routing.OutboundBlock,
		IP:          []string{routing.TUNReservedCIDR},
	}
	return append([]ruleJSON{guard}, rules...)
}

func toRuleJSON(rules []routing.Rule) []ruleJSON {
	out := make([]ruleJSON, 0, len(rules))
	for _, r := range rules {
		out = append(out, ruleJSON{
			Type:        "field",
			OutboundTag: r.Outbound,
			Domain:      r.Domains,
			IP:          r.IPs,
			Network:     r.Network,
		})
	}
	return out
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
