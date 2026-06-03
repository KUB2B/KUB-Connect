package firewall

import (
	"strings"
	"testing"
)

func TestSplitCIDRs(t *testing.T) {
	v4, v6 := splitCIDRs([]string{"149.154.160.0/20", "2001:db8::/32", "203.0.113.0/24"})
	if len(v4) != 2 || v4[0] != "149.154.160.0/20" || v4[1] != "203.0.113.0/24" {
		t.Errorf("v4 = %v", v4)
	}
	if len(v6) != 1 || v6[0] != "2001:db8::/32" {
		t.Errorf("v6 = %v", v6)
	}
}

func TestBuildRulesetV4Only(t *testing.T) {
	rs := buildRuleset("tun0", []string{"149.154.160.0/20"}, nil)
	if !strings.Contains(rs, "table inet vless_killswitch") {
		t.Errorf("missing table name:\n%s", rs)
	}
	if !strings.Contains(rs, `oifname != "tun0" ip daddr { 149.154.160.0/20 } drop`) {
		t.Errorf("missing v4 drop rule:\n%s", rs)
	}
	if strings.Contains(rs, "ip6 daddr") {
		t.Errorf("should not emit ip6 rule when no v6 CIDRs:\n%s", rs)
	}
}

func TestBuildRulesetBothFamilies(t *testing.T) {
	rs := buildRuleset("tun0", []string{"203.0.113.0/24"}, []string{"2001:db8::/32"})
	if !strings.Contains(rs, `oifname != "tun0" ip daddr { 203.0.113.0/24 } drop`) {
		t.Errorf("missing v4 rule:\n%s", rs)
	}
	if !strings.Contains(rs, `oifname != "tun0" ip6 daddr { 2001:db8::/32 } drop`) {
		t.Errorf("missing v6 rule:\n%s", rs)
	}
}
