package plugin

import (
	"context"
	"fmt"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/ipc"
)

// initializeEnv is everything the handshake says about one process start.
//
// It is a struct rather than a parameter list because the list had already reached seven and was
// baselined there; the fields that vary per start belong together anyway, and grouping them is what
// made room to tell the plugin the interface language.
type initializeEnv struct {
	plugin domainplugin.InstalledPlugin
	// dataDir is this plugin's writable directory.
	dataDir string
	// locale is the interface language, empty when the host has none to report yet.
	locale           string
	portableReadOnly bool
	negotiated       domainplugin.NegotiatedDescriptor
	warnings         []string
}

// initializePluginProcess sends the initialize handshake. Compatibility was already resolved into
// `negotiated` (by Negotiate in Start, against the live registry) and any migration `warnings`
// collected there; this function only surfaces those advisories and deprecation notices — one per
// load — and sends the descriptor. Deprecated items still work (ADR-012); notices never block.
func initializePluginProcess(ctx context.Context, conn *ipc.Conn, env initializeEnv) error {
	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	plugin := env.plugin
	if env.portableReadOnly {
		slog.Warn("portable data root is read-only", "pluginId", plugin.Manifest.ID)
	}

	for _, w := range env.warnings {
		slog.Warn("plugin API advisory", "pluginId", plugin.Manifest.ID, "detail", w)
	}
	for _, n := range env.negotiated.DeprecationNotices(domainplugin.HostRegistry()) {
		slog.Warn("plugin uses deprecated API", "pluginId", plugin.Manifest.ID, "detail", n)
	}

	initParams := domainplugin.InitializeParams{
		PluginID:     plugin.Manifest.ID,
		APIVersion:   domainplugin.PluginAPIVersion,
		API:          domainplugin.HostDescriptor(),
		Capabilities: plugin.Manifest.Capabilities,
		DataDir:      env.dataDir,
		CoreVersion:  domainplugin.HostCoreVersion,
		Locale:       env.locale,
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
