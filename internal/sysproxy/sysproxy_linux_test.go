//go:build linux

package sysproxy

import (
	"reflect"
	"testing"
)

func TestLinuxSetCommands(t *testing.T) {
	got := setCommands("127.0.0.1", 10808)
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", "127.0.0.1"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", "10808"},
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("setCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestLinuxRestoreCommands(t *testing.T) {
	got := restoreCommands(&linuxProxyState{mode: "'auto'", host: "'old'", port: "9000"})
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", "'old'"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", "9000"},
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "'auto'"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("restoreCommands =\n%v\nwant\n%v", got, want)
	}
}
