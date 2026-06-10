//go:build windows

package autostart

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	winRunKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	winValueName = "KUB Connect"
)

type winManager struct{}

// New returns the Windows autostart manager.
func New() Manager { return winManager{} }

func (winManager) Supported() bool { return true }

func (winManager) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, winRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(winValueName, runValue(exe))
}

func (winManager) Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, winRunKey, registry.SET_VALUE)
	if err != nil {
		return nil // key absent → nothing registered
	}
	defer k.Close()
	if err := k.DeleteValue(winValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
