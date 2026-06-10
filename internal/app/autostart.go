package app

import "fmt"

// autostartSupported reports whether launch-on-login is available on this OS.
func (s *Service) autostartSupported() bool {
	return s.deps.Autostart != nil && s.deps.Autostart.Supported()
}

// applyAutostart enables or disables OS launch-on-login. Disabling when
// unsupported is a no-op; enabling when unsupported is an error.
func (s *Service) applyAutostart(enable bool) error {
	if !s.autostartSupported() {
		if enable {
			return fmt.Errorf("autostart not supported on this OS")
		}
		return nil
	}
	if enable {
		return s.deps.Autostart.Enable()
	}
	return s.deps.Autostart.Disable()
}

// ReconcileAutostart refreshes the OS login entry to the current exe path when
// AutoStart is enabled (handles path drift after an update/reinstall).
// Non-fatal: a failure is logged to the bus.
func (s *Service) ReconcileAutostart() {
	s.mu.Lock()
	enabled := s.state.Settings.AutoStart
	s.mu.Unlock()
	if !enabled || !s.autostartSupported() {
		return
	}
	if err := s.deps.Autostart.Enable(); err != nil {
		s.bus.Append("warning: refresh autostart: " + err.Error())
	}
}
