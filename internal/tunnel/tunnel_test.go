package tunnel

import (
	"errors"
	"testing"

	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/store"
)

type fakeStopper struct{ stopped bool }

func (f *fakeStopper) Stop() error { f.stopped = true; return nil }

type fakeCore struct {
	started bool
	inst    *fakeStopper
	err     error
}

func (f *fakeCore) Start(_ []byte) (Stopper, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.started = true
	f.inst = &fakeStopper{}
	return f.inst, nil
}

type fakeProxy struct{ set, cleared bool }

func (f *fakeProxy) Set(_ string, _ int) error { f.set = true; return nil }
func (f *fakeProxy) Clear() error              { f.cleared = true; return nil }

type fakeTun struct{ started, stopped bool }

func (f *fakeTun) Start(_, _ string) error { f.started = true; return nil }
func (f *fakeTun) Stop() error             { f.stopped = true; return nil }

type fakeRouter struct{ up, down bool }

func (f *fakeRouter) Up(_ netcfg.Config) error   { f.up = true; return nil }
func (f *fakeRouter) Down(_ netcfg.Config) error { f.down = true; return nil }

type fakeFirewall struct {
	on, off bool
	device  string
	cidrs   []string
	err     error
}

func (f *fakeFirewall) On(c firewall.Config) error {
	if f.err != nil {
		return f.err
	}
	f.on = true
	f.device = c.Device
	f.cidrs = c.CIDRs
	return nil
}
func (f *fakeFirewall) Off() error { f.off = true; return nil }

func newDeps() (*fakeCore, *fakeProxy, *fakeTun, *fakeRouter, *fakeFirewall, Deps) {
	c, p, tn, r, fw := &fakeCore{}, &fakeProxy{}, &fakeTun{}, &fakeRouter{}, &fakeFirewall{}
	return c, p, tn, r, fw, Deps{Core: c, Proxy: p, Tun: tn, Router: r, Firewall: fw}
}

func baseConfig(mode store.Mode) Config {
	return Config{
		XrayJSON: []byte("{}"), SocksHost: "127.0.0.1", SocksPort: 10808,
		Mode: mode, Device: "tun0", TunIP: "198.18.0.1", TunPrefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20"},
	}
}

func TestProxyModeStartStop(t *testing.T) {
	c, p, _, _, _, deps := newDeps()
	tn := New(baseConfig(store.ModeProxy), deps)
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !c.started || !p.set {
		t.Errorf("proxy mode should start core and set proxy: core=%v proxy=%v", c.started, p.set)
	}
	if err := tn.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !p.cleared || !c.inst.stopped {
		t.Errorf("proxy mode stop should clear proxy and stop core")
	}
}

func TestTunModeStartStop(t *testing.T) {
	c, _, tnsvc, r, _, deps := newDeps()
	tn := New(baseConfig(store.ModeTUN), deps)
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !c.started || !tnsvc.started || !r.up {
		t.Errorf("tun mode should start core+tun and apply routes")
	}
	if err := tn.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !r.down || !tnsvc.stopped || !c.inst.stopped {
		t.Errorf("tun mode stop should revert routes, stop tun, stop core")
	}
}

func TestStartRollsBackOnCoreFailure(t *testing.T) {
	c, p, _, _, _, deps := newDeps()
	c.err = errors.New("boom")
	tn := New(baseConfig(store.ModeProxy), deps)
	if err := tn.Start(); err == nil {
		t.Fatal("expected Start to fail when core fails")
	}
	if p.set {
		t.Error("proxy must not be set when core failed to start")
	}
}

func TestTunModeKillSwitchOnOff(t *testing.T) {
	_, _, _, _, fw, deps := newDeps()
	cfg := baseConfig(store.ModeTUN)
	cfg.KillSwitch = true
	tn := New(cfg, deps)
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !fw.on {
		t.Error("kill switch should be enabled in TUN mode when KillSwitch is set")
	}
	if fw.device != "tun0" || len(fw.cidrs) == 0 {
		t.Errorf("firewall got device=%q cidrs=%v", fw.device, fw.cidrs)
	}
	if err := tn.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !fw.off {
		t.Error("kill switch should be disabled on Stop")
	}
}

func TestTunModeKillSwitchDisabled(t *testing.T) {
	_, _, _, _, fw, deps := newDeps()
	tn := New(baseConfig(store.ModeTUN), deps) // KillSwitch false by default
	if err := tn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fw.on {
		t.Error("kill switch must stay off when KillSwitch is false")
	}
	_ = tn.Stop()
}

func TestStartRollsBackOnFirewallFailure(t *testing.T) {
	c, _, tnsvc, r, fw, deps := newDeps()
	fw.err = errors.New("nft boom")
	cfg := baseConfig(store.ModeTUN)
	cfg.KillSwitch = true
	tn := New(cfg, deps)
	if err := tn.Start(); err == nil {
		t.Fatal("expected Start to fail when firewall fails")
	}
	if !r.down || !tnsvc.stopped || !c.inst.stopped {
		t.Errorf("firewall failure must roll back routes/tun/core: down=%v tunStopped=%v coreStopped=%v",
			r.down, tnsvc.stopped, c.inst.stopped)
	}
}
