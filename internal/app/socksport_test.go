package app

import (
	"net"
	"testing"
)

func TestFreeSocksPortKeepsPreferredWhenFree(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	got, err := freeSocksPort(port)
	if err != nil {
		t.Fatalf("freeSocksPort: %v", err)
	}
	if got != port {
		t.Fatalf("freeSocksPort(%d) = %d, want the preferred port", port, got)
	}
}

func TestFreeSocksPortFallsBackWhenTaken(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	taken := l.Addr().(*net.TCPAddr).Port

	got, err := freeSocksPort(taken)
	if err != nil {
		t.Fatalf("freeSocksPort: %v", err)
	}
	if got == taken {
		t.Fatalf("freeSocksPort(%d) returned the occupied port", taken)
	}
	if !portFree(got) {
		t.Fatalf("freeSocksPort returned unusable port %d", got)
	}
}

// UDP-only occupation must also disqualify the port: the socks inbound binds
// both protocols.
func TestFreeSocksPortFallsBackWhenUDPTaken(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()
	taken := pc.LocalAddr().(*net.UDPAddr).Port

	got, err := freeSocksPort(taken)
	if err != nil {
		t.Fatalf("freeSocksPort: %v", err)
	}
	if got == taken {
		t.Fatalf("freeSocksPort(%d) returned the UDP-occupied port", taken)
	}
}
