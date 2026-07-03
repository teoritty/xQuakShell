package capability

import (
	domainplugin "ssh-client/internal/domain/plugin"
)

// Gate enforces manifest capabilities for plugin→core RPC methods.
type Gate struct {
	manifest domainplugin.Manifest
}

// NewGate creates a capability gate for a plugin manifest.
func NewGate(m domainplugin.Manifest) *Gate {
	return &Gate{manifest: m}
}

// Allow reports whether the plugin may invoke method.
func (g *Gate) Allow(method string) bool {
	switch method {
	case "log.write", "ping":
		return true
	case "fs.read", "fs.list":
		return g.manifest.Capabilities.FS != nil && len(g.manifest.Capabilities.FS.Read) > 0
	case "fs.write":
		return g.manifest.Capabilities.FS != nil && len(g.manifest.Capabilities.FS.Write) > 0
	case "net.dial":
		return g.hasNetworkAccess()
	case "net.close", "net.read", "net.write":
		return g.hasNetworkAccess()
	case "vault.getConnection":
		return g.manifest.Capabilities.Vault != nil && len(g.manifest.Capabilities.Vault.ReadConnectionFields) > 0
	case "vault.getSecret":
		return g.manifest.Capabilities.Vault != nil && len(g.manifest.Capabilities.Vault.GetSecret) > 0
	case "session.writeTerminal":
		return g.manifest.Capabilities.Session != nil && g.manifest.Capabilities.Session.Terminal
	case "session.updateState":
		s := g.manifest.Capabilities.Session
		return s != nil && (s.Terminal || s.Embed)
	case "session.registerEmbed", "session.tunnelOpen", "session.tunnelFrame", "session.tunnelClose":
		return g.manifest.Capabilities.Session != nil && g.manifest.Capabilities.Session.Embed
	case "session.reportLocalEmbed":
		s := g.manifest.Capabilities.Session
		return s != nil && s.Embed && s.LocalEmbedServer
	case "events.publish":
		return g.manifest.Capabilities.Events != nil && len(g.manifest.Capabilities.Events.Publish) > 0
	case "events.subscribe":
		return g.manifest.Capabilities.Events != nil && len(g.manifest.Capabilities.Events.Subscribe) > 0
	case "view.postMessage":
		return g.manifest.HasViews()
	default:
		return false
	}
}

// ValidateManifestCapabilities rejects unsafe capability patterns (Phase 2).
func ValidateManifestCapabilities(m domainplugin.Manifest) error {
	return m.ValidateCapabilities()
}

func (g *Gate) hasNetworkAccess() bool {
	n := g.manifest.Capabilities.Network
	if n == nil {
		return false
	}
	return n.AllowArbitraryOutbound || len(n.Outbound) > 0
}
