//go:build darwin

package sysproxy

import (
	"reflect"
	"testing"
)

func TestDarwinSetCommands(t *testing.T) {
	got := setCommands("Wi-Fi", "127.0.0.1", 10808)
	want := [][]string{
		{"networksetup", "-setsocksfirewallproxy", "Wi-Fi", "127.0.0.1", "10808"},
		{"networksetup", "-setsocksfirewallproxystate", "Wi-Fi", "on"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("setCommands =\n%v\nwant\n%v", got, want)
	}
}

func TestDarwinRestoreCommands(t *testing.T) {
	got := restoreCommands(&darwinProxyState{service: "Wi-Fi", host: "old", port: "9000", enabled: true})
	want := [][]string{
		{"networksetup", "-setsocksfirewallproxy", "Wi-Fi", "old", "9000"},
		{"networksetup", "-setsocksfirewallproxystate", "Wi-Fi", "on"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("restoreCommands =\n%v\nwant\n%v", got, want)
	}
}
