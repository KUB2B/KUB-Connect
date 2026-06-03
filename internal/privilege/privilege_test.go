package privilege

import "testing"

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
