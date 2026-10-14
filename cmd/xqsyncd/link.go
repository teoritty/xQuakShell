package main

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"strings"
)

// A connect link is the server's address with its identity fingerprint in the fragment:
//
//	https://sync.example.org:8443/#fp=91796-59138-05751-…
//
// An operator who hands it out has done the comparison for the user: the client probes the server,
// computes the fingerprint of the identity key it was offered and refuses to continue unless it is
// the one in the link - the same check a person makes by reading twelve groups aloud, done without
// a typo. The fragment never leaves the client: HTTP sends no fragment, so the server cannot learn
// or strip it, and a link that reached the user through a channel the attacker controls is no
// worse than a fingerprint read out over that channel.

// defaultLinkPort is the port the link names when the command cannot know the listening one.
const defaultLinkPort = "8443"

// connectLink builds the link for host and port.
func connectLink(host, port, fingerprint string) string {
	return "https://" + net.JoinHostPort(host, port) + "/#fp=" + strings.ReplaceAll(fingerprint, " ", "-")
}

// certHost is the first name or address a certificate is issued for, "localhost" when it names
// none or does not parse - the link is a convenience and must not stop a server from starting.
func certHost(cert tls.Certificate) string {
	if len(cert.Certificate) == 0 {
		return "localhost"
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return "localhost"
	}
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames[0]
	}
	if len(leaf.IPAddresses) > 0 {
		return leaf.IPAddresses[0].String()
	}
	return "localhost"
}

// portOf is the port of a listening address, defaultLinkPort when it has none.
func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	return defaultLinkPort
}
