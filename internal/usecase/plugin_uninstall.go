package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"ssh-client/internal/domain"
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

	if err := m.ensureNoConnectionsUsePlugin(ctx, pluginInfo); err != nil {
		return err
	}

	if err := m.StopPlugin(ctx, pluginID); err != nil {
		slog.Warn("failed to stop plugin before uninstall", "plugin", pluginID, "error", err)
	}

	if m.portableData == nil {
		return fmt.Errorf("portable data store unavailable")
	}

	if err := m.portableData.Remove(pluginInfo.RootDir); err != nil {
		return fmt.Errorf("failed to remove plugin files: %w", err)
	}

	if removeData {
		dataDir := pluginDataDir(m.installRoot, pluginID)
		if err := m.portableData.Remove(dataDir); err != nil {
			slog.Warn("failed to remove plugin data", "plugin", pluginID, "error", err)
		}
	}

	if err := m.registry.Unregister(pluginID); err != nil {
		return err
	}

	return nil
}

func (m *PluginManager) ensureNoConnectionsUsePlugin(ctx context.Context, plugin domainplugin.InstalledPlugin) error {
	if m.connChecker == nil {
		return nil
	}
	protocols := make(map[string]struct{})
	for _, cp := range plugin.Manifest.Contributions.ConnectionProtocols {
		protocols[cp.ID] = struct{}{}
	}
	if len(protocols) == 0 {
		return nil
	}
	conns, err := m.connChecker.GetAllConnections(ctx)
	if err != nil {
		return fmt.Errorf("check plugin connections: %w", err)
	}
	for _, conn := range conns {
		if _, ok := protocols[conn.GetProtocol()]; ok {
			return fmt.Errorf("cannot uninstall plugin: connection %q uses protocol %q", conn.Name, conn.GetProtocol())
		}
	}
	return nil
}

// PluginConnectionChecker loads persisted connections for uninstall guards.
type PluginConnectionChecker interface {
	GetAllConnections(ctx context.Context) ([]domain.Connection, error)
}

// SetConnectionChecker configures connection lookup for uninstall guards.
func (m *PluginManager) SetConnectionChecker(checker PluginConnectionChecker) {
	m.connChecker = checker
}

func pluginDataDir(dataRoot, pluginID string) string {
	safeID := strings.ReplaceAll(pluginID, string(filepath.Separator), "_")
	return filepath.Join(dataRoot, "plugins", safeID, "data")
}
