//go:build !windows

package wintundll

// Ensure is a no-op on non-Windows platforms, where Wintun is not used.
func Ensure() error { return nil }
