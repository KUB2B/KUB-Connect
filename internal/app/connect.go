package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zki/vless-client/internal/logbus"
	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/xrayconf"
)

// tunRouteCIDRs is the set of whitelisted destination IP CIDRs: Telegram's
// published ranges (when enabled) plus the user's custom proxy IPs. Under the
// legacy selective host-route model these are the routes steered into the TUN
// (domain-based rules cannot be host-routed there); under full capture they
// only feed the kill-switch firewall.
//
// IPv6 ranges are deliberately excluded. Each proxied connection becomes its own
// Reality+Vision handshake to the server (Vision is incompatible with mux, so
// there is no connection reuse). Telegram opens dozens of sockets in parallel
// and, via Happy Eyeballs, races an IPv4 and an IPv6 connection to every data
// center at once — doubling that burst. The resulting storm of simultaneous
// handshakes overwhelms the path to the server (NAT table / rate limiting /
// DPI), so most dials time out and retry, collapsing throughput (media stalls
// while text trickles through). Routing only IPv4 into the TUN halves the burst
// and lets Happy Eyeballs fall back to the IPv4 path; the IPv6 attempts leave
// the TUN and fail fast on the host instead of piling onto the tunnel.
func tunRouteCIDRs(p routing.Profile) []string {
	var cidrs []string
	if p.Telegram {
		cidrs = append(cidrs, routing.TelegramCIDRs...)
	}
	cidrs = append(cidrs, p.CustomProxyIPs...)
	return ipv4Only(cidrs)
}

// ipv4Only drops IPv6 CIDRs (those containing a colon) from the list, preserving
// order. See tunRouteCIDRs for why IPv6 is kept out of the TUN.
func ipv4Only(cidrs []string) []string {
	out := cidrs[:0:0]
	for _, c := range cidrs {
		if !strings.Contains(c, ":") {
			out = append(out, c)
		}
	}
	return out
}

// killSwitchSupported reports whether the kill switch can run on this OS. A nil
// dep is treated as supported so unit tests need not wire it.
func (s *Service) killSwitchSupported() bool {
	return s.deps.KillSwitchSupported == nil || s.deps.KillSwitchSupported()
}

// tunSupported reports whether TUN routing can run on this OS. A nil dep is
// treated as supported so tests need not set it.
func (s *Service) tunSupported() bool {
	return s.deps.TUNSupported == nil || s.deps.TUNSupported()
}

// elevated reports whether the process has admin/root privileges. A nil dep is
// treated as elevated so unit tests need not set it (mirrors tunSupported).
func (s *Service) elevated() bool {
	return s.deps.Elevated == nil || s.deps.Elevated()
}

// bindInterface returns the physical default-route interface name, or "" when
// discovery is unsupported or fails (logged; the caller falls back to the
// legacy capture model).
func (s *Service) bindInterface() string {
	if s.deps.DefaultInterface == nil {
		return ""
	}
	name, err := s.deps.DefaultInterface()
	if err != nil {
		s.bus.Append("warning: default interface discovery: " + err.Error())
		return ""
	}
	return name
}

// xrayLogPath is where xray writes its error log, tailed into the log bus.
func (s *Service) xrayLogPath() string {
	return filepath.Join(s.deps.LogDir, "xray.log")
}

// setConn updates state + lastError and emits. Caller must hold s.mu.
func (s *Service) setConn(c ConnState, errMsg string) {
	s.conn = c
	s.lastError = errMsg
	s.emitState()
	s.notifyConn()
}

