// Package sysproxy temporarily replaces and then restores the OS-wide SOCKS proxy.
package sysproxy

// Proxy configures the system-wide SOCKS proxy.
type Proxy interface {
	// Set points the system SOCKS proxy at host:port.
	Set(host string, port int) error
	// Clear restores the proxy configuration captured by Set.
	Clear() error
}
