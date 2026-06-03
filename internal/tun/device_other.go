//go:build !linux

package tun

// checkDevice is a no-op on platforms where the TUN device is opened by name
// rather than through a /dev node (Windows, macOS).
func checkDevice() error { return nil }
