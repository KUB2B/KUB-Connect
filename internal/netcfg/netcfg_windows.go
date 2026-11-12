//go:build windows

package netcfg

import (
	"fmt"
	"net"
	"strconv"
)

// windowsRouter remembers the physical interface the server-bypass routes were
// added on, so Down removes them from that interface rather than from
// whichever one is the default by then (they differ after a network change).
type windowsRouter struct {
	physDev string
}

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
func New() Router { return &windowsRouter{} }

// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return true }

func (r *windowsRouter) Up(c Config) error {
	if c.FullTunnel {
		gw, dev, err := defaultGateway()
		if err != nil {
			return err
		}
		r.physDev = dev
		// netsh has no "replace": drop a bypass route left on this interface by
		// an earlier session (crash, network change) so the add below does not
		// fail with "object already exists". Normally there is none.
		_ = runAllBestEffort(winBypassDeleteCommands(c, dev))
		if err := runAll(winFullUpCommands(c, gw, dev)); err != nil {
			_ = runAllBestEffort(winFullDownCommands(c, dev))
			return err
		}
		return nil
	}
	if err := runAll(winUpCommands(c)); err != nil {
		_ = runAllBestEffort(winDownCommands(c))
		return err
	}
	return nil
}

// Down removes the routes Up added. It never fails: a route that is already
// gone is the wanted end state (Windows drops an interface's routes when the
// link goes down, e.g. on a Wi-Fi -> Ethernet switch), and whatever remains on
// the TUN adapter disappears with it. Returning an error here would leave the
// session stuck in "error" with no way to disconnect or reconnect.
func (r *windowsRouter) Down(c Config) error {
	if c.FullTunnel {
		_ = runAllBestEffort(winFullDownCommands(c, r.physDev))
		return nil
	}
	_ = runAllBestEffort(winDownCommands(c))
	return nil
}
