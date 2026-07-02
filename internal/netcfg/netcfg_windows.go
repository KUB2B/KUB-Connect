//go:build windows

package netcfg

import (
	"fmt"
	"net"
	"strconv"
)

type windowsRouter struct{}

// DefaultInterfaceName resolves the default route's interface index (see
// psDefaultRoute for why an index, not an alias) to its name via the Go net
// package, which reads Unicode adapter names directly from the OS — no
// OEM-codepage mangling. xray's sockopt.interface looks interfaces up by name.
func DefaultInterfaceName() (string, error) {
	_, dev, err := defaultGateway()
	if err != nil {
		return "", err
	}
	idx, err := strconv.Atoi(dev)
	if err != nil {
		return "", fmt.Errorf("bad interface index %q: %w", dev, err)
	}
	inf, err := net.InterfaceByIndex(idx)
	if err != nil {
		return "", fmt.Errorf("interface by index %d: %w", idx, err)
	}
	return inf.Name, nil
}

// New returns the Windows (netsh) router.
func New() Router { return windowsRouter{} }

// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return true }

func (windowsRouter) Up(c Config) error {
	if c.FullTunnel {
		gw, dev, err := defaultGateway()
		if err != nil {
			return err
		}
		return runAll(winFullUpCommands(c, gw, dev))
	}
	return runAll(winUpCommands(c))
}

func (windowsRouter) Down(c Config) error {
	if c.FullTunnel {
		_, dev, err := defaultGateway()
		if err != nil {
			// Best effort: still try to remove the split-default + ipv6 routes,
			// which do not need the physical interface. Server-bypass deletes are
			// skipped if the interface is unknown (they are non-persistent anyway).
			dev = ""
		}
		return runAll(winFullDownCommands(c, dev))
	}
	return runAll(winDownCommands(c))
}
