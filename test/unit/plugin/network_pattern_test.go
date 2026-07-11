package plugin_test

import (
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

// assertNetworkPatternRejected is shared across proto prefixes so the wildcard/port/ambiguity
// rejection rules are proven identical for tcp: and udp:, rather than duplicated per-proto.
func assertNetworkPatternRejected(t *testing.T, proto, suffix string) {
	t.Helper()
	pattern := proto + ":" + suffix
	if _, err := domainplugin.ParseNetworkPattern(pattern); err == nil {
		t.Fatalf("expected reject for pattern %q", pattern)
	}
}

func assertNetworkPatternAccepted(t *testing.T, proto, suffix string) {
	t.Helper()
	pattern := proto + ":" + suffix
	got, err := domainplugin.ParseNetworkPattern(pattern)
	if err != nil {
		t.Fatalf("expected accept for %q: %v", pattern, err)
	}
	if got.Proto != proto {
		t.Fatalf("ParseNetworkPattern(%q).Proto = %q, want %q", pattern, got.Proto, proto)
	}
}

func TestParseNetworkPatternRejectsPortOnly(t *testing.T) {
	suffixes := []string{"443", "*", ""}
	for _, proto := range []string{"tcp", "udp"} {
		for _, suffix := range suffixes {
			assertNetworkPatternRejected(t, proto, suffix)
		}
		// host-only, no port at all.
		assertNetworkPatternRejected(t, proto, "example.com")
	}
	if _, err := domainplugin.ParseNetworkPattern("443"); err == nil {
		t.Fatal("expected reject for pattern with no proto prefix")
	}
}

func TestParseNetworkPatternAcceptsHostPort(t *testing.T) {
	suffixes := []string{"127.0.0.1:443", "example.com:22", "localhost:8000-9000"}
	for _, proto := range []string{"tcp", "udp"} {
		for _, suffix := range suffixes {
			assertNetworkPatternAccepted(t, proto, suffix)
		}
	}
}

func TestParseNetworkPatternSetsProto(t *testing.T) {
	tcp, err := domainplugin.ParseNetworkPattern("tcp:example.com:22")
	if err != nil || tcp.Proto != "tcp" {
		t.Fatalf("tcp:example.com:22 => %+v, %v", tcp, err)
	}
	udp, err := domainplugin.ParseNetworkPattern("udp:example.com:53")
	if err != nil || udp.Proto != "udp" {
		t.Fatalf("udp:example.com:53 => %+v, %v", udp, err)
	}
}

func TestParseNetworkPatternRejectsAmbiguousHostPort(t *testing.T) {
	for _, proto := range []string{"tcp", "udp"} {
		assertNetworkPatternRejected(t, proto, "1234:5678")
	}
}

func TestParseNetworkPatternRejectsUnrecognizedProto(t *testing.T) {
	cases := []string{"host:port", "ftp:example.com:21", "example.com:22"}
	for _, pattern := range cases {
		if _, err := domainplugin.ParseNetworkPattern(pattern); err == nil {
			t.Fatalf("expected reject for pattern %q with no recognized proto prefix", pattern)
		}
	}
}

func TestParseNetworkPatternRejectsBadPort(t *testing.T) {
	for _, proto := range []string{"tcp", "udp"} {
		assertNetworkPatternRejected(t, proto, "example.com:0")
		assertNetworkPatternRejected(t, proto, "example.com:70000")
		assertNetworkPatternRejected(t, proto, "example.com:9000-8000")
	}
}

func TestValidateManifestRejectsPortOnlyNetwork(t *testing.T) {
	m := domainplugin.Manifest{
		ID: "com.test.portonly", Name: "x", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Network: &domainplugin.NetworkCaps{Outbound: []string{"tcp:443"}},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected tcp:443 to be rejected at manifest validation")
	}
}

func TestParseNetworkPatternErrorMentionsProto(t *testing.T) {
	_, err := domainplugin.ParseNetworkPattern("udp:")
	if err == nil {
		t.Fatal("expected reject for udp: with no host:port")
	}
	if !strings.Contains(err.Error(), "wildcards") {
		// Empty rest after "udp:" is treated as the wildcard-rejection branch (rest == "").
		t.Fatalf("unexpected error for udp: %v", err)
	}
}
