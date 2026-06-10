package app

import "testing"

func TestResumePendingConnectConnectsAndClears(t *testing.T) {
	svc, _, fc, _ := testDepsElevation(t, true)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	svc.SetPendingConnect(true)

	if !svc.ResumePendingConnect() {
		t.Fatal("ResumePendingConnect should report it ran")
	}
	if !fc.started {
		t.Error("ResumePendingConnect should have started the connector")
	}
	if svc.state.PendingConnect {
		t.Error("flag should be cleared in memory")
	}
}

func TestResumePendingConnectSkipsWhenUnset(t *testing.T) {
	svc, _, fc, _ := testDepsElevation(t, true)
	mustAdd(t, svc, sampleLink)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	// PendingConnect defaults false.
	if svc.ResumePendingConnect() {
		t.Error("ResumePendingConnect should report false when unset")
	}
	if fc.started {
		t.Error("connector should not start when flag unset")
	}
}

func TestSetPendingConnectWritesField(t *testing.T) {
	svc, _, _, _ := testDepsElevation(t, true)
	svc.SetPendingConnect(true)
	if !svc.state.PendingConnect {
		t.Error("SetPendingConnect(true) should set the field")
	}
	svc.SetPendingConnect(false)
	if svc.state.PendingConnect {
		t.Error("SetPendingConnect(false) should clear the field")
	}
}
