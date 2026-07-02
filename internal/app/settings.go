package app

import (
	"fmt"
	"net"
	"strings"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
)

// Presets returns the selectable service presets for the UI.
func (s *Service) Presets() []PresetDTO {
	out := make([]PresetDTO, 0, len(routing.Presets))
	for _, p := range routing.Presets {
		out = append(out, PresetDTO{Key: p.Key, Title: p.Title})
	}
	return out
}

// cleanList trims entries and drops empties, preserving order.
func cleanList(entries []string) []string {
	var out []string
	for _, e := range entries {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// validateDomains rejects entries that cannot be xray domain rules. Prefixed
// forms (geosite:, domain:, full:, regexp:, keyword:) are allowed as-is.
func validateDomains(entries []string) error {
	for _, e := range entries {
		if strings.ContainsAny(e, " \t") || strings.Contains(e, "://") {
			return fmt.Errorf("некорректный домен: %q", e)
		}
	}
	return nil
}

// validateIPs accepts plain IPs and CIDRs (both families).
func validateIPs(entries []string) error {
	for _, e := range entries {
		if net.ParseIP(e) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(e); err != nil {
			return fmt.Errorf("некорректный IP или CIDR: %q", e)
		}
	}
	return nil
}

func validatePresets(keys []string) error {
	for _, k := range keys {
		if _, ok := routing.PresetByKey(k); !ok {
			return fmt.Errorf("неизвестный пресет: %q", k)
		}
	}
	return nil
}

// UpdateProfile validates and replaces the routing profile.
func (s *Service) UpdateProfile(p ProfileDTO) error {
	next := routing.Profile{
		Full:                p.Full,
		Telegram:            p.Telegram,
		ForceRUDirect:       p.ForceRUDirect,
		CustomProxyDomains:  cleanList(p.CustomProxyDomains),
		CustomProxyIPs:      cleanList(p.CustomProxyIPs),
		ProxyPresets:        cleanList(p.ProxyPresets),
		CustomDirectDomains: cleanList(p.CustomDirectDomains),
		CustomDirectIPs:     cleanList(p.CustomDirectIPs),
	}
	if err := validateDomains(next.CustomProxyDomains); err != nil {
		return err
	}
	if err := validateDomains(next.CustomDirectDomains); err != nil {
		return err
	}
	if err := validateIPs(next.CustomProxyIPs); err != nil {
		return err
	}
	if err := validateIPs(next.CustomDirectIPs); err != nil {
		return err
	}
	if err := validatePresets(next.ProxyPresets); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Profile = next
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
