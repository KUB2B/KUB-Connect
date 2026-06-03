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
