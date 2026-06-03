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
