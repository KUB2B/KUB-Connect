// Package tun runs a tun2socks engine that forwards a TUN device's traffic
// to a SOCKS proxy. The engine creates and owns the TUN device; IP and route
// configuration is handled separately (see internal/netcfg).
package tun

import (
	"fmt"
	"net/url"

	"github.com/xjasonlyu/tun2socks/v2/engine"
)

// validateArgs checks the inputs tun2socks needs before we hand them to the
// engine, which crashes via log.Fatalf on bad input rather than returning an
// error. We can't intercept that exit, so we reject obvious misconfigurations
// here.
func validateArgs(device, socksURL string) error {
	if device == "" {
		return fmt.Errorf("tun: empty device name")
	}
	if socksURL == "" {
		return fmt.Errorf("tun: empty proxy URL")
	}
	u, err := url.Parse(socksURL)
	if err != nil {
		return fmt.Errorf("tun: bad proxy URL %q: %w", socksURL, err)
	}
	if u.Scheme != "socks5" {
		return fmt.Errorf("tun: proxy URL scheme %q is not socks5", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("tun: proxy URL %q has no host", socksURL)
	}
	return nil
}

// Start brings up a TUN device named `device` and forwards its traffic to
// socksURL (e.g. "socks5://127.0.0.1:10808"). Requires elevated privileges.
//
// Note: engine.Start() does not return an error; it calls log.Fatalf on
// failure. We validate inputs before calling it so obvious misconfigurations
// are returned as errors rather than crashing the process.
func Start(device, socksURL string) error {
	if err := validateArgs(device, socksURL); err != nil {
		return err
	}
	if err := checkDevice(); err != nil {
		return err
	}
	key := &engine.Key{
		Device:   "tun://" + device,
		Proxy:    socksURL,
		LogLevel: "warning",
	}
	engine.Insert(key)
	engine.Start()
	// engine.Start installs its own logger; override afterwards so the engine's
	// runtime logs reach our capture sink (the GUI process has no stderr).
	if engineLogWriter != nil {
		installEngineLogger(engineLogWriter)
	}
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
