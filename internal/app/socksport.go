package app

import (
	"fmt"
	"net"
)

// freeSocksPort returns preferred when it can be bound on loopback, otherwise a
// free port assigned by the OS. The fixed default (10808) is shared with other
// VLESS clients (v2rayN, Nekoray, Hiddify) and can also fall inside a Windows
// reserved port block (Hyper-V/WSL/Docker excluded ranges). In both cases the
// xray socks inbound fails to start with "bind: address already in use"
// ("Only one usage of each socket address ..." on Windows), which used to abort
// the whole connection.
//
// ponytail: the probe binds and releases, so there is a small window before
// xray binds for real; a retry of Connect covers it.
func freeSocksPort(preferred int) (int, error) {
	if preferred > 0 && portFree(preferred) {
		return preferred, nil
	}
	// The OS hands out a free TCP port; UDP may still be taken on it, so probe
	// both and retry a few times before giving up.
	for i := 0; i < 10; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("find free local port: %w", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if portFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free local SOCKS port available")
}

// portFree reports whether both loopback sockets for port are available. The
// socks inbound is created with UDP enabled, so xray binds TCP and UDP alike.
func portFree(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	l.Close()
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return false
	}
	pc.Close()
	return true
}
