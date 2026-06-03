//go:build !linux

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

func (unsupportedRouter) Up(Config) error {
	return fmt.Errorf("netcfg: TUN routing not supported on %s yet", runtime.GOOS)
}

func (unsupportedRouter) Down(Config) error {
	return fmt.Errorf("netcfg: TUN routing not supported on %s yet", runtime.GOOS)
}
