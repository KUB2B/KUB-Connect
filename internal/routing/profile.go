package routing

// Outbound tag names. Must match the tags emitted by the xrayconf builder.
const (
	OutboundProxy  = "proxy"
	OutboundDirect = "direct"
	OutboundBlock  = "block"
)

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
			Rule{Outbound: OutboundProxy, IPs: []string{"geoip:telegram"}},
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
