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

// UpdateSettings replaces app settings after validating the capture mode.
func (s *Service) UpdateSettings(in SettingsDTO) error {
	mode := store.Mode(in.Mode)
	if mode != store.ModeProxy && mode != store.ModeTUN {
		return fmt.Errorf("invalid mode %q", in.Mode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Settings = store.Settings{
		Mode:        mode,
		AutoConnect: in.AutoConnect,
		AutoStart:   in.AutoStart,
		KillSwitch:  in.KillSwitch,
		Mux:         in.Mux,
		LogLevel:    store.NormalizeLogLevel(in.LogLevel),
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}
