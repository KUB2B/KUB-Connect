//go:build windows

package netcfg

import (
	"fmt"
	"net"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// defaultGateway returns the physical IPv4 default route via Windows' native IP
// Helper API. This avoids spawning PowerShell (which used to flash a window and
// is not consistently available on Windows 7) and also accounts for both the
// route and interface metrics when several uplinks are present.
func defaultGateway() (gw, dev string, err error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return "", "", fmt.Errorf("get IPv4 route table: %w", err)
	}
	if table == nil {
		return "", "", fmt.Errorf("get IPv4 route table: empty result")
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	bestMetric := ^uint64(0)
	var best *windows.MibIpForwardRow2
	rows := table.Rows()
	for i := range rows {
		row := &rows[i]
		if !isIPv4DefaultRoute(row) {
			continue
		}

		metric := uint64(row.Metric)
		iface := windows.MibIpInterfaceRow{
			Family:         windows.AF_INET,
			InterfaceLuid:  row.InterfaceLuid,
			InterfaceIndex: row.InterfaceIndex,
		}
		if err := windows.GetIpInterfaceEntry(&iface); err == nil {
			if iface.Connected == 0 || iface.DisableDefaultRoutes != 0 {
				continue
			}
			metric += uint64(iface.Metric)
		}
		if best == nil || metric < bestMetric {
			copy := *row
			best = &copy
			bestMetric = metric
		}
	}
	if best == nil {
		return "", "", fmt.Errorf("no active IPv4 default route found")
	}

	nextHop, ok := rawIPv4(best.NextHop)
	if !ok {
		return "", "", fmt.Errorf("default route has a non-IPv4 next hop")
	}
	return nextHop.String(), strconv.FormatUint(uint64(best.InterfaceIndex), 10), nil
}

func isIPv4DefaultRoute(row *windows.MibIpForwardRow2) bool {
	if row == nil || row.InterfaceIndex == 0 || row.DestinationPrefix.PrefixLength != 0 {
		return false
	}
	ip, ok := rawIPv4(row.DestinationPrefix.Prefix)
	return ok && ip.Equal(net.IPv4zero)
}

func rawIPv4(addr windows.RawSockaddrInet) (net.IP, bool) {
	if addr.Family != windows.AF_INET {
		return nil, false
	}
	v4 := (*windows.RawSockaddrInet4)(unsafe.Pointer(&addr))
	return net.IPv4(v4.Addr[0], v4.Addr[1], v4.Addr[2], v4.Addr[3]), true
}
