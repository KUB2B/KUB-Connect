//go:build linux

package netcfg

import (
	"reflect"
	"testing"
)

func TestLinuxUpCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20", "91.108.4.0/22"}}
	got := upCommands(c)
	want := [][]string{
		{"ip", "addr", "add", "198.18.0.1/15", "dev", "tun0"},
		{"ip", "link", "set", "dev", "tun0", "up"},
		{"ip", "route", "add", "149.154.160.0/20", "dev", "tun0"},
		{"ip", "route", "add", "91.108.4.0/22", "dev", "tun0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("upCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestLinuxDownCommands(t *testing.T) {
	c := Config{Device: "tun0", TunIP: "198.18.0.1", Prefix: 15,
		RouteCIDRs: []string{"149.154.160.0/20"}}
	got := downCommands(c)
	want := [][]string{
		{"ip", "route", "del", "149.154.160.0/20", "dev", "tun0"},
		{"ip", "addr", "del", "198.18.0.1/15", "dev", "tun0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("downCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestSupportedLinux(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false on linux, want true")
	}
}

// Routes that are already gone (interface went down, network changed) must not
// turn a disconnect into an error. The device does not exist, so every command
// fails whether or not the test runs as root.
func TestLinuxDownIgnoresMissingRoutes(t *testing.T) {
	c := Config{Device: "kubtest-absent0", TunIP: "198.18.0.1", Prefix: 30, RouteCIDRs: []string{"203.0.113.0/24"}}
	if err := New().Down(c); err != nil {
		t.Errorf("Down with nothing to remove = %v, want nil", err)
	}
}
