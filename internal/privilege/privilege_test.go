package privilege

import (
	"reflect"
	"runtime"
	"testing"
)

func TestRelaunchArgsReplacesInternalParent(t *testing.T) {
	got := relaunchArgs([]string{"--user", relaunchParentArg + "12"}, 34)
	want := []string{"--user", relaunchParentArg + "34"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relaunchArgs = %v, want %v", got, want)
	}
	pid, remaining, err := takeRelaunchParent(got)
	if err != nil || pid != 34 || !reflect.DeepEqual(remaining, []string{"--user"}) {
		t.Fatalf("takeRelaunchParent = pid %d args %v err %v", pid, remaining, err)
	}
}

func TestTakeRelaunchParentRejectsInvalid(t *testing.T) {
	if _, _, err := takeRelaunchParent([]string{relaunchParentArg + "nope"}); err == nil {
		t.Fatal("expected invalid PID error")
	}
}

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
