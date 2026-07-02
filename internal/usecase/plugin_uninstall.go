package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// UninstallPlugin stops, unregisters, and removes a user-installed plugin.
func (m *PluginManager) UninstallPlugin(ctx context.Context, pluginID string, removeData bool) error {
	if m.registry == nil {
		return fmt.Errorf("plugin registry unavailable")
	}

	pluginInfo, err := m.registry.Get(pluginID)
	if err != nil {
		return err
	}
	if pluginInfo.Source != domainplugin.SourceUser {
		return fmt.Errorf("cannot uninstall bundled plugin: %s", pluginID)
	}

	if err := m.StopPlugin(ctx, pluginID); err != nil {
		slog.Warn("failed to stop plugin before uninstall", "plugin", pluginID, "error", err)
	}

	if err := os.RemoveAll(pluginInfo.RootDir); err != nil {
		return fmt.Errorf("failed to remove plugin files: %w", err)
	}

	if removeData {
		dataDir := pluginDataDir(m.installRoot, pluginID)
		if err := os.RemoveAll(dataDir); err != nil {
			slog.Warn("failed to remove plugin data", "plugin", pluginID, "error", err)
		}
	}

	if err := m.registry.Unregister(pluginID); err != nil {
		return err
	}

	return nil
}

func pluginDataDir(dataRoot, pluginID string) string {
	safeID := strings.ReplaceAll(pluginID, string(os.PathSeparator), "_")
	return filepath.Join(dataRoot, "plugins", safeID, "data")
}
