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
