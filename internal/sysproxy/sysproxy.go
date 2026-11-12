// Package sysproxy temporarily replaces and then restores the OS-wide SOCKS proxy.
package sysproxy

import (
	"strconv"
	"strings"
)

// Proxy configures the system-wide SOCKS proxy.
type Proxy interface {
	// Set points the system SOCKS proxy at host:port.
	Set(host string, port int) error
	// Clear restores the proxy configuration captured by Set.
	Clear() error
}

// staleSocksPrefix is the ProxyServer value prefix our Windows Set writes.
const staleSocksPrefix = "socks=127.0.0.1:"

// localSocksAddr returns the loopback SOCKS address from a WinINet ProxyServer
// value written by Set ("socks=127.0.0.1:<port>"), or ok=false for anything else.
func localSocksAddr(server string) (addr string, ok bool) {
	port, found := strings.CutPrefix(server, staleSocksPrefix)
	if !found {
		return "", false
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", false
	}
	return "127.0.0.1:" + port, true
}
