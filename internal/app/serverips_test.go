package app

import "testing"

func TestResolveServerIPv4Literal(t *testing.T) {
	got := resolveServerIPv4("203.0.113.7")
	if len(got) != 1 || got[0] != "203.0.113.7" {
		t.Fatalf("literal IPv4 = %v, want [203.0.113.7]", got)
	}
}

func TestResolveServerIPv4LiteralV6Ignored(t *testing.T) {
	// A literal IPv6 server address yields no bypass entries (v6 is blocked).
	if got := resolveServerIPv4("2001:db8::1"); len(got) != 0 {
		t.Fatalf("literal IPv6 = %v, want empty", got)
	}
}

func TestResolveServerIPv4Empty(t *testing.T) {
	if got := resolveServerIPv4(""); got != nil {
		t.Fatalf("empty host = %v, want nil", got)
	}
}
