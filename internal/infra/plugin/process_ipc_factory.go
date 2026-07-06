package plugin

import (
	"io"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func (h *ProcessHost) newConn(plugin domainplugin.InstalledPlugin, dataDir, sessionID string, stdout io.Reader, stdin io.Writer) (*ipc.Conn, *capability.NetProxy, error) {
	fs, err := capability.NewFSProxy(plugin.Manifest.Capabilities.FS, dataDir)
	if err != nil {
		return nil, nil, err
	}
	netProxy := capability.NewNetProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Network)
	tunnelDial := capability.NewTunnelDialProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Tunnel, h.cfg.Tunnel)
	tunnelLocal := capability.NewTunnelLocalProxy(plugin.Manifest.ID, h.cfg.Tunnel, tunnelDial.ReleaseSlot)
	var sessions domainplugin.SessionRPCHandler
	if h.cfg.SessionRPC != nil {
		sessions = h.cfg.SessionRPC(plugin, sessionID)
	}
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID:    plugin.Manifest.ID,
		Gate:        capability.NewGate(plugin.Manifest),
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
	conn := ipc.NewConn(stdout, stdin, nil, server.RequestHandler())
	return conn, netProxy, nil
}
