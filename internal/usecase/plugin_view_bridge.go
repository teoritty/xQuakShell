package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
)

// PostViewMessage forwards a UI message to a plugin view panel.
func (m *PluginManager) PostViewMessage(ctx context.Context, pluginID, panelID string, message json.RawMessage) error {
	if _, err := m.registry.ViewEntry(pluginID, panelID); err != nil {
		return err
	}

	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return err
	}
	if !plugin.Manifest.HasViews() {
		return domainplugin.ErrCapabilityDenied
	}

	if err := m.AuthorizeStart(pluginID, StartReasonView, panelID); err != nil {
		return err
	}

	if err := m.EnsureRunning(ctx, pluginID); err != nil {
		return err
	}
	m.TouchActivity(pluginID)

	params, err := json.Marshal(map[string]any{
		"panelId": panelID,
		"message": json.RawMessage(message),
	})
	if err != nil {
		return fmt.Errorf("encode view.postMessage: %w", err)
	}
	return m.NotifyProcess(ctx, pluginID, "", "view.postMessage", params)
}

// ResolvePluginAssetRoot returns the on-disk root for plugin static assets.
func (m *PluginManager) ResolvePluginAssetRoot(pluginID string) (string, error) {
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return "", err
	}
	return plugin.RootDir, nil
}
