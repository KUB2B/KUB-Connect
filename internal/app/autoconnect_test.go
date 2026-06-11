package app

import "testing"

func TestMaybeAutoConnectConnectsWhenEnabled(t *testing.T) {
	svc, _, fc, _ := connectReadyService(t)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoConnect: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	svc.MaybeAutoConnect()
	if !fc.started {
		t.Error("auto-connect should have started the connector")
	}
	if svc.GetState().Conn != string(ConnConnected) {
		t.Error("auto-connect should reach connected")
	}
}

func TestMaybeAutoConnectSkipsWhenDisabled(t *testing.T) {
	svc, _, fc, _ := connectReadyService(t)
	// AutoConnect defaults to false.
	svc.MaybeAutoConnect()
	if fc.started {
		t.Error("auto-connect should not run when disabled")
	}
}

func TestMaybeAutoConnectSkipsWithoutActiveServer(t *testing.T) {
	svc, _, fc, _ := testDeps(t)
	_ = svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoConnect: true})
	svc.MaybeAutoConnect() // no servers added
	if fc.started {
		t.Error("auto-connect should not run without an active server")
	}
}

func TestWantsElevatedAutoConnectTUNUnprivileged(t *testing.T) {
	svc, _, _, _ := testDepsElevation(t, false)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "tun", AutoConnect: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if !svc.WantsElevatedAutoConnect() {
		t.Error("TUN auto-connect while unprivileged should request elevation")
	}
}

func TestWantsElevatedAutoConnectFalseCases(t *testing.T) {
	// Already elevated: no relaunch needed, MaybeAutoConnect handles it.
	svc, _, _, _ := testDepsElevation(t, true)
	mustAdd(t, svc, sampleLink)
	_ = svc.UpdateSettings(SettingsDTO{Mode: "tun", AutoConnect: true})
	if svc.WantsElevatedAutoConnect() {
		t.Error("elevated process should not request another elevation")
	}

	// Proxy mode never needs elevation.
	svc2, _, _, _ := testDepsElevation(t, false)
	mustAdd(t, svc2, sampleLink)
	_ = svc2.UpdateSettings(SettingsDTO{Mode: "proxy", AutoConnect: true})
	if svc2.WantsElevatedAutoConnect() {
		t.Error("proxy auto-connect should not request elevation")
	}

	// AutoConnect off: nothing to do.
	svc3, _, _, _ := testDepsElevation(t, false)
	mustAdd(t, svc3, sampleLink)
	_ = svc3.UpdateSettings(SettingsDTO{Mode: "tun", AutoConnect: false})
	if svc3.WantsElevatedAutoConnect() {
		t.Error("disabled auto-connect should not request elevation")
	}
}
