package main

import (
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infrapluginembed "xquakshell/internal/infra/embed"
	infrapluginassets "xquakshell/internal/infra/plugin/assets"
	"xquakshell/internal/pkg/safego"
	"xquakshell/internal/usecase"
)

// Serving a plugin's own UI files, and the embed broker beside them.
//
// Its own file because of the listener below: the reason it exists is a WebView2 limitation, which
// is a fact about one platform rather than about how the plugin runtime is composed, and it does
// not belong in the middle of a constructor.

// buildPluginAssetHandler returns the handler that serves plugin UI assets and embed tunnels, and
// starts the loopback server the latter needs.
func buildPluginAssetHandler(
	registry *usecase.PluginRegistry,
	embedTunnels *usecase.EmbedTunnelService,
) http.Handler {
	pluginAssets := infrapluginassets.NewHandler(infrapluginassets.PluginRegistryUIRootResolver(func(id string) (domainplugin.InstalledPlugin, error) {
		return registry.Get(id)
	}))
	embedBroker := infrapluginembed.NewBrokerHandler(embedTunnels, func(pluginID string) (string, error) {
		p, err := registry.Get(pluginID)
		if err != nil {
			return "", err
		}
		return filepath.Join(p.RootDir, "ui"), nil
	})
	compositeAssets := infrapluginembed.NewCompositeHandler(pluginAssets, embedBroker)

	// The composite handler is also served through the Wails asset server (wails.localhost), but
	// on Windows/WebView2 that host serves HTTP only — it does NOT proxy ws:// upgrades, so the
	// embed tunnel WebSocket can never reach the broker there. Serve the same handler from a real
	// loopback listener and point the embed UI/tunnel URLs at it (SetBaseURL), so the iframe loads
	// its assets AND opens its ws:// tunnel against one same-origin host that actually accepts the
	// upgrade. Best-effort: if the listener cannot bind, embed sessions degrade rather than crash.
	if ln, lnErr := net.Listen("tcp", "127.0.0.1:0"); lnErr != nil {
		slog.Warn("embed: loopback broker listener unavailable; embed tunnels disabled", "err", lnErr)
	} else {
		// No WriteTimeout/ReadTimeout: this server also serves WebSocket upgrades for embed
		// tunnels (broker_handler.go), and either deadline would sever those long-lived
		// connections. ReadHeaderTimeout and IdleTimeout are safe for upgraded connections and
		// still bound unauthenticated connections that never send a request.
		srv := &http.Server{
			Handler:           compositeAssets,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		safego.GoNamed("embed.loopbackBroker", func() { _ = srv.Serve(ln) })
		embedTunnels.SetBaseURL("http://" + ln.Addr().String())
		slog.Info("embed: loopback broker listening", "addr", ln.Addr().String())
	}

	return compositeAssets
}
