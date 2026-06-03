//go:build wails

package main

import (
	"fmt"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/core"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
	"github.com/zki/vless-client/internal/tunnel"
)

// coreAdapter adapts core.Start to the tunnel.Core interface.
type coreAdapter struct{}

func (coreAdapter) Start(jsonConfig []byte) (tunnel.Stopper, error) {
	return core.Start(jsonConfig)
}

// newConnector builds a proxy-mode capture session.
func newConnector(c app.ConnConfig) (app.Connector, error) {
	if c.Mode != store.ModeProxy {
		return nil, fmt.Errorf("mode %q not supported in this build (proxy only)", c.Mode)
	}
	return tunnel.New(tunnel.Config{
		XrayJSON:  c.XrayJSON,
		SocksHost: c.SocksHost,
		SocksPort: c.SocksPort,
		Mode:      store.ModeProxy,
	}, tunnel.Deps{
		Core:  coreAdapter{},
		Proxy: sysproxy.New(),
	}), nil
}
