package app

import (
	"context"
	"time"
)

// defaultNetWatchInterval is how often the physical default interface is
// re-checked while a TUN session is up.
const defaultNetWatchInterval = 5 * time.Second

// startNetWatch watches for the default route moving to another interface
// (Wi-Fi <-> Ethernet, a different adapter after resume) and reconnects when it
// does. xray's outbound sockets are bound to the interface that was the default
// at connect time; once that changes every dial fails while the status still
// reads "connected". Caller must hold s.mu.
//
// ponytail: polls the interface name. A gateway change on the same interface is
// not detected (the bound sockets keep working there), and a failed reconnect
// is not retried. Move to NotifyRouteChange2 / netlink if the delay matters.
func (s *Service) startNetWatch(iface string) {
	s.stopNetWatch()
	interval := s.netWatchInterval
	if interval <= 0 {
		interval = defaultNetWatchInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.netWatchStop = cancel
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			// No default route (link down, mid-switch) is not a change: wait
			// for one to come back.
			name, err := s.deps.DefaultInterface()
			if err != nil || name == "" || name == iface {
				continue
			}
			s.mu.Lock()
			// A Disconnect/Connect that won the lock already replaced this
			// watcher; it must not tear down their session.
			if ctx.Err() == nil {
				s.bus.Append("network changed (" + iface + " -> " + name + ") — reconnecting")
				if err := s.disconnectLocked(); err == nil {
					_ = s.connectLocked()
				}
			}
			s.mu.Unlock()
			return
		}
	}()
}

// stopNetWatch stops the watcher. Caller must hold s.mu.
func (s *Service) stopNetWatch() {
	if s.netWatchStop != nil {
		s.netWatchStop()
		s.netWatchStop = nil
	}
}
