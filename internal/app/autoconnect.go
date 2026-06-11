package app

import "github.com/zki/vless-client/internal/store"

// MaybeAutoConnect connects on startup when AutoConnect is enabled and an
// active server is configured. A connection failure is non-fatal: it is
// recorded in state/logs (via Connect) and otherwise ignored.
//
// In TUN mode this only succeeds when the process is already elevated; an
// unprivileged TUN auto-connect must go through WantsElevatedAutoConnect +
// RelaunchElevated instead, since Connect just fails the privilege check here.
func (s *Service) MaybeAutoConnect() {
	s.mu.Lock()
	enabled := s.state.Settings.AutoConnect
	hasActive := s.state.ActiveServer >= 0 && s.state.ActiveServer < len(s.state.Servers)
	s.mu.Unlock()

	if !enabled || !hasActive {
		return
	}
	_ = s.Connect()
}

// WantsElevatedAutoConnect reports whether the startup auto-connect needs an
// elevated relaunch first: AutoConnect is enabled with an active server, the
// mode is TUN, and the process is not yet privileged. The GUI uses this to
// RelaunchElevated(connectAfter=true) instead of calling MaybeAutoConnect,
// which would otherwise fail Connect's privilege check silently.
func (s *Service) WantsElevatedAutoConnect() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	enabled := s.state.Settings.AutoConnect
	hasActive := s.state.ActiveServer >= 0 && s.state.ActiveServer < len(s.state.Servers)
	return enabled && hasActive &&
		s.state.Settings.Mode == store.ModeTUN && !s.elevated()
}
