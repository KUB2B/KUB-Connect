//go:build !windows

package privilege

import "errors"

// RelaunchElevated is unsupported off Windows. On Linux dev hosts run the binary
// under sudo; macOS does not support TUN in this build.
func RelaunchElevated() error {
	return errors.New("elevation restart is not supported on this OS")
}

// RunElevated is unsupported off Windows; the in-app updater only ships a
// Windows installer.
func RunElevated(path string) error {
	return errors.New("elevated launch is not supported on this OS")
}
