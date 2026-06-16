package netcfg

import "testing"

func TestParseWinDefaultRoute(t *testing.T) {
	cases := []struct {
		in, gw, dev string
		ok          bool
	}{
		{"192.168.1.1 12\n", "192.168.1.1", "12", true},
		{"  10.0.0.1   7  ", "10.0.0.1", "7", true},
		{"", "", "", false},
		{"0.0.0.0", "", "", false}, // missing interface token
	}
	for _, c := range cases {
		gw, dev, err := parseWinDefaultRoute(c.in)
		if c.ok && (err != nil || gw != c.gw || dev != c.dev) {
			t.Errorf("parse(%q) = %q,%q,%v want %q,%q,nil", c.in, gw, dev, err, c.gw, c.dev)
		}
		if !c.ok && err == nil {
			t.Errorf("parse(%q) expected error", c.in)
		}
	}
}
