package netcfg

import (
	"strings"
	"testing"
)

func joinAll(cmds [][]string) string {
	var lines []string
	for _, c := range cmds {
		lines = append(lines, strings.Join(c, " "))
	}
	return strings.Join(lines, "\n")
}

func fullCfg() Config {
	return Config{
		Device: "tun0", TunIP: "198.18.0.1", Prefix: 30,
		FullTunnel: true, BlockIPv6: true,
		ServerIPs: []string{"203.0.113.7"},
	}
}

func TestLinuxFullUpCommands(t *testing.T) {
	out := joinAll(linuxFullUpCommands(fullCfg(), "192.168.1.1", "eth0"))
	for _, want := range []string{
		"ip route add 203.0.113.7/32 via 192.168.1.1 dev eth0",
		"ip route add 0.0.0.0/1 dev tun0",
		"ip route add 128.0.0.0/1 dev tun0",
		"ip -6 route add ::/1 dev tun0",
		"ip -6 route add 8000::/1 dev tun0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("linux full up missing %q in:\n%s", want, out)
		}
	}
}

func TestLinuxFullDownCommands(t *testing.T) {
	out := joinAll(linuxFullDownCommands(fullCfg()))
	for _, want := range []string{
		"ip route del 0.0.0.0/1",
		"ip route del 128.0.0.0/1",
		"ip route del 203.0.113.7/32",
		"ip -6 route del ::/1",
		"ip -6 route del 8000::/1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("linux full down missing %q in:\n%s", want, out)
		}
	}
}

func TestWinFullUpCommands(t *testing.T) {
	out := joinAll(winFullUpCommands(fullCfg(), "192.168.1.1", "Ethernet"))
	for _, want := range []string{
		`netsh interface ipv4 add route prefix=203.0.113.7/32 interface=Ethernet nexthop=192.168.1.1 store=active`,
		`netsh interface ipv4 add route prefix=0.0.0.0/1 interface=tun0 store=active`,
		`netsh interface ipv4 add route prefix=128.0.0.0/1 interface=tun0 store=active`,
		`netsh interface ipv6 add route prefix=::/1 interface=tun0 store=active`,
		`netsh interface ipv6 add route prefix=8000::/1 interface=tun0 store=active`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("win full up missing %q in:\n%s", want, out)
		}
	}
}

func TestWinFullDownCommands(t *testing.T) {
	out := joinAll(winFullDownCommands(fullCfg(), "Ethernet"))
	for _, want := range []string{
		`netsh interface ipv4 delete route prefix=0.0.0.0/1 interface=tun0`,
		`netsh interface ipv4 delete route prefix=128.0.0.0/1 interface=tun0`,
		`netsh interface ipv4 delete route prefix=203.0.113.7/32 interface=Ethernet`,
		`netsh interface ipv6 delete route prefix=::/1 interface=tun0`,
		`netsh interface ipv6 delete route prefix=8000::/1 interface=tun0`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("win full down missing %q in:\n%s", want, out)
		}
	}
}
