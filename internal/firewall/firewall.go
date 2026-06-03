// Package firewall installs a selective kill switch: it drops whitelisted
// destination CIDRs when they would leave the host through any interface other
// than the TUN device, so a tunnel failure cannot leak that traffic in
// plaintext. Everything not in the whitelist is untouched (it is meant to go
// direct under the project's selective host-route model).
package firewall

import (
	"fmt"
	"strings"
)

// tableName is the nftables table the kill switch owns end to end.
const tableName = "vless_killswitch"

// Config describes one kill-switch session.
type Config struct {
	Device string   // TUN device whose egress is allowed, e.g. "tun0"
	CIDRs  []string // whitelisted destination CIDRs to protect
}

// Firewall installs and removes the kill switch.
type Firewall interface {
	On(c Config) error
	Off() error
}

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

// buildRuleset renders the nftables ruleset. A family's drop line is emitted
// only when that family has at least one CIDR.
func buildRuleset(device string, v4, v6 []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s {\n", tableName)
	b.WriteString("\tchain out {\n")
	b.WriteString("\t\ttype filter hook output priority 0; policy accept;\n")
	if len(v4) > 0 {
		fmt.Fprintf(&b, "\t\toifname != %q ip daddr { %s } drop\n", device, strings.Join(v4, ", "))
	}
	if len(v6) > 0 {
		fmt.Fprintf(&b, "\t\toifname != %q ip6 daddr { %s } drop\n", device, strings.Join(v6, ", "))
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}
