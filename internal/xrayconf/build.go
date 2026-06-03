package xrayconf

import (
	"encoding/json"
	"fmt"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

// Options holds runtime knobs for the generated config.
type Options struct {
	SocksPort int // local SOCKS inbound port (e.g. 10808)
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
}

type outbound struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	StreamSettings *streamSettings `json:"streamSettings,omitempty"`
}

type streamSettings struct {
	Network         string           `json:"network"`
	Security        string           `json:"security"`
	RealitySettings *realitySettings `json:"realitySettings,omitempty"`
	TLSSettings     *tlsSettings     `json:"tlsSettings,omitempty"`
	WSSettings      *wsSettings      `json:"wsSettings,omitempty"`
	GRPCSettings    *grpcSettings    `json:"grpcSettings,omitempty"`
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

	proxyOut, err := buildProxyOutbound(s)
	if err != nil {
		return nil, err
	}

	cfg := xrayConfig{
		Log: logConf{LogLevel: "warning"},
		DNS: dnsConf{Servers: []any{"1.1.1.1", "localhost"}},
		Inbounds: []inbound{{
			Tag:      "socks-in",
			Listen:   "127.0.0.1",
			Port:     opts.SocksPort,
			Protocol: "socks",
			Settings: socksSettings{Auth: "noauth", UDP: true},
			Sniffing: sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}},
		}},
		Outbounds: []outbound{
			proxyOut,
			{Tag: "direct", Protocol: "freedom"},
			{Tag: "block", Protocol: "blackhole"},
		},
		Routing: routingConf{
			DomainStrategy: "IPIfNonMatch",
			Rules:          toRuleJSON(p.Rules()),
		},
	}
	return json.Marshal(cfg)
}

func buildProxyOutbound(s *vless.ServerConfig) (outbound, error) {
	settings := map[string]any{
		"vnext": []any{map[string]any{
			"address": s.Host,
			"port":    s.Port,
			"users": []any{map[string]any{
				"id":         s.UUID,
				"encryption": "none",
				"flow":       s.Flow,
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

	return outbound{
		Tag:            "proxy",
		Protocol:       "vless",
		Settings:       rawSettings,
		StreamSettings: stream,
	}, nil
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
