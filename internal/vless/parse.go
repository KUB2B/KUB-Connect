package vless

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Parse decodes a vless:// share link into a ServerConfig.
func Parse(link string) (*ServerConfig, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "vless" {
		return nil, fmt.Errorf("not a vless link: scheme %q", u.Scheme)
	}
	if u.User == nil || u.User.Username() == "" {
		return nil, errors.New("missing uuid in link")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %q", u.Port())
	}

	q := u.Query()
	cfg := &ServerConfig{
		Name:        u.Fragment,
		Host:        u.Hostname(),
		Port:        port,
		UUID:        u.User.Username(),
		Flow:        q.Get("flow"),
		Security:    Security(orDefault(q.Get("security"), string(SecurityNone))),
		Network:     Network(orDefault(q.Get("type"), string(NetworkTCP))),
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		PublicKey:   q.Get("pbk"),
		ShortID:     q.Get("sid"),
		SpiderX:     q.Get("spx"),
		Path:        q.Get("path"),
		WsHost:      q.Get("host"),
		ServiceName: q.Get("serviceName"),
	}
	if alpn := q.Get("alpn"); alpn != "" {
		cfg.ALPN = strings.Split(alpn, ",")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *ServerConfig) validate() error {
	if c.Host == "" {
		return errors.New("missing host")
	}
	switch c.Security {
	case SecurityNone, SecurityTLS, SecurityReality:
	default:
		return fmt.Errorf("unknown security %q", c.Security)
	}
	switch c.Network {
	case NetworkTCP, NetworkWS, NetworkGRPC:
	default:
		return fmt.Errorf("unknown network %q", c.Network)
	}
	if c.Security == SecurityReality && c.PublicKey == "" {
		return errors.New("reality requires pbk (public key)")
	}
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
