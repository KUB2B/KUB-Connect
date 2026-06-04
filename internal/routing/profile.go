package routing

// Outbound tag names. Must match the tags emitted by the xrayconf builder.
const (
	OutboundProxy  = "proxy"
	OutboundDirect = "direct"
	OutboundBlock  = "block"
)

// TUNReservedCIDR is the RFC 2544 benchmarking range used for the TUN adapter's
// address and on-link subnet. xray blackholes it so a packet addressed to the
// TUN's own subnet is never re-dialed by the direct outbound back into the TUN
// (which would form an amplifying routing loop). Nothing real uses this range,
// so blocking it is harmless in proxy mode too.
const TUNReservedCIDR = "198.18.0.0/15"

// TelegramCIDRs are Telegram's officially published IP ranges (AS62014 /
// AS62041). Baked in rather than relying on a "geoip:telegram" category,
// which is absent from the canonical v2fly geoip.dat. This keeps Telegram
// tunneling robust regardless of which geoip.dat the user ships.
// Source: https://core.telegram.org/resources/cidr.txt
var TelegramCIDRs = []string{
	"91.105.192.0/23",
	"91.108.4.0/22",
	"91.108.8.0/22",
	"91.108.12.0/22",
	"91.108.16.0/22",
	"91.108.20.0/22",
	"91.108.56.0/22",
	"95.161.64.0/20",
	"149.154.160.0/20",
	"185.76.151.0/24",
	"2001:67c:4e8::/48",
	"2001:b28:f23c::/48",
	"2001:b28:f23d::/48",
	"2001:b28:f23f::/48",
	"2a0a:f280::/32",
}

// Profile is the user's whitelist routing choices.
type Profile struct {
	Telegram           bool
	ForceRUDirect      bool
	CustomProxyDomains []string
	CustomProxyIPs     []string
}

// Rule is one xray routing rule, outbound-tagged.
type Rule struct {
	Outbound string
	Domains  []string
	IPs      []string
	Network  string // e.g. "tcp,udp"; empty when matching by domain/ip
}

// Rules returns the ordered rule list. Order encodes priority:
// forced-direct RU first, then private, then proxy matches, then a
// catch-all direct (whitelist: anything unmatched goes direct).
func (p Profile) Rules() []Rule {
	var rules []Rule

	if p.ForceRUDirect {
		rules = append(rules,
			Rule{Outbound: OutboundDirect, Domains: []string{"geosite:category-ru"}},
			Rule{Outbound: OutboundDirect, IPs: []string{"geoip:ru"}},
		)
	}
	rules = append(rules, Rule{Outbound: OutboundDirect, IPs: []string{"geoip:private"}})

	if p.Telegram {
		rules = append(rules,
			Rule{Outbound: OutboundProxy, Domains: []string{"geosite:telegram"}},
			Rule{Outbound: OutboundProxy, IPs: TelegramCIDRs},
		)
	}
	if len(p.CustomProxyDomains) > 0 {
		rules = append(rules, Rule{Outbound: OutboundProxy, Domains: p.CustomProxyDomains})
	}
	if len(p.CustomProxyIPs) > 0 {
		rules = append(rules, Rule{Outbound: OutboundProxy, IPs: p.CustomProxyIPs})
	}

	rules = append(rules, Rule{Outbound: OutboundDirect, Network: "tcp,udp"})
	return rules
}

// Default returns the shipped default profile: Telegram on, RU forced direct.
func Default() Profile {
	return Profile{Telegram: true, ForceRUDirect: true}
}
