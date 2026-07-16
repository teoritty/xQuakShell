package plugin

import (
	"context"
	"encoding/json"
	"io"

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
	channelProxy := capability.NewChannelProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Channel, h.cfg.ChannelResolver, h.cfg.ChannelAudit)
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

	return conn, netProxy, tunnelDial, tunnelLocal, channelProxy, nil
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
