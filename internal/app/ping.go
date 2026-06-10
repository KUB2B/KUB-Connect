package app

import (
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/zki/vless-client/internal/netping"
)

const pingTimeout = 5 * time.Second

// Ping measures TCP connect latency to the server at the given index. The dial
// runs outside the lock so concurrent pings don't block other operations.
func (s *Service) Ping(index int) PingResultDTO {
	s.mu.Lock()
	if index < 0 || index >= len(s.state.Servers) {
		s.mu.Unlock()
		return PingResultDTO{Error: "неверный сервер"}
	}
	srv := s.state.Servers[index]
	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))
	s.mu.Unlock()

	d, err := netping.Ping(addr, pingTimeout)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return PingResultDTO{Error: "таймаут"}
		}
		return PingResultDTO{Error: "недоступен"}
	}
	return PingResultDTO{OK: true, LatencyMs: int(d.Milliseconds())}
}
