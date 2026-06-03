// Package tun runs a tun2socks engine that forwards a TUN device's traffic
// to a SOCKS proxy. The engine creates and owns the TUN device; IP and route
// configuration is handled separately (see internal/netcfg).
package tun

import (
	"errors"

	"github.com/xjasonlyu/tun2socks/v2/engine"
)

// Start brings up a TUN device named `device` and forwards its traffic to
// socksURL (e.g. "socks5://127.0.0.1:10808"). Requires elevated privileges.
//
// Note: engine.Start() does not return an error; it calls log.Fatalf on
// failure. We validate inputs before calling it so obvious misconfigurations
// are returned as errors rather than crashing the process.
func Start(device, socksURL string) error {
	if socksURL == "" {
		return errors.New("tun: empty proxy URL")
	}
	if device == "" {
		return errors.New("tun: empty device name")
	}
	key := &engine.Key{
		Device:   "tun://" + device,
		Proxy:    socksURL,
		LogLevel: "warning",
	}
	engine.Insert(key)
	engine.Start()
	return nil
}

// Stop tears down the engine and its TUN device.
//
// Note: engine.Stop() does not return an error; it calls log.Fatalf on
// failure. This wrapper returns nil to satisfy the expected interface.
func Stop() error {
	engine.Stop()
	return nil
}
