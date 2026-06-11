//go:build !windows

package privilege

import "testing"

func TestRunElevatedUnsupported(t *testing.T) {
	if err := RunElevated("/tmp/whatever.exe"); err == nil {
		t.Error("RunElevated: expected error off Windows, got nil")
	}
}
