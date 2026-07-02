//go:build !linux && !windows

package netcfg

import (
	"fmt"
	"runtime"
)

type unsupportedRouter struct{}

// New returns a router that reports TUN routing is not yet implemented on this
// OS. It exists so the GUI build compiles on Windows/macOS; the real platform
// routing is a later iteration. Proxy mode is unaffected.
func New() Router { return unsupportedRouter{} }

// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return false }

// DefaultInterfaceName is unsupported on this OS.
func DefaultInterfaceName() (string, error) {
	return "", fmt.Errorf("netcfg: default interface discovery not supported on %s", runtime.GOOS)
}

func (unsupportedRouter) Up(Config) error {
	return fmt.Errorf("netcfg: TUN routing not supported on %s yet", runtime.GOOS)
}

func (unsupportedRouter) Down(Config) error {
	return fmt.Errorf("netcfg: TUN routing not supported on %s yet", runtime.GOOS)
}
