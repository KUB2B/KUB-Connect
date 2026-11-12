package app

import (
	"fmt"

	"github.com/zki/vless-client/internal/vless"
)

// AddServer parses link, appends it, and (if it is the first) selects it.
func (s *Service) AddServer(link string) error {
	cfg, err := vless.Parse(link)
	if err != nil {
		return fmt.Errorf("parse link: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prevServers := append([]*vless.ServerConfig(nil), s.state.Servers...)
	prevActive := s.state.ActiveServer
	s.state.Servers = append(s.state.Servers, cfg)
	if s.state.ActiveServer < 0 {
		s.state.ActiveServer = 0
	}
	if err := s.persist(); err != nil {
		s.state.Servers = prevServers
		s.state.ActiveServer = prevActive
		return err
	}
	s.emitState()
	return nil
}

// RemoveServer deletes the server at index and keeps ActiveServer valid.
func (s *Service) RemoveServer(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.state.Servers) {
		return fmt.Errorf("server index %d out of range", index)
	}
	// The running session would keep using a server that no longer exists.
	if index == s.state.ActiveServer && s.connector != nil {
		return fmt.Errorf("сервер используется — сначала отключитесь")
	}
	prevServers := append([]*vless.ServerConfig(nil), s.state.Servers...)
	prevActive := s.state.ActiveServer
	s.state.Servers = append(s.state.Servers[:index], s.state.Servers[index+1:]...)
	switch {
	case len(s.state.Servers) == 0:
		s.state.ActiveServer = -1
	case s.state.ActiveServer > index:
		s.state.ActiveServer--
	case s.state.ActiveServer == index:
		s.state.ActiveServer = 0
	}
	if err := s.persist(); err != nil {
		s.state.Servers = prevServers
		s.state.ActiveServer = prevActive
		return err
	}
	s.emitState()
	return nil
}

// SetActiveServer selects the active server by index. When connected, the
// session is restarted on the newly selected server.
func (s *Service) SetActiveServer(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.state.Servers) {
		return fmt.Errorf("server index %d out of range", index)
	}
	prev := s.state.ActiveServer
	s.state.ActiveServer = index
	if err := s.persist(); err != nil {
		s.state.ActiveServer = prev
		return err
	}
	s.emitState()
	// Switch a live session over; otherwise the UI shows the new server as
	// active while traffic still flows through the old one.
	if index != prev && s.conn == ConnConnected {
		if err := s.disconnectLocked(); err != nil {
			return err
		}
		return s.connectLocked()
	}
	return nil
}
