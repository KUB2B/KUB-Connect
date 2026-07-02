package netcfg

type linuxRouter struct{}

// New returns the Linux (iproute2) router.
func New() Router { return linuxRouter{} }

// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return true }

func upCommands(c Config) [][]string {
	cmds := [][]string{
		{"ip", "addr", "add", cidr(c.TunIP, c.Prefix), "dev", c.Device},
		{"ip", "link", "set", "dev", c.Device, "up"},
	}
	for _, r := range c.RouteCIDRs {
		cmds = append(cmds, []string{"ip", "route", "add", r, "dev", c.Device})
	}
	return cmds
}

func downCommands(c Config) [][]string {
	var cmds [][]string
	for _, r := range c.RouteCIDRs {
		cmds = append(cmds, []string{"ip", "route", "del", r, "dev", c.Device})
	}
	cmds = append(cmds, []string{"ip", "addr", "del", cidr(c.TunIP, c.Prefix), "dev", c.Device})
	return cmds
}

// DefaultInterfaceName returns the physical default-route interface name, used
// to bind xray's outbound sockets (sockopt.interface → SO_BINDTODEVICE) so
// direct traffic cannot loop back into the TUN.
func DefaultInterfaceName() (string, error) {
	_, dev, err := defaultGateway()
	return dev, err
}

func (linuxRouter) Up(c Config) error {
	if c.FullTunnel {
		gw, dev, err := defaultGateway()
		if err != nil {
			return err
		}
		return runAll(linuxFullUpCommands(c, gw, dev))
	}
	return runAll(upCommands(c))
}

func (linuxRouter) Down(c Config) error {
	if c.FullTunnel {
		return runAll(linuxFullDownCommands(c))
	}
	return runAll(downCommands(c))
}
