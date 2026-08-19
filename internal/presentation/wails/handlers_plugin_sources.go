package wails

import (
	"fmt"
)

// ListPluginSources returns every place the plugin screen may offer to install from.
//
// The vault gate is not incidental. A source list names the repositories a user registered, which
// is part of their configuration and not public information; serving it from a locked vault would
// leak that list to anyone who reaches a locked window.
func (a *AppAPI) ListPluginSources() ([]PluginSourceDTO, error) {
	if a.pluginCatalog == nil {
		return nil, fmt.Errorf("plugin catalog not available")
	}
	if !a.IsVaultUnlocked() {
		return nil, fmt.Errorf("vault is locked")
	}

	sources, err := a.pluginCatalog.ListSources(a.reqCtx())
	if err != nil {
		return nil, err
	}

	// A non-nil empty slice: Wails marshals a nil slice to null, and the UI would have to guard
	// every iteration over it.
	dtos := make([]PluginSourceDTO, 0, len(sources))
	for i := range sources {
		dtos = append(dtos, pluginSourceToDTO(sources[i]))
	}
	return dtos, nil
}
