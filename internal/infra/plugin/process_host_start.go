package plugin

import (
	"context"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/safego"
)

// Start launches the plugin binary and sends initialize.
func (h *ProcessHost) Start(ctx context.Context, plugin domainplugin.InstalledPlugin, sessionID string) error {
	key := processKey(plugin, sessionID)

	h.mu.Lock()
	if existing, ok := h.processes[key]; ok {
		switch existing.state {
		case domainplugin.ProcessRunning, domainplugin.ProcessStarting:
			h.mu.Unlock()
			return domainplugin.ErrPluginAlreadyRunning
		}
	}
	mp := &managedProcess{
		key:       key,
		plugin:    plugin,
		sessionID: sessionID,
		state:     domainplugin.ProcessStarting,
	}
	h.processes[key] = mp
	h.mu.Unlock()

	running := false
	defer func() {
		if !running {
			h.releaseStartReservation(key, mp)
		}
	}()

	// Resolve the plugin against the LIVE registry into the single runtime contract, BEFORE
	// spawning a process. This is the authoritative handshake check (ADR-012): an incompatible
	// plugin — including live-registry skew since install — fails here and never spawns. The same
	// descriptor is threaded to the gate (feature-version enforcement) and the initializer, so
	// there is exactly one negotiation for this process.
	negotiated, negotiationWarnings, err := domainplugin.Negotiate(&plugin.Manifest, domainplugin.HostRegistry())
	if err != nil {
		return err
	}
	mp.negotiated = negotiated

	spawned, err := spawnPluginProcess(ctx, h.cfg.DataRoot, plugin, sessionID)
	if err != nil {
		return err
	}
	mp.cmd = spawned.cmd
	mp.reaper = spawned.reaper
	mp.stderr = spawned.stderr

	job, dataDir, err := preparePluginSandbox(h.cfg.DataRoot, plugin, sessionID, spawned.cmd.Process.Pid)
	if err != nil {
		return err
	}
	mp.job = job

	conn, netProxy, tunnelDial, tunnelLocal, channelProxy, err := h.newConn(plugin, dataDir, sessionID, spawned.stdout, spawned.stdin, negotiated)
	if err != nil {
		return err
	}
	mp.conn = conn
	mp.netProxy = netProxy
	mp.tunnelDial = tunnelDial
	mp.tunnelLocal = tunnelLocal
	mp.channels = channelProxy
	if h.cfg.ChannelBus != nil {
		h.cfg.ChannelBus.Register(key, channelProxy)
	}

	portableReadOnly := h.cfg.Portable != nil && h.cfg.Portable.DataRootReadOnly()
	if err := initializePluginProcess(ctx, conn, plugin, dataDir, portableReadOnly, negotiated, negotiationWarnings); err != nil {
		return err
	}

	h.mu.Lock()
	mp.state = domainplugin.ProcessRunning
	h.mu.Unlock()

	running = true
	safego.GoNamed("plugin.waitProcess", func() { h.waitProcess(key, mp) })
	return nil
}
