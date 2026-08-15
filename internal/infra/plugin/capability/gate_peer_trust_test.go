package capability

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func trustManifest(protocols ...string) domainplugin.Manifest {
	m := domainplugin.Manifest{ID: "com.example.rdp", Name: "RDP", Version: "1.0.0"}
	m.Engine.Type = "go-binary"
	if len(protocols) > 0 {
		m.Capabilities.Session = &domainplugin.SessionCaps{ConnectProtocols: protocols}
	}
	return m
}

// The method is open only to session plugins: asking the user about a remote identity is for the
// plugin whose connection leads there.
func TestGateAllowsVerifyPeerOnlyForSessionPlugins(t *testing.T) {
	withProtocol := NewGate(trustManifest("rdp"), domainplugin.NegotiatedDescriptor{})
	if !withProtocol.Allow("trust.verifyPeer") {
		t.Fatal("a session plugin was denied the method")
	}

	without := NewGate(trustManifest(), domainplugin.NegotiatedDescriptor{})
	if without.Allow("trust.verifyPeer") {
		t.Fatal("a plugin declaring no protocols was allowed the method")
	}
}

// A declared session block with no protocol in it is not grounds: trust binds to a connection,
// and such a plugin has none.
func TestGateRefusesVerifyPeerForEmptyProtocolList(t *testing.T) {
	m := trustManifest()
	m.Capabilities.Session = &domainplugin.SessionCaps{Embed: true}
	if NewGate(m, domainplugin.NegotiatedDescriptor{}).Allow("trust.verifyPeer") {
		t.Fatal("a plugin with embed but no protocols reached the trust mechanism")
	}
}
