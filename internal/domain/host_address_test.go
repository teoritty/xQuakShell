package domain

import (
	"net"
	"testing"
)

func TestHostPortDialsEveryIPv6Spelling(t *testing.T) {
	const want = "[2a0f:cdc6:500:3399::2]:22"
	for _, host := range []string{
		"2a0f:cdc6:500:3399::2",
		"2a0f:cdc6:0500:3399:0000:0000:0000:0002",
		"2A0F:CDC6:500:3399::2",
		"[2a0f:cdc6:500:3399::2]",
		"  2a0f:cdc6:500:3399::2 ",
	} {
		got := HostPort(host, 22)
		if got != want {
			t.Errorf("HostPort(%q, 22) = %q, want %q; every spelling of one address must key the same host", host, got, want)
		}
		if _, _, err := net.SplitHostPort(got); err != nil {
			t.Errorf("HostPort(%q, 22) = %q is not dialable: %v", host, got, err)
		}
	}
}

func TestHostPortKeepsShortAndScopedIPv6(t *testing.T) {
	cases := map[string]string{
		"::1":          "[::1]:2222",
		"fe80::1%eth0": "[fe80::1%eth0]:2222",
		"[::]":         "[::]:2222",
	}
	for host, want := range cases {
		if got := HostPort(host, 2222); got != want {
			t.Errorf("HostPort(%q, 2222) = %q, want %q", host, got, want)
		}
	}
}

func TestHostPortLeavesNamesAndIPv4Alone(t *testing.T) {
	cases := map[string]string{
		"example.com": "example.com:22",
		"10.0.0.1":    "10.0.0.1:22",
		" bastion ":   "bastion:22",
	}
	for host, want := range cases {
		if got := HostPort(host, 22); got != want {
			t.Errorf("HostPort(%q, 22) = %q, want %q; only IPv6 literals are rewritten", host, got, want)
		}
	}
}
