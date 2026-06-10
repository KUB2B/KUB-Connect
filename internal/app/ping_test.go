package app

import (
	"net"
	"strconv"
	"testing"

	"github.com/zki/vless-client/internal/vless"
)

// listenerPort returns the numeric port a 127.0.0.1:0 listener bound to.
func listenerPort(t *testing.T, ln net.Listener) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi port: %v", err)
	}
	return port
}

func TestPingInvalidIndex(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	r := svc.Ping(0) // no servers
	if r.OK || r.Error == "" {
		t.Errorf("want not-ok with error, got %+v", r)
	}
}

func TestPingReachableServer(t *testing.T) {
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
	port := listenerPort(t, ln)

	svc, _, _, _ := testDeps(t)
	svc.state.Servers = append(svc.state.Servers, &vless.ServerConfig{
		Name: "local", Host: "127.0.0.1", Port: port,
	})

	r := svc.Ping(0)
	if !r.OK {
		t.Errorf("want ok, got %+v", r)
	}
}

func TestPingUnreachableServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listenerPort(t, ln)
	ln.Close() // free the port

	svc, _, _, _ := testDeps(t)
	svc.state.Servers = append(svc.state.Servers, &vless.ServerConfig{
		Name: "dead", Host: "127.0.0.1", Port: port,
	})

	r := svc.Ping(0)
	if r.OK || r.Error == "" {
		t.Errorf("want not-ok with error, got %+v", r)
	}
}
