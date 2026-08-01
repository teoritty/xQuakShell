package usecase

import (
	"fmt"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// DiscoveryIconAssetReader turns a plugin's declared discovery icon assets into data URIs, keyed by
// icon ID. Satisfied by the infra reader wired in the composition root; nil everywhere no icons are
// needed, which leaves nodes rendering without them rather than failing anything.
type DiscoveryIconAssetReader interface {
	ReadDiscoveryIcons(p domainplugin.InstalledPlugin) map[string]string
}

// PluginRegistry holds discovered installed plugins.
type PluginRegistry struct {
	iconReader DiscoveryIconAssetReader

	mu           sync.RWMutex
	plugins      map[string]domainplugin.InstalledPlugin
	protocolDefs map[string]map[string]*domainplugin.ProtocolDef // pluginID -> protocolID -> def
	// discoveryIcons caches iconID -> data URI per plugin, read once when the plugin enters the
	// registry. Icons are static files inside an installed bundle: re-reading them per ListPlugins
	// would mean up to 64 file reads per plugin every time the settings page repaints, and it is
	// also what keeps the "log an unreadable asset once per plugin" promise honest.
	discoveryIcons map[string]map[string]string
}

// NewPluginRegistry creates an empty plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins:        make(map[string]domainplugin.InstalledPlugin),
		protocolDefs:   make(map[string]map[string]*domainplugin.ProtocolDef),
		discoveryIcons: make(map[string]map[string]string),
	}
}

// SetDiscoveryIconAssetReader installs the reader used to load icon assets as plugins are
// registered. Call it before the first Load: plugins already in the registry keep whatever icons
// they were read with.
func (r *PluginRegistry) SetDiscoveryIconAssetReader(reader DiscoveryIconAssetReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.iconReader = reader
}

// readDiscoveryIcons loads one plugin's icons outside the registry lock.
//
// Outside on purpose: reading up to 64 files is unbounded I/O, and holding the registry's mutex
// across it would stall every protocol lookup, capability check and session bind in the app behind
// a disk read for decoration.
func (r *PluginRegistry) readDiscoveryIcons(p domainplugin.InstalledPlugin) map[string]string {
	r.mu.RLock()
	reader := r.iconReader
	r.mu.RUnlock()
	if reader == nil {
		return nil
	}
	return reader.ReadDiscoveryIcons(p)
}

// DiscoveryIconDataURIs returns the plugin's icons as iconID -> data URI, or nil when it declared
// none. The map is a copy: it is handed to the presentation layer, which must not be able to edit
// what every later reader sees.
func (r *PluginRegistry) DiscoveryIconDataURIs(pluginID string) map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	icons := r.discoveryIcons[pluginID]
	if len(icons) == 0 {
		return nil
	}
	out := make(map[string]string, len(icons))
	for id, uri := range icons {
		out[id] = uri
	}
	return out
}

// Load replaces the registry contents with discovered plugins.
func (r *PluginRegistry) Load(plugins []domainplugin.InstalledPlugin) error {
	next := make(map[string]domainplugin.InstalledPlugin, len(plugins))
	nextDefs := make(map[string]map[string]*domainplugin.ProtocolDef, len(plugins))
	nextIcons := make(map[string]map[string]string, len(plugins))
	for _, p := range plugins {
		if err := validateProtocolOwnership(next, p); err != nil {
			return err
		}
		next[p.Manifest.ID] = p
		nextDefs[p.Manifest.ID] = domainplugin.BuildProtocolDefs(&p.Manifest)
		if icons := r.readDiscoveryIcons(p); len(icons) > 0 {
			nextIcons[p.Manifest.ID] = icons
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = next
	r.protocolDefs = nextDefs
	r.discoveryIcons = nextIcons
	return nil
}

// Register adds or replaces a single plugin entry after validating protocol ownership.
func (r *PluginRegistry) Register(p domainplugin.InstalledPlugin) error {
	// Read before taking the lock, for the reason readDiscoveryIcons documents. A plugin whose
	// registration is then refused merely wasted the read; nothing is stored for it.
	icons := r.readDiscoveryIcons(p)

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateProtocolOwnership(r.plugins, p); err != nil {
		return err
	}
	r.plugins[p.Manifest.ID] = p
	r.protocolDefs[p.Manifest.ID] = domainplugin.BuildProtocolDefs(&p.Manifest)
	delete(r.discoveryIcons, p.Manifest.ID)
	if len(icons) > 0 {
		r.discoveryIcons[p.Manifest.ID] = icons
	}
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
	delete(r.discoveryIcons, id)
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

// DeclaredDiscoveryIcons returns the icon IDs a plugin registered in
// contributions.discoveryIcons, as a set. An unknown plugin yields an empty set rather than an
// error: the caller's answer to "not declared" and to "no such plugin" is identical — publish the
// node without an icon — and an error would only invite a second, divergent handling of the same
// outcome.
//
// It returns IDs, never asset paths. The paths were validated once at install time
// (ValidateViewAssetEntry) and have no business on the publish hot path (ADR-014 "manifest").
func (r *PluginRegistry) DeclaredDiscoveryIcons(pluginID string) map[string]struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[pluginID]
	if !ok {
		return nil
	}
	ids := make(map[string]struct{}, len(p.Manifest.Contributions.DiscoveryIcons))
	for _, icon := range p.Manifest.Contributions.DiscoveryIcons {
		ids[icon.ID] = struct{}{}
	}
	return ids
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
