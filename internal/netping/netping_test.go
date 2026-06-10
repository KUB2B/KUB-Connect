package netping

import (
	"net"
	"testing"
	"time"
)

func TestPingReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	d, err := Ping(ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("Ping reachable: %v", err)
	}
	if d <= 0 || d > 2*time.Second {
		t.Errorf("latency = %v, want >0 and <2s", d)
	}
}

func TestPingClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if _, err := Ping(addr, 1*time.Second); err == nil {
		t.Error("expected error dialing a closed port")
	}
}

func TestPingBadAddr(t *testing.T) {
	if _, err := Ping("not-an-addr", time.Second); err == nil {
		t.Error("expected error for malformed address")
	}
}
