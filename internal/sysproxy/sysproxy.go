// Package sysproxy sets and clears the OS-wide SOCKS proxy.
package sysproxy

// Proxy configures the system-wide SOCKS proxy.
type Proxy interface {
	// Set points the system SOCKS proxy at host:port.
	Set(host string, port int) error
	// Clear disables the system SOCKS proxy.
	Clear() error
}
