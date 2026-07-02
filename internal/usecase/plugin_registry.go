package usecase

import (
	"fmt"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginRegistry holds discovered installed plugins.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]domainplugin.InstalledPlugin
}

// NewPluginRegistry creates an empty plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{plugins: make(map[string]domainplugin.InstalledPlugin)}
}

// Load replaces the registry contents with discovered plugins.
func (r *PluginRegistry) Load(plugins []domainplugin.InstalledPlugin) error {
	next := make(map[string]domainplugin.InstalledPlugin, len(plugins))
	for _, p := range plugins {
		if err := validateProtocolOwnership(next, p); err != nil {
			return err
		}
		next[p.Manifest.ID] = p
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = next
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
