// Package netping measures TCP reachability and latency to a host:port.
package netping

import (
	"net"
	"time"
)

// Ping dials addr ("host:port") over TCP and returns the connect round-trip
// time. The dial is bounded by timeout; a successful connection is closed
// immediately.
func Ping(addr string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}
