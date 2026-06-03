package netcfg

import (
	"reflect"
	"testing"
)

func TestPrefixToMaskV4(t *testing.T) {
	cases := map[int]string{
		15: "255.254.0.0",
		24: "255.255.255.0",
		32: "255.255.255.255",
		0:  "0.0.0.0",
	}
	for p, want := range cases {
		if got := prefixToMaskV4(p); got != want {
			t.Errorf("prefixToMaskV4(%d) = %q, want %q", p, got, want)
		}
	}
}

func TestSplitCIDRs(t *testing.T) {
	v4, v6 := splitCIDRs([]string{"149.154.160.0/20", "2001:67c:4e8::/48", "91.108.4.0/22"})
	wantV4 := []string{"149.154.160.0/20", "91.108.4.0/22"}
	wantV6 := []string{"2001:67c:4e8::/48"}
	if !reflect.DeepEqual(v4, wantV4) {
		t.Errorf("v4 = %v, want %v", v4, wantV4)
	}
	if !reflect.DeepEqual(v6, wantV6) {
		t.Errorf("v6 = %v, want %v", v6, wantV6)
	}
}

func TestWinUpCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20", "2001:67c:4e8::/48"}}
	got := winUpCommands(c)
	want := [][]string{
		{"netsh", "interface", "ipv4", "set", "address", "name=tun0", "static", "198.18.0.1", "255.254.0.0"},
		{"netsh", "interface", "ipv4", "add", "route", "149.154.160.0/20", "interface=tun0", "store=active"},
		{"netsh", "interface", "ipv6", "add", "route", "2001:67c:4e8::/48", "interface=tun0", "store=active"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("winUpCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestWinDownCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20", "2001:67c:4e8::/48"}}
	got := winDownCommands(c)
	want := [][]string{
		{"netsh", "interface", "ipv4", "delete", "route", "149.154.160.0/20", "interface=tun0"},
		{"netsh", "interface", "ipv6", "delete", "route", "2001:67c:4e8::/48", "interface=tun0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("winDownCommands =\n%v\nwant\n%v", got, want)
	}
}
