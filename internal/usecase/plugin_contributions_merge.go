package usecase

import domainplugin "ssh-client/internal/domain/plugin"

// MergedView is a plugin-contributed view panel.
type MergedView struct {
	PluginID string
	View     domainplugin.ViewContribution
}

// FullViewID returns the namespaced view identifier.
func (v MergedView) FullViewID() string {
	return v.PluginID + "." + v.View.ID
}

// AssetURL returns the web path for a view entry HTML asset.
func (v MergedView) AssetURL() string {
	entry := v.View.Entry
	if entry == "" {
		entry = "ui/index.html"
	}
	return "/plugin/" + v.PluginID + "/" + entry
}

// MergedStatusBarItem is a plugin-contributed status bar entry.
type MergedStatusBarItem struct {
	PluginID string
	Item     domainplugin.StatusBarContribution
}

// MergedAuthMethod is a plugin-contributed SSH auth method.
type MergedAuthMethod struct {
	PluginID string
	Method   domainplugin.AuthMethodContribution
}

// MergedTunnelProvider is a plugin-contributed dynamic forward provider.
type MergedTunnelProvider struct {
	PluginID string
	Provider domainplugin.TunnelProviderContribution
}

// MergedContributions is the full merged plugin UI contribution tree.
type MergedContributions struct {
	Commands        []MergedCommand
	Views           []MergedView
	StatusBar       []MergedStatusBarItem
	AuthMethods     []MergedAuthMethod
	TunnelProviders []MergedTunnelProvider
}

// MergeContributions collects all UI contributions from the registry.
func MergeContributions(r *PluginRegistry) MergedContributions {
	return MergeContributionsFiltered(r, nil)
}

// MergeContributionsFiltered collects contributions from enabled plugins only.
func MergeContributionsFiltered(r *PluginRegistry, enabled func(pluginID string) bool) MergedContributions {
	if r == nil {
		return MergedContributions{}
	}
	if enabled == nil {
		return MergedContributions{
			Commands:        r.Commands(),
			Views:           r.Views(),
			StatusBar:       r.StatusBarItems(),
			AuthMethods:     r.AuthMethods(),
			TunnelProviders: r.TunnelProviders(),
		}
	}
	return MergedContributions{
		Commands:        r.CommandsFiltered(enabled),
		Views:           r.ViewsFiltered(enabled),
		StatusBar:       r.StatusBarItemsFiltered(enabled),
		AuthMethods:     r.AuthMethodsFiltered(enabled),
		TunnelProviders: r.TunnelProvidersFiltered(enabled),
	}
}

// Views returns merged view contributions from all plugins.
func (r *PluginRegistry) Views() []MergedView {
	return r.ViewsFiltered(nil)
}

// ViewsFiltered returns views from plugins passing the enabled filter.
func (r *PluginRegistry) ViewsFiltered(enabled func(string) bool) []MergedView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []MergedView
	for _, p := range r.plugins {
		if enabled != nil && !enabled(p.Manifest.ID) {
			continue
		}
		for _, v := range p.Manifest.Contributions.Views {
			out = append(out, MergedView{PluginID: p.Manifest.ID, View: v})
		}
	}
	return out
}

// StatusBarItems returns merged status bar contributions sorted by priority.
func (r *PluginRegistry) StatusBarItems() []MergedStatusBarItem {
	return r.StatusBarItemsFiltered(nil)
}

// StatusBarItemsFiltered returns status bar items from enabled plugins.
func (r *PluginRegistry) StatusBarItemsFiltered(enabled func(string) bool) []MergedStatusBarItem {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []MergedStatusBarItem
	for _, p := range r.plugins {
		if enabled != nil && !enabled(p.Manifest.ID) {
			continue
		}
		for _, item := range p.Manifest.Contributions.StatusBar {
			out = append(out, MergedStatusBarItem{PluginID: p.Manifest.ID, Item: item})
		}
	}
	return out
}

// ViewEntry resolves a plugin view panel by plugin and panel ID.
func (r *PluginRegistry) ViewEntry(pluginID, panelID string) (MergedView, error) {
	for _, v := range r.Views() {
		if v.PluginID == pluginID && v.View.ID == panelID {
			return v, nil
		}
	}
	return MergedView{}, domainplugin.ErrPluginNotFound
}

// AuthMethods returns merged auth method contributions from all plugins.
func (r *PluginRegistry) AuthMethods() []MergedAuthMethod {
	return r.AuthMethodsFiltered(nil)
}

// AuthMethodsFiltered returns auth methods from enabled plugins.
func (r *PluginRegistry) AuthMethodsFiltered(enabled func(string) bool) []MergedAuthMethod {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []MergedAuthMethod
	for _, p := range r.plugins {
		if enabled != nil && !enabled(p.Manifest.ID) {
			continue
		}
		for _, am := range p.Manifest.Contributions.AuthMethods {
			out = append(out, MergedAuthMethod{PluginID: p.Manifest.ID, Method: am})
		}
	}
	return out
}

// TunnelProviders returns merged tunnel provider contributions from all plugins.
func (r *PluginRegistry) TunnelProviders() []MergedTunnelProvider {
	return r.TunnelProvidersFiltered(nil)
}

// TunnelProvidersFiltered returns tunnel providers from enabled plugins.
func (r *PluginRegistry) TunnelProvidersFiltered(enabled func(string) bool) []MergedTunnelProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []MergedTunnelProvider
	for _, p := range r.plugins {
		if enabled != nil && !enabled(p.Manifest.ID) {
			continue
		}
		for _, tp := range p.Manifest.Contributions.TunnelProviders {
			out = append(out, MergedTunnelProvider{PluginID: p.Manifest.ID, Provider: tp})
		}
	}
	return out
}
