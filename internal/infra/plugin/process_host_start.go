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

	spawned, err := spawnPluginProcess(h.cfg.DataRoot, plugin, sessionID)
	if err != nil {
		return err
	}

	job, dataDir, err := preparePluginSandbox(h.cfg.DataRoot, plugin, sessionID, spawned.cmd.Process.Pid)
	if err != nil {
		// The process is not on mp yet, so the deferred teardown below cannot see it. Kill it here.
		discardSpawnedProcess(spawned, pluginJob{})
		return err
	}

	// Handing the process to mp and asking whether the reservation is still ours must be ONE atomic
	// step, and Start must be prepared to lose it.
	//
	// Stop takes mp under this lock, sets ProcessStopping, and only then reads mp.cmd/mp.reaper —
	// which, for everything above this point, are still nil. So a Stop landing in the spawn window
	// kills nothing, finalizeProcess burns cleanupOnce, and mp leaves the registry, while this Start
	// walks on and reports success. The child used to die anyway, killed by the caller's context on
	// its way out; once that stopped owning the process, the same window left a live process the host
	// had already forgotten — with a job handle (which on Windows is what kills the process on close)
	// that nothing would ever close, and a waitProcess goroutine parked on a reaper nobody would
	// reach. No amount of locking closes this: in that window Stop HONESTLY has nothing to kill. The
	// reservation has to be re-checked by the side that owns the process.
	h.mu.Lock()
	current, stillRegistered := h.processes[key]
	abandoned := !stillRegistered || current != mp || mp.state == domainplugin.ProcessStopping
	if !abandoned {
		mp.cmd = spawned.cmd
		mp.cancel = spawned.cancel
		mp.reaper = spawned.reaper
		mp.stderr = spawned.stderr
		mp.job = job
	}
	h.mu.Unlock()

	if abandoned {
		discardSpawnedProcess(spawned, job)
		return errStartAbortedByStop
	}

	conn, netProxy, tunnelDial, tunnelLocal, channelProxy, err := h.newConn(plugin, dataDir, sessionID, spawned.stdout, spawned.stdin, negotiated)
	if err != nil {
		return err
	}

	// Second checkpoint, same shape and same reason as the first. A Stop landing between them finds
	// the process (published above) and kills it, but its closeResources runs against an mp whose
	// connection and capability proxies this goroutine is still assigning — a write/read data race,
	// and a leak of whatever it assigned afterwards, since cleanupOnce has already been spent.
	//
	// Publishing under the same lock that Stop takes to declare ProcessStopping makes the two
	// mutually exclusive: either Stop got there first and nothing is published, or the publication
	// happened-before Stop and closeResources sees all five.
	//
	// The bus registration belongs in here with them, and for the same reason: the bus says which
	// processes exist, so a registration written while the reservation belongs to somebody else is
	// the same lie as a stale mp.channels. ChannelBus.mu is a leaf — nothing holds it while taking
	// another lock — so taking it under h.mu introduces no ordering constraint on anyone.
	h.mu.Lock()
	current, stillRegistered = h.processes[key]
	abandoned = !stillRegistered || current != mp || mp.state == domainplugin.ProcessStopping
	if !abandoned {
		mp.conn = conn
		mp.netProxy = netProxy
		mp.tunnelDial = tunnelDial
		mp.tunnelLocal = tunnelLocal
		mp.channels = channelProxy
		if h.cfg.ChannelBus != nil {
			h.cfg.ChannelBus.Register(key, channelProxy)
		}
	}
	h.mu.Unlock()

	if abandoned {
		// The process itself was already killed by the Stop that took the reservation — it had the
		// reaper and the job by then. These five never reached mp, so nothing else can close them.
		discardConnResources(conn, netProxy, tunnelDial, tunnelLocal, channelProxy)
		return errStartAbortedByStop
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
