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

func TestParseWSTLS(t *testing.T) {
	link := "vless://uuid-1@host.net:8443" +
		"?type=ws&security=tls&sni=host.net&path=%2Fwspath&host=cdn.host.net&alpn=h2,http%2F1.1#ws"
	cfg, err := Parse(link)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Network != NetworkWS {
		t.Errorf("Network = %q, want ws", cfg.Network)
	}
	if cfg.Security != SecurityTLS {
		t.Errorf("Security = %q, want tls", cfg.Security)
	}
	if cfg.Path != "/wspath" {
		t.Errorf("Path = %q, want /wspath", cfg.Path)
	}
	if cfg.WsHost != "cdn.host.net" {
		t.Errorf("WsHost = %q, want cdn.host.net", cfg.WsHost)
	}
	if len(cfg.ALPN) != 2 || cfg.ALPN[0] != "h2" || cfg.ALPN[1] != "http/1.1" {
		t.Errorf("ALPN = %v, want [h2 http/1.1]", cfg.ALPN)
	}
}

func TestParseGRPC(t *testing.T) {
	link := "vless://uuid-2@host.net:443?type=grpc&security=tls&sni=host.net&serviceName=mygrpc#g"
	cfg, err := Parse(link)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Network != NetworkGRPC {
		t.Errorf("Network = %q, want grpc", cfg.Network)
	}
	if cfg.ServiceName != "mygrpc" {
		t.Errorf("ServiceName = %q, want mygrpc", cfg.ServiceName)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"wrong scheme":   "vmess://uuid@host:443",
		"missing uuid":   "vless://host.net:443?type=tcp",
		"bad port":       "vless://uuid@host.net:0?type=tcp",
		"reality no pbk": "vless://uuid@host.net:443?type=tcp&security=reality",
		"unknown net":    "vless://uuid@host.net:443?type=kcp&security=tls&sni=x",
	}
	for name, link := range cases {
		if _, err := Parse(link); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
