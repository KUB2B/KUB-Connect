package routing

import "testing"

func TestRulesWhitelistDefault(t *testing.T) {
	p := Profile{
		Telegram:           true,
		ForceRUDirect:      true,
		CustomProxyDomains: []string{"openai.com"},
		CustomProxyIPs:     []string{"1.2.3.4/32"},
	}
	rules := p.Rules()

	if rules[0].Outbound != OutboundDirect {
		t.Fatalf("rule[0].Outbound = %q, want direct", rules[0].Outbound)
	}
	if !contains(rules[0].Domains, "geosite:category-ru") {
		t.Errorf("rule[0] domains = %v, want geosite:category-ru", rules[0].Domains)
	}

	last := rules[len(rules)-1]
	if last.Outbound != OutboundDirect || last.Network != "tcp,udp" {
		t.Errorf("last rule = %+v, want catch-all direct tcp,udp", last)
	}

	if !hasProxyDomain(rules, "geosite:telegram") {
		t.Error("expected geosite:telegram -> proxy")
	}
	if !hasProxyDomain(rules, "openai.com") {
		t.Error("expected openai.com -> proxy")
	}
}

func TestRulesTelegramOff(t *testing.T) {
	p := Profile{Telegram: false, ForceRUDirect: true}
	if hasProxyDomain(p.Rules(), "geosite:telegram") {
		t.Error("telegram off: must not route telegram to proxy")
	}
}

func TestRulesRUOff(t *testing.T) {
	p := Profile{Telegram: true, ForceRUDirect: false}
	rules := p.Rules()
	for _, r := range rules {
		if contains(r.Domains, "geosite:category-ru") {
			t.Error("ForceRUDirect off: must not emit category-ru rule")
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func hasProxyDomain(rules []Rule, domain string) bool {
	for _, r := range rules {
		if r.Outbound == OutboundProxy && contains(r.Domains, domain) {
			return true
		}
	}
	return false
}

func TestRulesFullTunnel(t *testing.T) {
	p := Profile{Full: true, Telegram: true, ForceRUDirect: true,
		CustomProxyDomains: []string{"example.com"}}
	rules := p.Rules()

	// Last rule is the catch-all and must now go to proxy.
	last := rules[len(rules)-1]
	if last.Outbound != OutboundProxy || last.Network != "tcp,udp" {
		t.Fatalf("full catch-all = %+v, want proxy tcp,udp", last)
	}

	// Telegram and custom-proxy rules are subsumed and must NOT appear.
	for _, r := range rules {
		for _, d := range r.Domains {
			if d == "geosite:telegram" || d == "example.com" {
				t.Errorf("full mode should omit whitelist proxy rule: %+v", r)
			}
		}
	}

	// RU-Direct and private direct exceptions are kept, before the catch-all.
	var sawRU, sawPrivate, sawV6Block bool
	for _, r := range rules {
		if r.Outbound == OutboundDirect {
			for _, ip := range r.IPs {
				if ip == "geoip:ru" {
					sawRU = true
				}
				if ip == "geoip:private" {
					sawPrivate = true
				}
			}
		}
		if r.Outbound == OutboundBlock {
			for _, ip := range r.IPs {
				if ip == "::/0" {
					sawV6Block = true
				}
			}
		}
	}
	if !sawRU || !sawPrivate || !sawV6Block {
		t.Errorf("full mode missing exception/block rules: ru=%v private=%v v6block=%v",
			sawRU, sawPrivate, sawV6Block)
	}
}

func TestRulesWhitelistUnchanged(t *testing.T) {
	// Regression guard: default (whitelist) catch-all stays direct.
	p := Default()
	rules := p.Rules()
	last := rules[len(rules)-1]
	if last.Outbound != OutboundDirect || last.Network != "tcp,udp" {
		t.Fatalf("whitelist catch-all = %+v, want direct tcp,udp", last)
	}
}
