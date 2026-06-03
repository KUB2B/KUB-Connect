package vless

import "testing"

func TestParseRealityTCP(t *testing.T) {
	link := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@example.com:443" +
		"?type=tcp&security=reality&pbk=ABCpublicKey&fp=chrome&sni=www.microsoft.com" +
		"&sid=0123abcd&spx=%2F&flow=xtls-rprx-vision#my%20server"

	cfg, err := Parse(link)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Name", cfg.Name, "my server"},
		{"Host", cfg.Host, "example.com"},
		{"UUID", cfg.UUID, "b831381d-6324-4d53-ad4f-8cda48b30811"},
		{"Flow", cfg.Flow, "xtls-rprx-vision"},
		{"Security", string(cfg.Security), "reality"},
		{"Network", string(cfg.Network), "tcp"},
		{"SNI", cfg.SNI, "www.microsoft.com"},
		{"Fingerprint", cfg.Fingerprint, "chrome"},
		{"PublicKey", cfg.PublicKey, "ABCpublicKey"},
		{"ShortID", cfg.ShortID, "0123abcd"},
		{"SpiderX", cfg.SpiderX, "/"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if cfg.Port != 443 {
		t.Errorf("Port = %d, want 443", cfg.Port)
	}
}