// Connect builds the config from the active server + profile and starts the
// capture session. Proxy mode only this phase.
func (s *Service) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == ConnConnected || s.conn == ConnConnecting {
		return fmt.Errorf("already %s", s.conn)
	}
	if s.state.ActiveServer < 0 || s.state.ActiveServer >= len(s.state.Servers) {
		return fmt.Errorf("no active server selected")
	}
	mode := s.state.Settings.Mode
	if mode == store.ModeTUN && !s.elevated() {
		return fmt.Errorf("TUN mode requires administrator/root privileges")
	}

	srv := s.state.Servers[s.state.ActiveServer]
	s.bus.Append(fmt.Sprintf("connecting to %s (%s:%d)", srv.Name, srv.Host, srv.Port))
	s.setConn(ConnConnecting, "")

	// TUN mode binds xray's outbound sockets to the physical interface so
	// direct-tagged traffic exits via the NIC instead of looping back into the
	// TUN under the split-default routes. Discovered before Build because the
	// xray config carries the binding.
	bindIface := ""
	if mode == store.ModeTUN {
		bindIface = s.bindInterface()
	}

	cfgJSON, err := xrayconf.Build(srv, s.state.Profile, xrayconf.Options{
		SocksPort: socksPort,
		LogFile:   s.xrayLogPath(),
		LogLevel:  s.state.Settings.LogLevel,
		// Mux tames the Telegram connection storm. It drops the xtls-rprx-vision
		// flow, so it only works when the server's client is configured with no
		// flow. User-gated to avoid breaking vision-only servers.
		Mux:           s.state.Settings.Mux,
		BindInterface: bindIface,
	})
	if err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: build config: " + err.Error())
		return fmt.Errorf("build config: %w", err)
	}

	cc := ConnConfig{
		XrayJSON:  cfgJSON,
		SocksHost: "127.0.0.1",
		SocksPort: socksPort,
		Mode:      mode,
		LogLevel:  s.state.Settings.LogLevel,
	}
	if mode == store.ModeTUN {
		cc.Device = tunDevice
		cc.TunIP = tunIP
		cc.TunPrefix = tunPrefix
		cc.RouteCIDRs = tunRouteCIDRs(s.state.Profile)
		cc.ServerIPs = resolveServerIPv4(srv.Host)
		switch {
		case bindIface != "":
			// Full-capture model: split-default routes steer all IPv4 into the
			// TUN and xray's rules decide proxy vs direct. Safe because the
			// outbounds are bound to bindIface. Domain-based rules (geosite,
			// presets, custom domains) work in both routing modes.
			cc.FullTunnel = true
			cc.BlockIPv6 = s.state.Profile.Full
			if len(cc.ServerIPs) == 0 {
				s.bus.Append("warning: could not resolve server IP for bypass route")
			}
		case s.state.Profile.Full:
			// Legacy full tunnel without a bound interface: direct exceptions
			// (RU напрямую, свои исключения) would loop — warn loudly.
			cc.FullTunnel = true
			cc.BlockIPv6 = true
			cc.RouteCIDRs = nil
			s.bus.Append("warning: physical interface unknown — direct exceptions may loop; disable 'RU напрямую' if browsing breaks")
			if len(cc.ServerIPs) == 0 {
				s.bus.Append("warning: could not resolve server IP for bypass; full-tunnel may loop")
			}
		default:
			// Legacy selective host-route model: only whitelisted IPs enter the
			// TUN, so domain rules cannot capture traffic.
			cc.ServerIPs = nil
			s.bus.Append("note: physical interface unknown — TUN routes whitelisted IPs only; domains/presets are not captured")
		}
		if s.state.Settings.KillSwitch {
			if s.killSwitchSupported() {
				cc.KillSwitch = true
				s.bus.Append("note: kill switch active — whitelisted IPs are blocked if they leave the TUN")
			} else {
				s.bus.Append("note: kill switch not supported on this OS — continuing without it")
			}
		}
	}

	conn, err := s.deps.Factory(cc)
	if err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: " + err.Error())
		return err
	}
	if err := conn.Start(); err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: start: " + err.Error())
		return err
	}

	s.connector = conn
	s.startTailing()
	s.bus.Append("connected")
	s.setConn(ConnConnected, "")
	return nil
}

// Disconnect stops the capture session.
func (s *Service) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connector == nil {
		s.setConn(ConnDisconnected, "")
		return nil
	}
	s.setConn(ConnDisconnecting, "")
	err := s.connector.Stop()
	s.connector = nil
	s.stopTailing()
	if err != nil {
		s.setConn(ConnError, err.Error())
		s.bus.Append("error: stop: " + err.Error())
		return err
	}
	s.bus.Append("disconnected")
	s.setConn(ConnDisconnected, "")
	return nil
}

// startTailing launches the xray log tailer. Caller must hold s.mu.
func (s *Service) startTailing() {
	if s.tailStop != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.tailStop = cancel
	go logbus.TailFile(ctx, s.xrayLogPath(), s.bus, 500*time.Millisecond)
}

// stopTailing stops the tailer. Caller must hold s.mu.
func (s *Service) stopTailing() {
	if s.tailStop != nil {
		s.tailStop()
		s.tailStop = nil
	}
}

// Logs returns the buffered log lines for the initial UI paint.
func (s *Service) Logs() []string {
	return s.bus.Lines()
}

// SubscribeLogs forwards every new log line to fn until the returned cancel is
// called. The Wails layer uses this to emit "log" events.
func (s *Service) SubscribeLogs(fn func(string)) (cancel func()) {
	return s.bus.Subscribe(fn)
}

// SetPendingConnect sets the one-shot connect-after-restart intent. Called by
// the GUI before an elevated restart so the new instance connects once.
func (s *Service) SetPendingConnect(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.PendingConnect = v
}

// ResumePendingConnect connects once if a pending-connect intent was persisted
// (set before an elevated restart). The flag is cleared on disk regardless of
// the connection outcome. Returns true if the intent was present (so the caller
// can skip the normal AutoConnect path). A connection failure is non-fatal:
// it is recorded in state/logs via Connect.
func (s *Service) ResumePendingConnect() bool {
	s.mu.Lock()
	pending := s.state.PendingConnect
	if pending {
		s.state.PendingConnect = false
		if err := s.persist(); err != nil {
			s.bus.Append("error: clear pending-connect: " + err.Error())
		}
	}
	s.mu.Unlock()

	if !pending {
		return false
	}
	_ = s.Connect()
	return true
}
