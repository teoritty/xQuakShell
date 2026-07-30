package usecase

import (
	"fmt"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginRegistry holds discovered installed plugins.
type PluginRegistry struct {
	mu           sync.RWMutex
	plugins      map[string]domainplugin.InstalledPlugin
	protocolDefs map[string]map[string]*domainplugin.ProtocolDef // pluginID -> protocolID -> def
}

// NewPluginRegistry creates an empty plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins:      make(map[string]domainplugin.InstalledPlugin),
		protocolDefs: make(map[string]map[string]*domainplugin.ProtocolDef),
	}
}

// Load replaces the registry contents with discovered plugins.
func (r *PluginRegistry) Load(plugins []domainplugin.InstalledPlugin) error {
	next := make(map[string]domainplugin.InstalledPlugin, len(plugins))
	nextDefs := make(map[string]map[string]*domainplugin.ProtocolDef, len(plugins))
	for _, p := range plugins {
		if err := validateProtocolOwnership(next, p); err != nil {
			return err
		}
		next[p.Manifest.ID] = p
		nextDefs[p.Manifest.ID] = domainplugin.BuildProtocolDefs(&p.Manifest)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = next
	r.protocolDefs = nextDefs
	return nil
}

// Register adds or replaces a single plugin entry after validating protocol ownership.
func (r *PluginRegistry) Register(p domainplugin.InstalledPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateProtocolOwnership(r.plugins, p); err != nil {
		return err
	}
	r.plugins[p.Manifest.ID] = p
	r.protocolDefs[p.Manifest.ID] = domainplugin.BuildProtocolDefs(&p.Manifest)
	return nil
}

// Unregister removes a plugin from the registry.
func (r *PluginRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plugins[id]; !ok {
		return fmt.Errorf("%w: %s", domainplugin.ErrPluginNotFound, id)
	}
	delete(r.plugins, id)
	delete(r.protocolDefs, id)
	return nil
}

func validateProtocolOwnership(plugins map[string]domainplugin.InstalledPlugin, candidate domainplugin.InstalledPlugin) error {
	for id, existing := range plugins {
		if id == candidate.Manifest.ID {
			continue
		}
		for _, cp := range candidate.Manifest.Contributions.ConnectionProtocols {
			for _, ep := range existing.Manifest.Contributions.ConnectionProtocols {
				if cp.ID == ep.ID {
					return fmt.Errorf("%w: %s claimed by %s and %s", domainplugin.ErrInvalidManifest, cp.ID, id, candidate.Manifest.ID)
				}
			}
		}
	}
	return nil
}

// Get returns an installed plugin by ID.
func (r *PluginRegistry) Get(id string) (domainplugin.InstalledPlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	if !ok {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("%w: %s", domainplugin.ErrPluginNotFound, id)
	}
	return p, nil
}

// List returns all registered plugins sorted by ID.
func (r *PluginRegistry) List() []domainplugin.InstalledPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domainplugin.InstalledPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// PluginIDForProtocol returns the plugin ID that owns a connection protocol.
func (r *PluginRegistry) PluginIDForProtocol(protocol string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, p := range r.plugins {
		for _, cp := range p.Manifest.Contributions.ConnectionProtocols {
			if cp.ID == protocol {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("%w: protocol %s", domainplugin.ErrPluginNotFound, protocol)
}

// ConnectionProtocols returns merged protocol contributions from all plugins.
func (r *PluginRegistry) ConnectionProtocols() []domainplugin.ConnectionProtocolContribution {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domainplugin.ConnectionProtocolContribution
	seen := make(map[string]struct{})
	for _, p := range r.plugins {
		for _, cp := range p.Manifest.Contributions.ConnectionProtocols {
			if _, ok := seen[cp.ID]; ok {
				continue
			}
			seen[cp.ID] = struct{}{}
			out = append(out, cp)
		}
	}
	return out
}

// HasProtocol reports whether any plugin contributes the given connection protocol.
func (r *PluginRegistry) HasProtocol(protocol string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		for _, cp := range p.Manifest.Contributions.ConnectionProtocols {
			if cp.ID == protocol {
				return true
			}
		}
	}
	return false
}

// GetProtocolDef returns the cached protocol definition for a plugin and protocol ID.
func (r *PluginRegistry) GetProtocolDef(pluginID, protocolID string) *domainplugin.ProtocolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs, ok := r.protocolDefs[pluginID]
	if !ok {
		return nil
	}
	return defs[protocolID]
}

// ProtocolDefForConnection returns the protocol definition owning the given protocol ID.
func (r *PluginRegistry) ProtocolDefForConnection(protocolID string) (*domainplugin.ProtocolDef, string, error) {
	pluginID, err := r.PluginIDForProtocol(protocolID)
	if err != nil {
		return nil, "", err
	}
	def := r.GetProtocolDef(pluginID, protocolID)
	if def == nil {
		return nil, "", fmt.Errorf("%w: protocol %s", domainplugin.ErrPluginNotFound, protocolID)
	}
	return def, pluginID, nil
}

// DefaultPortForProtocol returns the default port from plugin manifest contributions.
func (r *PluginRegistry) DefaultPortForProtocol(protocol string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		for _, cp := range p.Manifest.Contributions.ConnectionProtocols {
			if cp.ID == protocol && cp.DefaultPort > 0 {
				return cp.DefaultPort, true
			}
		}
	}
	return 0, false
}

// PluginManifestLookup resolves manifest fields for embed validation (ADR-009).
type PluginManifestLookup interface {
	EmbedEntryForProtocol(pluginID, protocol string) (string, error)
}

// EmbedEntryForProtocol returns the expected ui entry for a plugin protocol.
func (r *PluginRegistry) EmbedEntryForProtocol(pluginID, protocol string) (string, error) {
	plugin, err := r.Get(pluginID)
	if err != nil {
		return "", err
	}
	return plugin.Manifest.EmbedEntryForProtocol(protocol), nil
}

// HasAuthProvider reports whether the plugin declares auth.provider capability.
func (r *PluginRegistry) HasAuthProvider(pluginID string) (bool, error) {
	plugin, err := r.Get(pluginID)
	if err != nil {
		return false, err
	}
	return plugin.Manifest.Capabilities.Auth != nil && plugin.Manifest.Capabilities.Auth.Provider, nil
}

// AuthMethodKind returns the manifest-declared kind for a contributed auth method.
func (r *PluginRegistry) AuthMethodKind(pluginID, authMethodID string) (string, error) {
	plugin, err := r.Get(pluginID)
	if err != nil {
		return "", err
	}
	for _, am := range plugin.Manifest.Contributions.AuthMethods {
		if am.ID == authMethodID {
			return am.Kind, nil
		}
	}
	return "", fmt.Errorf("%w: auth method %s", domainplugin.ErrPluginNotFound, authMethodID)
}

// DiscoveryPlugins lists the plugins that declared capabilities.discovery, with the connection
// protocols each one asked to be told about. Plugins without the capability are absent rather than
// filtered later: the host must never address discovery.observe to a plugin that did not declare
// it, and the cheapest way to guarantee that is to never put it in the list.
func (r *PluginRegistry) DiscoveryPlugins() []DiscoveryPluginTarget {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var targets []DiscoveryPluginTarget
	for id, p := range r.plugins {
		caps := p.Manifest.Capabilities.Discovery
		if caps == nil {
			continue
		}
		targets = append(targets, DiscoveryPluginTarget{
			PluginID:        id,
			ParentProtocols: append([]string(nil), caps.ParentProtocols...),
		})
	}
	return targets
}

// HasTunnelProvider reports whether the plugin declares tunnel.provider capability.
func (r *PluginRegistry) HasTunnelProvider(pluginID string) (bool, error) {
	plugin, err := r.Get(pluginID)
	if err != nil {
		return false, err
	}
	return plugin.Manifest.Capabilities.Tunnel != nil && plugin.Manifest.Capabilities.Tunnel.Provider, nil
}

// TunnelProviderExists reports whether providerID is contributed by the plugin.
func (r *PluginRegistry) TunnelProviderExists(pluginID, providerID string) (bool, error) {
	plugin, err := r.Get(pluginID)
	if err != nil {
		return false, err
	}
	for _, tp := range plugin.Manifest.Contributions.TunnelProviders {
		if tp.ID == providerID {
			return true, nil
		}
	}
	return false, nil
}
