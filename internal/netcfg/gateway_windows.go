//go:build windows

package netcfg

import (
	"fmt"
	"os/exec"
)

// psDefaultRoute prints "<NextHop> <InterfaceAlias>" for the lowest-metric
// IPv4 default route. Wrapped in a single line so the output is trivial to parse.
const psDefaultRoute = `$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction Stop | Sort-Object RouteMetric | Select-Object -First 1; "$($r.NextHop) $($r.InterfaceAlias)"`

// defaultGateway returns the physical default route's gateway IP and interface
// alias via PowerShell's Get-NetRoute, before the split-default routes shadow it.
func defaultGateway() (gw, dev string, err error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psDefaultRoute).Output()
	if err != nil {
		return "", "", fmt.Errorf("get-netroute default: %w", err)
	}
	return parseWinDefaultRoute(string(out))
}
