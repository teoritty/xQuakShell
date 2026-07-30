package capability

import (
	domainplugin "xquakshell/internal/domain/plugin"
)

// Gate enforces manifest capabilities for plugin→core RPC methods.
//
// Two orthogonal checks apply (ADR-012). First, the capability GRANT: whether the plugin's
// manifest permits the method at all — this is the authorization boundary and is unchanged.
// Second, the negotiated VERSION: if a method belongs to a feature introduced above a
// capability's baseline, the plugin must have negotiated a capability version that includes it.
// At the 1.0 freeze every method is baseline, so the version check never denies; it is the
// forward-looking mechanism that lets a future minor add methods without exposing them to plugins
// that negotiated an older version (edge #13).
type Gate struct {
	manifest        domainplugin.Manifest
	negotiated      domainplugin.NegotiatedDescriptor
	featureVersions map[string]methodFeature
}

// methodFeature records that an RPC method was introduced at minVersion of a capability. A method
// with no entry is baseline and is governed by the capability grant alone.
type methodFeature struct {
	capability domainplugin.CapabilityID
	minVersion domainplugin.Semver
}

// NewGate creates a capability gate for a plugin manifest and its negotiated runtime contract. The
// manifest supplies the capability GRANTS (authorization); the negotiated descriptor supplies the
// VERSIONS the plugin agreed to (feature reachability). The descriptor is computed once at
// handshake and threaded in — the gate never re-derives it, so there is a single source of truth.
func NewGate(m domainplugin.Manifest, negotiated domainplugin.NegotiatedDescriptor) *Gate {
	return &Gate{manifest: m, negotiated: negotiated, featureVersions: defaultFeatureVersions()}
}

// defaultFeatureVersions maps methods introduced above their capability's 1.0 baseline to the
// version that introduced them. Empty at the 1.0 freeze: every current method is baseline. Add an
// entry here (never remove one within a major) when a minor bump introduces a new method.
func defaultFeatureVersions() map[string]methodFeature {
	return map[string]methodFeature{}
}

// versionAllows reports whether the plugin's negotiated capability version covers a method that
// was introduced above baseline. Baseline methods (no entry) always pass this check.
func (g *Gate) versionAllows(method string) bool {
	mf, ok := g.featureVersions[method]
	if !ok {
		return true
	}
	have, ok := g.negotiated.CapabilityVersion(mf.capability)
	if !ok {
		return false
	}
	return domainplugin.Satisfies(have, mf.minVersion)
}

// Allow reports whether the plugin may invoke method.
func (g *Gate) Allow(method string) bool {
	if !g.versionAllows(method) {
		return false
	}
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
	case "tunnel.dial", "tunnel.close", "tunnel.localWrite", "tunnel.localClose", "tunnel.bind":
		return g.manifest.Capabilities.Tunnel != nil && g.manifest.Capabilities.Tunnel.Provider
	case "channel.open", "channel.close":
		return g.manifest.Capabilities.Channel != nil && len(g.manifest.Capabilities.Channel.Purposes) > 0
	case "discovery.publish":
		// The only discovery verb a plugin may call. observe and invokeAction travel host->plugin
		// and never reach this gate at all: the host declines to address them to a plugin without
		// the capability (PluginRegistry.DiscoveryPlugins), which is an addressing decision rather
		// than a denial (ADR-014 "Security model").
		//
		// The grant is the capability's presence, with no sub-field to check. parentProtocols
		// governs which connections the plugin is told about, not whether it may publish — a
		// publish is already confined to a session the plugin holds a binding for, and that
		// session's protocol is fixed by the connection behind it.
		return g.manifest.Capabilities.Discovery != nil
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
