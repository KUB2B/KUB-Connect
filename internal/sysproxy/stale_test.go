package sysproxy

import "testing"

func TestLocalSocksAddr(t *testing.T) {
	for in, want := range map[string]string{
		"socks=127.0.0.1:10808": "127.0.0.1:10808",
		"socks=127.0.0.1:":      "",
		"socks=127.0.0.1:99999": "",
		"socks=10.0.0.1:1080":   "",
		"http=proxy:3128":       "",
		"":                      "",
	} {
		got, ok := localSocksAddr(in)
		if got != want || ok != (want != "") {
			t.Errorf("localSocksAddr(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
}
