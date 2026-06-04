//go:build !linux && !windows

package tun

// checkDevice is a no-op on platforms where the TUN device is opened by name
// rather than through a /dev node (macOS). Windows has its own check that
// loads the Wintun DLL (see device_windows.go).
func checkDevice() error { return nil }
