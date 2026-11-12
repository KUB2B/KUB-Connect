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
		if err := runAll(linuxFullUpCommands(c, gw, dev)); err != nil {
			_ = runAllBestEffort(linuxFullDownCommands(c))
			return err
		}
		return nil
	}
	if err := runAll(upCommands(c)); err != nil {
		_ = runAllBestEffort(downCommands(c))
		return err
	}
	return nil
}

// Down removes the routes Up added. It never fails: a route that is already
// gone is the wanted end state (the kernel drops an interface's routes when
// the link goes down), and whatever remains on the TUN device disappears with
// it. Returning an error here would leave the session stuck in "error" with no
// way to disconnect or reconnect.
func (linuxRouter) Down(c Config) error {
	if c.FullTunnel {
		_ = runAllBestEffort(linuxFullDownCommands(c))
		return nil
	}
	_ = runAllBestEffort(downCommands(c))
	return nil
}
