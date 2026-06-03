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
	s.state.Servers = append(s.state.Servers, cfg)
	if s.state.ActiveServer < 0 {
		s.state.ActiveServer = 0
	}
	if err := s.persist(); err != nil {
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
		return err
	}
	s.emitState()
	return nil
}

// SetActiveServer selects the active server by index.
func (s *Service) SetActiveServer(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.state.Servers) {
		return fmt.Errorf("server index %d out of range", index)
	}
	s.state.ActiveServer = index
	if err := s.persist(); err != nil {
		return err
	}
	s.emitState()
	return nil
}
