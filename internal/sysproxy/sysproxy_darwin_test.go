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

func TestDarwinClearCommands(t *testing.T) {
	got := clearCommands("Wi-Fi")
	want := [][]string{
		{"networksetup", "-setsocksfirewallproxystate", "Wi-Fi", "off"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clearCommands =\n%v\nwant\n%v", got, want)
	}
}
