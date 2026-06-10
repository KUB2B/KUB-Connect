package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type fakeAutostart struct {
	supported    bool
	enableCalls  int
	disableCalls int
	enableErr    error
}

func (f *fakeAutostart) Supported() bool { return f.supported }
func (f *fakeAutostart) Enable() error   { f.enableCalls++; return f.enableErr }
func (f *fakeAutostart) Disable() error  { f.disableCalls++; return nil }

func TestAutostartSupportedInCaps(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	svc.deps.Autostart = &fakeAutostart{supported: true}
	if !svc.GetState().Caps.AutostartSupported {
		t.Error("Caps.AutostartSupported should be true with a supported manager")
	}

	svc2, _, _, _ := testDeps(t)
	// Autostart dep left nil.
	if svc2.GetState().Caps.AutostartSupported {
		t.Error("Caps.AutostartSupported should be false when dep is nil")
	}
}

func TestReconcileAutostartRefreshesWhenEnabled(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	fa.enableCalls = 0 // ignore the enable from the toggle above
	svc.ReconcileAutostart()
	if fa.enableCalls != 1 {
		t.Errorf("ReconcileAutostart enableCalls = %d, want 1", fa.enableCalls)
	}
}

func TestReconcileAutostartSkipsWhenDisabled(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	// AutoStart defaults false.
	svc.ReconcileAutostart()
	if fa.enableCalls != 0 {
		t.Errorf("ReconcileAutostart should not enable when disabled; calls = %d", fa.enableCalls)
	}
}

func TestUpdateSettingsEnablesAutostartOnDelta(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if fa.enableCalls != 1 || fa.disableCalls != 0 {
		t.Errorf("enable=%d disable=%d, want 1/0", fa.enableCalls, fa.disableCalls)
	}
}

func TestUpdateSettingsDisablesAutostartOnDelta(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: false}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if fa.disableCalls != 1 {
		t.Errorf("disableCalls = %d, want 1", fa.disableCalls)
	}
}

func TestUpdateSettingsNoAutostartCallWithoutDelta(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	// AutoStart stays false (default) → no apply.
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: false}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if fa.enableCalls != 0 || fa.disableCalls != 0 {
		t.Errorf("enable=%d disable=%d, want 0/0", fa.enableCalls, fa.disableCalls)
	}
}

func TestUpdateSettingsAutostartErrorNotPersisted(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true, enableErr: fmt.Errorf("boom")}
	svc.deps.Autostart = fa
	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err == nil {
		t.Fatal("expected error from failed Enable")
	}
	// In-memory unchanged.
	if svc.GetState().Settings.AutoStart {
		t.Error("AutoStart should not be set when Enable failed")
	}
	// Not persisted: a fresh service loads AutoStart=false.
	svc2, _ := New(svc.deps)
	if svc2.GetState().Settings.AutoStart {
		t.Error("AutoStart should not have been persisted")
	}
}

func TestUpdateSettingsAutostartRolledBackOnPersistFailure(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	fa := &fakeAutostart{supported: true}
	svc.deps.Autostart = fa
	// Force persist to fail: point StatePath at a child of a regular file so
	// MkdirAll on its parent errors.
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.deps.StatePath = filepath.Join(blocker, "state.json")

	if err := svc.UpdateSettings(SettingsDTO{Mode: "proxy", AutoStart: true}); err == nil {
		t.Fatal("expected persist error")
	}
	if fa.enableCalls != 1 || fa.disableCalls != 1 {
		t.Errorf("want enable=1 (apply) + disable=1 (rollback), got enable=%d disable=%d", fa.enableCalls, fa.disableCalls)
	}
	if svc.GetState().Settings.AutoStart {
		t.Error("AutoStart should be rolled back to false in memory")
	}
}
