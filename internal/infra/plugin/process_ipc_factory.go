package plugin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func (h *ProcessHost) newConn(plugin domainplugin.InstalledPlugin, dataDir, sessionID string, stdout io.Reader, stdin io.Writer, negotiated domainplugin.NegotiatedDescriptor) (*ipc.Conn, *capability.NetProxy, *capability.TunnelDialProxy, *capability.TunnelLocalProxy, *capability.ChannelProxy, error) {
	fs, err := capability.NewFSProxy(plugin.Manifest.Capabilities.FS, dataDir)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	netProxy := capability.NewNetProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Network)
	tunnelDial := capability.NewTunnelDialProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Tunnel, h.cfg.Tunnel)
	tunnelLocal := capability.NewTunnelLocalProxy(plugin.Manifest.ID, h.cfg.Tunnel, tunnelDial)
	// One ChannelProxy per plugin process (ADR-011 Stage 4b): its lifetime is tied to this
	// managedProcess, not to any session, so a plugin crash tears down exactly its own channels.
	// The resolver is built here and not in the composition root because this is where the
	// process's manifest and sessionID are in scope — everything a purpose backend needs beyond
	// the purpose string itself. A nil factory leaves the proxy resolverless, which it already
	// reports as a denied channel.open rather than a panic.
	var resolveChannel capability.ChannelBackendResolver
	if h.cfg.ChannelResolverFor != nil {
		resolveChannel = h.cfg.ChannelResolverFor(plugin, sessionID)
	}
	channelProxy := capability.NewChannelProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Channel, resolveChannel, h.cfg.ChannelAudit)
	var sessions domainplugin.SessionRPCHandler
	if h.cfg.SessionRPC != nil {
		sessions = h.cfg.SessionRPC(plugin, sessionID, channelInboundAdapter{proxy: channelProxy})
	}
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID:    plugin.Manifest.ID,
		Gate:        capability.NewGate(plugin.Manifest, negotiated),
		FS:          fs,
		Net:         netProxy,
		Vault:       capability.NewVaultProxy(h.cfg.Vault),
		Sessions:    sessions,
		Events:      capability.NewEventsProxy(h.cfg.Events),
		Views:       capability.NewViewProxy(h.cfg.Views),
		TunnelDial:  tunnelDial,
		TunnelLocal: tunnelLocal,
		Audit:       h.cfg.Audit,
		OnActivity:  h.cfg.OnPluginActivity,
	})
	conn := ipc.NewConn(stdout, stdin, nil, server.RequestHandler(), channelThroughputKbps(plugin.Manifest.Capabilities.Channel))

	// The proxy needs the conn to open data paths, and the conn needs the request handler that
	// routes channel.open back into the proxy — a cycle no constructor ordering resolves. It is
	// broken here, in the composition root, and only here: this is the one place that knows both
	// halves. Attaching after NewConn is safe because no channel can exist before the plugin's
	// first channel.open, which cannot arrive before the handler above is serving.
	channelProxy.AttachDataPathOpener(conn)

	// The close notifier sits behind the same cycle and is broken the same way: a backend built by
	// the resolver above cannot be handed this process's Conn.Notify, because the Conn does not
	// exist yet. So the composition root is given it here instead, once, for this process — the
	// granularity the notifier needs, since the channel id travels in the call (D8) rather than in
	// a closure no resolver could build.
	if h.cfg.AttachChannelCloseNotifier != nil {
		h.cfg.AttachChannelCloseNotifier(plugin, sessionID, func(channelID uint32, reason, message string) {
			notifyChannelClose(conn, plugin.Manifest.ID, channelID, reason, message)
		})
	}

	return conn, netProxy, tunnelDial, tunnelLocal, channelProxy, nil
}

// notifyChannelClose emits channel.close {channelId, reason, message} to one plugin process.
//
// A failed write is logged and discarded rather than surfaced: the notifier's callers are backend
// teardown paths that have nothing left to do about it, and the only way this write fails is a
// connection already gone — which the read loop and crash teardown already handle. Raising it
// here would give the caller a second, redundant report of the same death.
func notifyChannelClose(conn *ipc.Conn, pluginID string, channelID uint32, reason, message string) {
	params, err := json.Marshal(map[string]any{
		"channelId": channelID,
		"reason":    reason,
		"message":   message,
	})
	if err != nil {
		slog.Warn("encode channel.close notification", "plugin", pluginID, "channel", channelID, "err", err)
		return
	}
	if err := conn.Notify("channel.close", params); err != nil {
		slog.Warn("notify channel.close", "plugin", pluginID, "channel", channelID, "err", err)
	}
}

// channelThroughputKbps resolves the manifest's declared per-channel bandwidth cap, falling back
// to the host default when the manifest is silent.
func channelThroughputKbps(caps *domainplugin.ChannelCaps) int {
	if caps == nil || caps.MaxThroughputKbps <= 0 {
		return domainplugin.DefaultChannelThroughputKbps
	}
	return caps.MaxThroughputKbps
}

// channelInboundAdapter adapts a single process's ChannelProxy (params-only Open/Close) to
// domainplugin.ChannelInboundPort (which additionally carries pluginID, unused here since the
// proxy is already scoped to exactly one plugin process).
type channelInboundAdapter struct {
	proxy *capability.ChannelProxy
}

func (a channelInboundAdapter) Open(ctx context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	return a.proxy.Open(ctx, params)
}

func (a channelInboundAdapter) Close(ctx context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	return a.proxy.Close(ctx, params)
}
