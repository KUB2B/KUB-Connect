// Package privilege reports whether the process has the OS privileges
// (root / Administrator) required for TUN-mode networking.
package privilege

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrElevationDeclined is returned by RelaunchElevated when the user dismisses
// the OS elevation (UAC) prompt.
var ErrElevationDeclined = errors.New("elevation request was declined")

const relaunchParentArg = "--kub-connect-elevation-parent="

// relaunchArgs removes any stale internal wait marker and appends the PID of
// the instance that currently owns Wails' single-instance mutex. The elevated
// child waits for that process before it starts Wails and attempts to acquire
// the mutex itself.
func relaunchArgs(args []string, parentPID int) []string {
	out := make([]string, 0, len(args)+1)
	for _, arg := range args {
		if !strings.HasPrefix(arg, relaunchParentArg) {
			out = append(out, arg)
		}
	}
	return append(out, relaunchParentArg+strconv.Itoa(parentPID))
}

// takeRelaunchParent extracts the internal parent marker from args and returns
// the remaining user-facing arguments.
func takeRelaunchParent(args []string) (pid int, remaining []string, err error) {
	remaining = make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, relaunchParentArg) {
			remaining = append(remaining, arg)
			continue
		}
		if pid != 0 {
			return 0, remaining, fmt.Errorf("duplicate elevation parent argument")
		}
		pid, err = strconv.Atoi(strings.TrimPrefix(arg, relaunchParentArg))
		if err != nil || pid <= 0 {
			return 0, remaining, fmt.Errorf("invalid elevation parent argument %q", arg)
		}
	}
	return pid, remaining, nil
}

// RequireElevated returns a descriptive error if the process is not elevated.
// purpose names the feature needing elevation, for the message.
func RequireElevated(purpose string) error {
	if IsElevated() {
		return nil
	}
	return fmt.Errorf("%s requires administrator/root privileges; re-run elevated or use proxy mode", purpose)
}
