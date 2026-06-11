package app

import (
	"fmt"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
)

// UpdateProfile replaces the routing profile.
func (s *Service) UpdateProfile(p ProfileDTO) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Profile = routing.Profile{
		Full:               p.Full,
		Telegram:           p.Telegram,
		ForceRUDirect:      p.ForceRUDirect,
		CustomProxyDomains: p.CustomProxyDomains,
		CustomProxyIPs:     p.CustomProxyIPs,
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}

// UpdateSettings replaces app settings after validating the capture mode. An
// AutoStart change is applied to the OS before the new settings are committed,
// so a failure leaves state unchanged.
func (s *Service) UpdateSettings(in SettingsDTO) error {
	mode := store.Mode(in.Mode)
	if mode != store.ModeProxy && mode != store.ModeTUN {
		return fmt.Errorf("invalid mode %q", in.Mode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.state.Settings
	autostartChanged := in.AutoStart != prev.AutoStart
	if autostartChanged {
		if err := s.applyAutostart(in.AutoStart); err != nil {
			return err // state not modified; frontend reverts the toggle
		}
	}
	s.state.Settings = store.Settings{
		Mode:        mode,
		AutoConnect: in.AutoConnect,
		AutoStart:   in.AutoStart,
		KillSwitch:  in.KillSwitch,
		Mux:         in.Mux,
		LogLevel:    store.NormalizeLogLevel(in.LogLevel),
	}
	if err := s.persist(); err != nil {
		// Roll back the OS autostart change and in-memory settings so disk,
		// memory, and the OS login entry stay consistent.
		if autostartChanged {
			_ = s.applyAutostart(prev.AutoStart)
		}
		s.state.Settings = prev
		return err
	}
	s.emitState()
	return nil
}
