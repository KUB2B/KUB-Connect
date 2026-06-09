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

// tunRouteCIDRs is the set of destination IP CIDRs steered into the TUN under
// the selective host-route model: Telegram's published ranges (when enabled)
// plus the user's custom proxy IPs. Domain-based rules cannot be host-routed.
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

	cfgJSON, err := xrayconf.Build(srv, s.state.Profile, xrayconf.Options{
		SocksPort: socksPort,
		LogFile:   s.xrayLogPath(),
		LogLevel:  s.state.Settings.LogLevel,
		// Mux tames the Telegram connection storm. It drops the xtls-rprx-vision
		// flow, so it only works when the server's client is configured with no
		// flow. User-gated to avoid breaking vision-only servers.
		Mux: s.state.Settings.Mux,
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
		s.bus.Append("note: TUN mode routes whitelisted IPs only; geosite domains are not host-routed")
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
