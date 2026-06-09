package privilege

import (
	"runtime"
	"testing"
)

func TestIsElevatedReturnsBool(t *testing.T) {
	// We can't assert the value (depends on how tests run), but the call
	// must not panic and must return without error on this platform.
	_ = IsElevated()
}

func TestRequireElevatedMessage(t *testing.T) {
	err := RequireElevated("TUN mode")
	// On a non-root CI run this returns an error; if running as root it's nil.
	if err != nil && err.Error() == "" {
		t.Error("RequireElevated returned an empty error message")
	}
}

func TestRelaunchElevatedUnsupportedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has a real ShellExecute implementation")
	}
	if err := RelaunchElevated(); err == nil {
		t.Fatal("want a non-nil error from the non-windows stub")
	}
}
