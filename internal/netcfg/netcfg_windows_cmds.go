// Windows TUN routing command builders. These are deliberately free of build
// tags so they compile and unit-test on any OS; the Windows-only Router that
// executes them lives in netcfg_windows.go. tun2socks creates the Wintun
// adapter (named after Config.Device) but assigns no IP or routes, so the
// router does that here via netsh, mirroring the Linux iproute2 path.
package netcfg

import (
	"fmt"
	"net"
	"strings"
)

// splitCIDRs partitions CIDRs into IPv4 and IPv6 by the presence of a colon.
func splitCIDRs(cidrs []string) (v4, v6 []string) {
	for _, c := range cidrs {
		if strings.Contains(c, ":") {
			v6 = append(v6, c)
		} else {
			v4 = append(v4, c)
		}
	}
	return v4, v6
}

// prefixToMaskV4 renders an IPv4 prefix length as a dotted-decimal mask
// (e.g. 15 -> "255.254.0.0").
func prefixToMaskV4(prefix int) string {
	m := net.CIDRMask(prefix, 32)
	return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}

// winUpCommands assigns the TUN adapter's IPv4 address and routes each
// whitelisted CIDR into the adapter. Routes use store=active so they are
// non-persistent and cannot survive a crash/reboot.
func winUpCommands(c Config) [][]string {
	v4, v6 := splitCIDRs(c.RouteCIDRs)
	cmds := [][]string{
		{"netsh", "interface", "ipv4", "set", "address", "name=" + c.Device, "static", c.TunIP, prefixToMaskV4(c.Prefix)},
	}
	for _, r := range v4 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "add", "route", r, "interface=" + c.Device, "store=active"})
	}
	for _, r := range v6 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv6", "add", "route", r, "interface=" + c.Device, "store=active"})
	}
	return cmds
}

// winDownCommands removes the routes added by winUpCommands. The adapter's IP
// disappears when tun2socks destroys the Wintun adapter, so only routes are
// torn down here.
func winDownCommands(c Config) [][]string {
	v4, v6 := splitCIDRs(c.RouteCIDRs)
	var cmds [][]string
	for _, r := range v4 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv4", "delete", "route", r, "interface=" + c.Device})
	}
	for _, r := range v6 {
		cmds = append(cmds, []string{"netsh", "interface", "ipv6", "delete", "route", r, "interface=" + c.Device})
	}
	return cmds
}
