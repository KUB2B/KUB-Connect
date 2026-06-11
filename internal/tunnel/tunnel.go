// Package tunnel orchestrates traffic capture: it starts the xray-core
// instance and then either sets the system SOCKS proxy (proxy mode) or
// brings up a TUN device and routes whitelisted CIDRs into it (tun mode).
package tunnel

import (
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

// Proxy sets/clears the system SOCKS proxy.
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
	cfg  Config
	deps Deps
	inst Stopper
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

// Start launches the tunnel. On any step failure it rolls back prior steps.
func (t *Tunnel) Start() error {
	inst, err := t.deps.Core.Start(t.cfg.XrayJSON)
	if err != nil {
		return fmt.Errorf("start core: %w", err)
	}
	t.inst = inst

	switch t.cfg.Mode {
	case store.ModeProxy:
		if err := t.deps.Proxy.Set(t.cfg.SocksHost, t.cfg.SocksPort); err != nil {
			_ = t.inst.Stop()
			t.inst = nil
			return fmt.Errorf("set system proxy: %w", err)
		}
	case store.ModeTUN:
		if err := t.deps.Tun.Start(t.cfg.Device, t.socksURL()); err != nil {
			_ = t.inst.Stop()
			t.inst = nil
			return fmt.Errorf("start tun: %w", err)
		}
		if err := t.deps.Router.Up(t.netcfgConfig()); err != nil {
			_ = t.deps.Tun.Stop()
			_ = t.inst.Stop()
			t.inst = nil
			return fmt.Errorf("apply routes: %w", err)
		}
		if t.cfg.KillSwitch && !t.cfg.Full {
			if err := t.deps.Firewall.On(firewall.Config{Device: t.cfg.Device, CIDRs: t.cfg.RouteCIDRs}); err != nil {
				_ = t.deps.Router.Down(t.netcfgConfig())
				_ = t.deps.Tun.Stop()
				_ = t.inst.Stop()
				t.inst = nil
				return fmt.Errorf("enable kill switch: %w", err)
			}
		}
	default:
		_ = t.inst.Stop()
		t.inst = nil
		return fmt.Errorf("unknown mode %q", t.cfg.Mode)
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
		record(t.deps.Proxy.Clear())
	case store.ModeTUN:
		if t.cfg.KillSwitch && !t.cfg.Full {
			record(t.deps.Firewall.Off())
		}
		record(t.deps.Router.Down(t.netcfgConfig()))
		record(t.deps.Tun.Stop())
	}
	if t.inst != nil {
		record(t.inst.Stop())
		t.inst = nil
	}
	return firstErr
}
