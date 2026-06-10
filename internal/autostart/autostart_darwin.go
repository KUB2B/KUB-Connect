//go:build darwin

package autostart

import (
	"os"
	"os/exec"
	"path/filepath"
)

type macManager struct{}

// New returns the macOS autostart manager.
func New() Manager { return macManager{} }

func (macManager) Supported() bool { return true }

func macPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", macLabel+".plist"), nil
}

func (macManager) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := macPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(plistContent(macLabel, exe)), 0o644); err != nil {
		return err
	}
	// Best-effort immediate (re)load; the plist also takes effect at next login.
	_ = exec.Command("launchctl", "unload", "-w", path).Run()
	_ = exec.Command("launchctl", "load", "-w", path).Run()
	return nil
}

func (macManager) Disable() error {
	path, err := macPlistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", "-w", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
