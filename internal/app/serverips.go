package app

import (
	"context"
	"net"
	"time"
)

const serverLookupTimeout = 3 * time.Second

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
	// Bounded: Connect holds the service lock while this runs, so a dead
	// resolver must not freeze every other call for the OS default timeout.
	ctx, cancel := context.WithTimeout(context.Background(), serverLookupTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.String())
	}
	return out
}
