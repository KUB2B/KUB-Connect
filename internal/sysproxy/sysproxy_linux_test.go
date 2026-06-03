//go:build linux

package sysproxy

import (
	"reflect"
	"testing"
)

func TestLinuxSetCommands(t *testing.T) {
	got := setCommands("127.0.0.1", 10808)
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", "127.0.0.1"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", "10808"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("setCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestLinuxClearCommands(t *testing.T) {
	got := clearCommands()
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "none"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clearCommands =\n%v\nwant\n%v", got, want)
	}
}
