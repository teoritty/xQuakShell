package domain

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// NormalizeHost returns the one spelling of a host that every dialer, jump hop
// and known_hosts lookup agrees on.
//
// An IPv6 literal reaches here in whatever form the user typed or a config
// carried: compressed or fully expanded, upper or lower case, with or without
// the brackets URLs and ssh_config use, with a zone. netip prints the RFC 5952
// canonical form, so `2A0F:0CDC6:0500::0002` and `[2a0f:cdc6:500::2]` key the
// same known_hosts entry instead of prompting for the same server twice.
// Hostnames and IPv4 literals pass through trimmed and otherwise untouched.
func NormalizeHost(host string) string {
	h := strings.TrimSpace(host)
	if len(h) >= 2 && h[0] == '[' && h[len(h)-1] == ']' {
		h = h[1 : len(h)-1]
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.String()
	}
	return h
}

// HostPort builds a dialable "host:port" address. It is the only correct way to
// join the two: a bare `host + ":" + port` makes every IPv6 literal unparseable
// ("too many colons in address"), because only brackets separate its colons
// from the port's.
func HostPort(host string, port int) string {
	return net.JoinHostPort(NormalizeHost(host), strconv.Itoa(port))
}
