package plugin_test

import (
	"net"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestAllowResolvedDialIPBlocksPrivateResolution(t *testing.T) {
	if domainplugin.AllowResolvedDialIP("evil.example", net.ParseIP("127.0.0.1")) {
		t.Fatal("expected hostname resolving to loopback to be denied")
	}
	if domainplugin.AllowResolvedDialIP("evil.example", net.ParseIP("10.0.0.5")) {
		t.Fatal("expected hostname resolving to private IP to be denied")
	}
	if domainplugin.AllowResolvedDialIP("evil.example", net.ParseIP("169.254.169.254")) {
		t.Fatal("expected link-local metadata IP to be denied")
	}
}

func TestAllowResolvedDialIPPermitsExplicitLoopbackPattern(t *testing.T) {
	if !domainplugin.AllowResolvedDialIP("127.0.0.1", net.ParseIP("127.0.0.1")) {
		t.Fatal("expected explicit loopback allowlist entry")
	}
}

func TestAllowResolvedDialIPPermitsPublicIPs(t *testing.T) {
	if !domainplugin.AllowResolvedDialIP("example.com", net.ParseIP("93.184.216.34")) {
		t.Fatal("expected public IP to be allowed")
	}
}
