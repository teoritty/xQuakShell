package plugin

import (
	"context"
	"fmt"
	"log/slog"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/ipc"
)

func initializePluginProcess(ctx context.Context, conn *ipc.Conn, plugin domainplugin.InstalledPlugin, dataDir string, portableReadOnly bool) error {
	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	if portableReadOnly {
		slog.Warn("portable data root is read-only", "pluginId", plugin.Manifest.ID)
	}

	initParams := domainplugin.InitializeParams{
		PluginID:     plugin.Manifest.ID,
		APIVersion:   coreAPIVersion,
		Capabilities: plugin.Manifest.Capabilities,
		DataDir:      dataDir,
		CoreVersion:  domainplugin.HostCoreVersion,
	}
	params, err := ipc.EncodeParams(initParams)
	if err != nil {
		return err
	}
	if _, err := conn.Call(initCtx, "initialize", params); err != nil {
		return fmt.Errorf("plugin initialize: %w", err)
	}
	return nil
}
