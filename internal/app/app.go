package app

import (
	"sync"

	"github.com/zki/vless-client/internal/logbus"
	"github.com/zki/vless-client/internal/store"
)

// Service is the GUI-facing application service.
type Service struct {
	deps Deps
	bus  *logbus.Bus

	mu        sync.Mutex
	state     *store.State
	conn      ConnState
	lastError string
	connector Connector
	tailStop  func() // cancels the log tailer goroutine; nil when not tailing
}

// New constructs the service, loading persisted state (or defaults).
func New(d Deps) (*Service, error) {
	st, err := store.Load(d.StatePath)
	if err != nil {
		return nil, err
	}
	return &Service{
		deps:  d,
		bus:   logbus.New(2000),
		state: st,
		conn:  ConnDisconnected,
	}, nil
}

// snapshot builds a StateDTO. Caller must hold s.mu.
func (s *Service) snapshot() StateDTO {
	servers := make([]ServerDTO, 0, len(s.state.Servers))
	for _, sv := range s.state.Servers {
		servers = append(servers, serverDTO(sv))
	}
	return StateDTO{
		Servers:      servers,
		ActiveServer: s.state.ActiveServer,
		Profile:      profileDTO(s.state.Profile),
		Settings:     settingsDTO(s.state.Settings),
		Conn:         string(s.conn),
		LastError:    s.lastError,
	}
}

// GetState returns the current state snapshot.
func (s *Service) GetState() StateDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

// emitState pushes the current snapshot to the frontend. Caller must hold s.mu.
func (s *Service) emitState() {
	if s.deps.Emitter != nil {
		s.deps.Emitter.Emit("state", s.snapshot())
	}
}

// persist writes state to disk. Caller must hold s.mu.
func (s *Service) persist() error {
	return store.Save(s.deps.StatePath, s.state)
}
