package netcfg

import (
	"fmt"
	"strings"
)

// parseLinuxDefaultRoute extracts the gateway and device from an
// `ip route show default` line: "default via <gw> dev <dev> ...".
func parseLinuxDefaultRoute(s string) (gw, dev string, err error) {
	// Use the first line only (there may be several default routes).
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		switch fields[i] {
		case "via":
			gw = fields[i+1]
		case "dev":
			dev = fields[i+1]
		}
	}
	if gw == "" || dev == "" {
		return "", "", fmt.Errorf("no default route found in %q", line)
	}
	return gw, dev, nil
}

// parseWinDefaultRoute parses "<gw> <interfaceIndex>" emitted by the Get-NetRoute
// PowerShell command. dev is the numeric InterfaceIndex (ASCII digits, no spaces);
// see psDefaultRoute for why we use the index rather than the localized alias.
func parseWinDefaultRoute(s string) (gw, dev string, err error) {
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", fmt.Errorf("malformed default route output %q", line)
	}
	gw = fields[0]
	dev = fields[1]
	return gw, dev, nil
}
