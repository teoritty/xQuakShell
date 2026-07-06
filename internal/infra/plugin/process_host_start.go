package plugin

import (
	"context"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
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

	conn, netProxy, releaseSlot, err := h.newConn(plugin, dataDir, sessionID, spawned.stdout, spawned.stdin)
	if err != nil {
		return err
	}
	mp.conn = conn
	mp.netProxy = netProxy
	mp.releaseDialSlot = releaseSlot

	portableReadOnly := h.cfg.Portable != nil && h.cfg.Portable.DataRootReadOnly()
	if err := initializePluginProcess(ctx, conn, plugin, dataDir, portableReadOnly); err != nil {
		return err
	}

	h.mu.Lock()
	mp.state = domainplugin.ProcessRunning
	h.mu.Unlock()

	running = true
	safego.GoNamed("plugin.waitProcess", func() { h.waitProcess(key, mp) })
	return nil
}
