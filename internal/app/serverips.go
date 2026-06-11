package app

import "net"

// resolveServerIPv4 returns the IPv4 addresses to bypass the tunnel for (so the
// encrypted connection to the server does not loop back into the TUN). A literal
// IPv4 host is returned as-is; a hostname is resolved via DNS. IPv6 addresses are
// dropped because full-tunnel mode blocks IPv6. Returns nil on empty input or
// resolution failure (the caller proceeds without a bypass, logging upstream).
func resolveServerIPv4(host string) []string {
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return []string{ip4.String()}
		}
		return nil // literal IPv6: nothing to bypass
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		if ip4 := a.To4(); ip4 != nil {
			out = append(out, ip4.String())
		}
	}
	return out
}
