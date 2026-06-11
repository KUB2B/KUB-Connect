package netcfg

import "testing"

func TestParseLinuxDefaultRoute(t *testing.T) {
	cases := []struct {
		in, gw, dev string
		ok          bool
	}{
		{"default via 192.168.1.1 dev eth0 proto dhcp metric 100", "192.168.1.1", "eth0", true},
		{"default via 10.0.0.1 dev wlan0\n", "10.0.0.1", "wlan0", true},
		{"", "", "", false},
		{"something unexpected", "", "", false},
	}
	for _, c := range cases {
		gw, dev, err := parseLinuxDefaultRoute(c.in)
		if c.ok && (err != nil || gw != c.gw || dev != c.dev) {
			t.Errorf("parse(%q) = %q,%q,%v want %q,%q,nil", c.in, gw, dev, err, c.gw, c.dev)
		}
		if !c.ok && err == nil {
			t.Errorf("parse(%q) expected error", c.in)
		}
	}
}
