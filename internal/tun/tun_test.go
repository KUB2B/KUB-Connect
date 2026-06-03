package tun

import (
	"os"
	"testing"
)

func TestStartRequiresProxyURL(t *testing.T) {
	if err := Start("tun0", ""); err == nil {
		t.Error("Start with empty proxy URL should error")
	}
}

func TestValidateArgs(t *testing.T) {
	tests := []struct {
		name     string
		device   string
		socksURL string
		wantErr  bool
	}{
		{"valid", "tun0", "socks5://127.0.0.1:10808", false},
		{"empty device", "", "socks5://127.0.0.1:10808", true},
		{"empty url", "tun0", "", true},
		{"unparseable url", "tun0", "://bad", true},
		{"wrong scheme", "tun0", "http://127.0.0.1:10808", true},
		{"no host", "tun0", "socks5://", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArgs(tt.device, tt.socksURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateArgs(%q,%q) err=%v, wantErr=%v", tt.device, tt.socksURL, err, tt.wantErr)
			}
		})
	}
}

func TestStartStopLinux(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("creating a TUN device requires root; skipping")
	}
	// Use the SOCKS proxy a running xray would expose; here just any URL —
	// the device must be created and torn down cleanly.
	if err := Start("tuntest0", "socks5://127.0.0.1:10808"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
