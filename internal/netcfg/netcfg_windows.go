//go:build windows

package netcfg

type windowsRouter struct{}

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
