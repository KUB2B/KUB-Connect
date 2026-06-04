//go:build !linux

package firewall

import (
	"fmt"
	"runtime"
)

type stubFirewall struct{}

// New returns a stub kill switch that errors on use. Real darwin/windows
// firewall support is a later iteration.
func New() Firewall { return stubFirewall{} }

// Supported reports whether the kill switch is implemented on this OS.
func Supported() bool { return false }

func (stubFirewall) On(Config) error {
	return fmt.Errorf("firewall: kill switch not supported on %s yet", runtime.GOOS)
}

func (stubFirewall) Off() error { return nil }
