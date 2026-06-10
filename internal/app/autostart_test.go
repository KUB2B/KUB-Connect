package app

import "testing"

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
