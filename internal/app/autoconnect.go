package app

// MaybeAutoConnect connects on startup when AutoConnect is enabled and an
// active server is configured. A connection failure is non-fatal: it is
// recorded in state/logs (via Connect) and otherwise ignored.
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
