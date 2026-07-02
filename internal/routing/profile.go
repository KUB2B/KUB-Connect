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

// Preset is a bundled service routed through the VPN when selected in
// whitelist mode. Domains are xray domain-rule entries (geosite: categories
// from the shipped geosite.dat).
type Preset struct {
	Key     string
	Title   string
	Domains []string
}

// Presets is the fixed set of service presets offered in the UI. Keys are
// persisted in Profile.ProxyPresets, so they must stay stable.
var Presets = []Preset{
	{Key: "youtube", Title: "YouTube", Domains: []string{"geosite:youtube"}},
	{Key: "instagram", Title: "Instagram", Domains: []string{"geosite:instagram"}},
	{Key: "facebook", Title: "Facebook", Domains: []string{"geosite:facebook"}},
	{Key: "twitter", Title: "Twitter / X", Domains: []string{"geosite:twitter"}},
	{Key: "discord", Title: "Discord", Domains: []string{"geosite:discord"}},
	{Key: "netflix", Title: "Netflix", Domains: []string{"geosite:netflix"}},
	{Key: "spotify", Title: "Spotify", Domains: []string{"geosite:spotify"}},
	{Key: "openai", Title: "ChatGPT (OpenAI)", Domains: []string{"geosite:openai"}},
	{Key: "anthropic", Title: "Claude (Anthropic)", Domains: []string{"geosite:anthropic"}},
}

// PresetByKey returns the preset with the given key, or false.
func PresetByKey(key string) (Preset, bool) {
	for _, p := range Presets {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// Profile is the user's routing choices. Full selects the full-tunnel model
// (everything through the VPN); otherwise the whitelist model applies.
type Profile struct {
	Full               bool
	Telegram           bool
	ForceRUDirect      bool
	CustomProxyDomains []string
	CustomProxyIPs     []string
	// ProxyPresets are Preset keys routed through the VPN in whitelist mode
	// (subsumed by the catch-all proxy rule in full mode).
	ProxyPresets []string
	// CustomDirectDomains/IPs are user exceptions that always bypass the VPN.
	// They win over every proxy rule in both models.
	CustomDirectDomains []string
	CustomDirectIPs     []string
}

// presetDomains aggregates the domain entries of the selected presets,
// silently skipping unknown keys (stale state after a preset is removed).
func (p Profile) presetDomains() []string {
	var domains []string
	for _, key := range p.ProxyPresets {
		if preset, ok := PresetByKey(key); ok {
			domains = append(domains, preset.Domains...)
		}
	}
	return domains
}

// directExceptions returns the user's bypass rules, emitted first so they win
// over every proxy match.
func (p Profile) directExceptions() []Rule {
	var rules []Rule
	if len(p.CustomDirectDomains) > 0 {
		rules = append(rules, Rule{Outbound: OutboundDirect, Domains: p.CustomDirectDomains})
	}
	if len(p.CustomDirectIPs) > 0 {
		rules = append(rules, Rule{Outbound: OutboundDirect, IPs: p.CustomDirectIPs})
	}
	return rules
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
	if p.Full {
		return p.fullRules()
	}

	var rules []Rule
	rules = append(rules, p.directExceptions()...)

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
	if domains := p.presetDomains(); len(domains) > 0 {
		rules = append(rules, Rule{Outbound: OutboundProxy, Domains: domains})
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

// fullRules is the full-tunnel rule set: keep LAN (and optionally RU) direct,
// blackhole all IPv6 (it is captured into the TUN but the server path is IPv4
// only — see netcfg BlockIPv6), and send everything else to the proxy.
//
// Under TUN split-default routes, direct rules for public destinations (RU,
// custom exceptions) only work when the direct outbound is bound to the
// physical interface (xrayconf Options.BindInterface); an unbound direct dial
// would be routed back into the TUN and loop.
func (p Profile) fullRules() []Rule {
	var rules []Rule
	rules = append(rules, p.directExceptions()...)
	if p.ForceRUDirect {
		rules = append(rules,
			Rule{Outbound: OutboundDirect, Domains: []string{"geosite:category-ru"}},
			Rule{Outbound: OutboundDirect, IPs: []string{"geoip:ru"}},
		)
	}
	rules = append(rules, Rule{Outbound: OutboundDirect, IPs: []string{"geoip:private"}})
	rules = append(rules, Rule{Outbound: OutboundBlock, IPs: []string{"::/0"}})
	rules = append(rules, Rule{Outbound: OutboundProxy, Network: "tcp,udp"})
	return rules
}

// Default returns the shipped default profile: Telegram on, RU forced direct.
func Default() Profile {
	return Profile{Telegram: true, ForceRUDirect: true}
}
