package app

import (
	"reflect"
	"testing"
)

func TestUpdateProfilePersistsAndEmits(t *testing.T) {
	svc, em, _, _ := testDeps(t)
	p := ProfileDTO{
		Telegram:           false,
		ForceRUDirect:      true,
		CustomProxyDomains: []string{"youtube.com"},
		CustomProxyIPs:     []string{"1.2.3.4/32"},
	}
	if err := svc.UpdateProfile(p); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got := svc.GetState().Profile
	if !reflect.DeepEqual(got, p) {
		t.Errorf("profile = %+v, want %+v", got, p)
	}
	if em.events[len(em.events)-1].name != "state" {
		t.Error("UpdateProfile should emit a state event")
	}

	svc2, _ := New(svc.deps)
	if !reflect.DeepEqual(svc2.GetState().Profile, p) {
		t.Error("profile was not persisted")
	}
}

func TestUpdateSettingsPersists(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	in := SettingsDTO{Mode: "proxy", AutoConnect: true, AutoStart: false, KillSwitch: false}
	if err := svc.UpdateSettings(in); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	got := svc.GetState().Settings
	if got.Mode != "proxy" || !got.AutoConnect {
		t.Errorf("settings = %+v, want proxy + autoConnect", got)
	}
}

func TestUpdateSettingsRejectsBadMode(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.UpdateSettings(SettingsDTO{Mode: "bogus"}); err == nil {
		t.Error("expected error for invalid mode")
	}
}
