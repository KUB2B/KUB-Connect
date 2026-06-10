//go:build !windows && !darwin

package autostart

import (
	"fmt"
	"runtime"
)

type noopManager struct{}

// New returns a no-op autostart manager for unsupported platforms.
func New() Manager { return noopManager{} }

func (noopManager) Supported() bool { return false }

func (noopManager) Enable() error {
	return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
}

func (noopManager) Disable() error { return nil }
