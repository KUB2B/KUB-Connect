// Full-tunnel route command builders. Like the whitelist builders these are
// free of build tags so they compile and unit-test on any OS; the OS-specific
// Routers (netcfg_linux.go / netcfg_windows.go) call them with a discovered
// physical gateway. Approach A: split-default routes (0.0.0.0/1 + 128.0.0.0/1)
// shadow the OS default without deleting it, while the server IPs are pinned to
// the physical gateway so the encrypted tunnel does not loop back into the TUN.
// IPv6 is captured into the TUN (::/1 + 8000::/1) where xray blackholes it.
package netcfg

import "strconv"

// cidr formats an IP and prefix length as "ip/prefix". Lives here (tag-free) so
// both the Linux whitelist builders and these full-tunnel builders can use it
// regardless of GOOS.
func cidr(ip string, prefix int) string {
	return ip + "/" + strconv.Itoa(prefix)
}

// linuxFullUpCommands builds the iproute2 commands to enter full-tunnel mode.
// gw/dev are the physical default gateway and its interface.
func linuxFullUpCommands(c Config, gw, dev string) [][]string {
	cmds := [][]string{
		{"ip", "addr", "add", cidr(c.TunIP, c.Prefix), "dev", c.Device},
		{"ip", "link", "set", "dev", c.Device, "up"},
	}
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"ip", "route", "add", ip + "/32", "via", gw, "dev", dev})
	}
	cmds = append(cmds,
		[]string{"ip", "route", "add", "0.0.0.0/1", "dev", c.Device},
		[]string{"ip", "route", "add", "128.0.0.0/1", "dev", c.Device},
	)
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"ip", "-6", "route", "add", "::/1", "dev", c.Device},
			[]string{"ip", "-6", "route", "add", "8000::/1", "dev", c.Device},
		)
	}
	return cmds
}

// linuxFullDownCommands reverses linuxFullUpCommands. Routes are matched by
// prefix (gateway not needed for deletion); the TUN address is removed last.
func linuxFullDownCommands(c Config) [][]string {
	var cmds [][]string
	cmds = append(cmds,
		[]string{"ip", "route", "del", "0.0.0.0/1"},
		[]string{"ip", "route", "del", "128.0.0.0/1"},
	)
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"ip", "route", "del", ip + "/32"})
	}
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"ip", "-6", "route", "del", "::/1"},
			[]string{"ip", "-6", "route", "del", "8000::/1"},
		)
	}
	cmds = append(cmds, []string{"ip", "addr", "del", cidr(c.TunIP, c.Prefix), "dev", c.Device})
	return cmds
}

// winFullUpCommands builds the netsh commands to enter full-tunnel mode.
// gw/dev are the physical default gateway IP and its interface index.
func winFullUpCommands(c Config, gw, dev string) [][]string {
	cmds := [][]string{
		{"netsh", "interface", "ipv4", "set", "address", "name=" + c.Device, "static", c.TunIP, prefixToMaskV4(c.Prefix)},
	}
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "add", "route",
			"prefix=" + ip + "/32", "interface=" + dev, "nexthop=" + gw, "store=active"})
	}
	cmds = append(cmds,
		[]string{"netsh", "interface", "ipv4", "add", "route", "prefix=0.0.0.0/1", "interface=" + c.Device, "store=active"},
		[]string{"netsh", "interface", "ipv4", "add", "route", "prefix=128.0.0.0/1", "interface=" + c.Device, "store=active"},
	)
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"netsh", "interface", "ipv6", "add", "route", "prefix=::/1", "interface=" + c.Device, "store=active"},
			[]string{"netsh", "interface", "ipv6", "add", "route", "prefix=8000::/1", "interface=" + c.Device, "store=active"},
		)
	}
	return cmds
}

// winFullDownCommands reverses winFullUpCommands. dev is the physical interface
// index (needed to delete the server-bypass routes). The TUN adapter's address
// disappears when tun2socks destroys the adapter, so only routes are removed.
func winFullDownCommands(c Config, dev string) [][]string {
	cmds := [][]string{
		{"netsh", "interface", "ipv4", "delete", "route", "prefix=0.0.0.0/1", "interface=" + c.Device},
		{"netsh", "interface", "ipv4", "delete", "route", "prefix=128.0.0.0/1", "interface=" + c.Device},
	}
	for _, ip := range c.ServerIPs {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "delete", "route",
			"prefix=" + ip + "/32", "interface=" + dev})
	}
	if c.BlockIPv6 {
		cmds = append(cmds,
			[]string{"netsh", "interface", "ipv6", "delete", "route", "prefix=::/1", "interface=" + c.Device},
			[]string{"netsh", "interface", "ipv6", "delete", "route", "prefix=8000::/1", "interface=" + c.Device},
		)
	}
	return cmds
}
