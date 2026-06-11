//go:build linux

package netcfg

import (
	"fmt"
	"os/exec"
)

// defaultGateway returns the physical default route's gateway IP and interface,
// read from `ip route show default`. Called before the split-default routes
// shadow it, so the original physical path is captured for the server bypass.
func defaultGateway() (gw, dev string, err error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("ip route show default: %w", err)
	}
	return parseLinuxDefaultRoute(string(out))
}
