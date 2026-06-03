//go:build windows

package sysproxy

import "testing"

func TestProxyServerValue(t *testing.T) {
	got := proxyServerValue("127.0.0.1", 10808)
	want := "socks=127.0.0.1:10808"
	if got != want {
		t.Errorf("proxyServerValue = %q, want %q", got, want)
	}
}
