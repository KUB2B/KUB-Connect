//go:build windows

package netcfg

import (
	"fmt"
	"os/exec"
)

// psDefaultRoute prints "<NextHop> <InterfaceIndex>" for the lowest-metric IPv4
// default route. We emit the numeric InterfaceIndex (not the alias) on purpose:
// PowerShell stdout uses the OEM codepage (e.g. cp866 on Russian Windows), so a
// non-ASCII alias like "Беспроводная сеть" comes back as mangled bytes that
// netsh then rejects with ERROR_INVALID_NAME. The index is pure ASCII digits and
// is codepage-immune, and netsh's interface= accepts an index as readily as a
// name. Wrapped in a single line so the output is trivial to parse.
const psDefaultRoute = `$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction Stop | Sort-Object RouteMetric | Select-Object -First 1; "$($r.NextHop) $($r.InterfaceIndex)"`

// defaultGateway returns the physical default route's gateway IP and interface
// index via PowerShell's Get-NetRoute, before the split-default routes shadow it.
func defaultGateway() (gw, dev string, err error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psDefaultRoute).Output()
	if err != nil {
		return "", "", fmt.Errorf("get-netroute default: %w", err)
	}
	return parseWinDefaultRoute(string(out))
}
