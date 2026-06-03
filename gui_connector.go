//go:build wails

package main

import (
	"fmt"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/core"
	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
	"github.com/zki/vless-client/internal/tun"
	"github.com/zki/vless-client/internal/tunnel"
)

// coreAdapter adapts core.Start to the tunnel.Core interface.
type coreAdapter struct{}

func (coreAdapter) Start(jsonConfig []byte) (tunnel.Stopper, error) {
	return core.Start(jsonConfig)
}

// tunAdapter adapts the internal/tun package funcs to the tunnel.Tun interface.
type tunAdapter struct{}

func (tunAdapter) Start(device, socksURL string) error { return tun.Start(device, socksURL) }
func (tunAdapter) Stop() error                         { return tun.Stop() }

// newConnector builds a capture session for the requested mode.
func newConnector(c app.ConnConfig) (app.Connector, error) {
	cfg := tunnel.Config{
		XrayJSON:  c.XrayJSON,
		SocksHost: c.SocksHost,
		SocksPort: c.SocksPort,
		Mode:      c.Mode,
	}
	deps := tunnel.Deps{Core: coreAdapter{}, Firewall: firewall.New()}

	switch c.Mode {
	case store.ModeProxy:
		deps.Proxy = sysproxy.New()
	case store.ModeTUN:
		cfg.Device = c.Device
		cfg.TunIP = c.TunIP
		cfg.TunPrefix = c.TunPrefix
		cfg.RouteCIDRs = c.RouteCIDRs
		cfg.KillSwitch = c.KillSwitch
		deps.Tun = tunAdapter{}
		deps.Router = netcfg.New()
	default:
		return nil, fmt.Errorf("unknown mode %q", c.Mode)
	}
	return tunnel.New(cfg, deps), nil
}
