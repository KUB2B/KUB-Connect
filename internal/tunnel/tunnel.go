// Package tunnel orchestrates traffic capture: it starts the xray-core
// instance and then either sets the system SOCKS proxy (proxy mode) or
// brings up a TUN device and routes whitelisted CIDRs into it (tun mode).
package tunnel

import (
	"errors"
	"fmt"

	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/store"
)

// Stopper stops a running xray-core instance (satisfied by *core.Instance).
type Stopper interface {
	Stop() error
}

// Core starts an xray-core instance from JSON config.
type Core interface {
	Start(jsonConfig []byte) (Stopper, error)
}

// Proxy temporarily replaces/restores the system SOCKS proxy.
type Proxy interface {
	Set(host string, port int) error
	Clear() error
}

// Tun runs the tun2socks engine.
type Tun interface {
	Start(device, socksURL string) error
	Stop() error
}

// Deps are the injected platform dependencies.
type Deps struct {
	Core     Core
	Proxy    Proxy
	Tun      Tun
	Router   netcfg.Router
	Firewall firewall.Firewall
}

// Config describes one tunnel session.
type Config struct {
	XrayJSON   []byte
	SocksHost  string
	SocksPort  int
	Mode       store.Mode
	Device     string
	TunIP      string
	TunPrefix  int
	RouteCIDRs []string
	KillSwitch bool
	Full       bool
	ServerIPs  []string
	BlockIPv6  bool
}

// Tunnel is a configured, possibly-running capture session.
type Tunnel struct {
	cfg        Config
	deps       Deps
	inst       Stopper
	proxySet   bool
	tunStarted bool
	routesUp   bool
	firewallOn bool
}

// New builds a Tunnel.
func New(cfg Config, deps Deps) *Tunnel {
	return &Tunnel{cfg: cfg, deps: deps}
}

func (t *Tunnel) socksURL() string {
	return fmt.Sprintf("socks5://%s:%d", t.cfg.SocksHost, t.cfg.SocksPort)
}

func (t *Tunnel) netcfgConfig() netcfg.Config {
	return netcfg.Config{
		Device:     t.cfg.Device,
		TunIP:      t.cfg.TunIP,
		Prefix:     t.cfg.TunPrefix,
		RouteCIDRs: t.cfg.RouteCIDRs,
		FullTunnel: t.cfg.Full,
		ServerIPs:  t.cfg.ServerIPs,
		BlockIPv6:  t.cfg.BlockIPv6,
	}
}

func (t *Tunnel) rollback(cause error) error {
	if err := t.Stop(); err != nil {
		return errors.Join(cause, fmt.Errorf("rollback tunnel: %w", err))
	}
	return cause
}

// Start launches the tunnel. On any step failure it rolls back prior steps.
func (t *Tunnel) Start() error {
	inst, err := t.deps.Core.Start(t.cfg.XrayJSON)
	if err != nil {
		return fmt.Errorf("start core: %w", err)
	}
	t.inst = inst

	switch t.cfg.Mode {
	case store.ModeProxy:
		// Set may fail after changing part of the platform proxy state, so mark
		// the step first and let Clear perform a best-effort rollback.
		t.proxySet = true
		if err := t.deps.Proxy.Set(t.cfg.SocksHost, t.cfg.SocksPort); err != nil {
			return t.rollback(fmt.Errorf("set system proxy: %w", err))
		}
	case store.ModeTUN:
		t.tunStarted = true
		if err := t.deps.Tun.Start(t.cfg.Device, t.socksURL()); err != nil {
			return t.rollback(fmt.Errorf("start tun: %w", err))
		}
		// Up is a command sequence and can fail after applying only a prefix of
		// it. Treat routes as active before entering it so rollback always runs.
		t.routesUp = true
		if err := t.deps.Router.Up(t.netcfgConfig()); err != nil {
			return t.rollback(fmt.Errorf("apply routes: %w", err))
		}
		// Kill switch guards the whitelisted CIDRs; with none (classic full
		// tunnel) there is nothing to guard, so it is skipped.
		if t.cfg.KillSwitch && len(t.cfg.RouteCIDRs) > 0 {
			// Mark it before the call because a failed nft transaction or another
			// firewall backend may still have applied part of its policy.
			t.firewallOn = true
			if err := t.deps.Firewall.On(firewall.Config{Device: t.cfg.Device, CIDRs: t.cfg.RouteCIDRs}); err != nil {
				return t.rollback(fmt.Errorf("enable kill switch: %w", err))
			}
		}
	default:
		return t.rollback(fmt.Errorf("unknown mode %q", t.cfg.Mode))
	}
	return nil
}

// Stop reverts capture and stops core. It attempts every step and returns the
// first error encountered.
func (t *Tunnel) Stop() error {
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	switch t.cfg.Mode {
	case store.ModeProxy:
		if t.proxySet {
			if err := t.deps.Proxy.Clear(); err != nil {
				record(err)
			} else {
				t.proxySet = false
			}
		}
	case store.ModeTUN:
		// Keep the kill switch installed while routes, the TUN engine and core
		// are being dismantled. Removing it first creates a short leak window.
		if t.routesUp {
			if err := t.deps.Router.Down(t.netcfgConfig()); err != nil {
				record(err)
			} else {
				t.routesUp = false
			}
		}
		if t.tunStarted {
			if err := t.deps.Tun.Stop(); err != nil {
				record(err)
			} else {
				t.tunStarted = false
			}
		}
	}
	if t.inst != nil {
		if err := t.inst.Stop(); err != nil {
			record(err)
		} else {
			t.inst = nil
		}
	}
	if t.cfg.Mode == store.ModeTUN && t.firewallOn {
		if err := t.deps.Firewall.Off(); err != nil {
			record(err)
		} else {
			t.firewallOn = false
		}
	}
	return firstErr
}
