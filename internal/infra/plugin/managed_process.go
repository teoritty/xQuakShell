package plugin

import (
	"context"
	"io"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/infra/plugin/ipc"
)

type managedProcess struct {
	key         string
	plugin      domainplugin.InstalledPlugin
	sessionID   string
	child       childProcess
	cancel      context.CancelFunc
	reaper      *processReaper
	stderr      io.WriteCloser
	conn        *ipc.Conn
	netProxy    *capability.NetProxy
	tunnelDial  *capability.TunnelDialProxy
	tunnelLocal *capability.TunnelLocalProxy
	channels    *capability.ChannelProxy
	negotiated  domainplugin.NegotiatedDescriptor
	state       domainplugin.ProcessState
	job         pluginJob
	// sandbox is the boundary THIS process actually came up behind. It lives here rather than on
	// the host because the answer is now per process: the same build can confine one plugin and
	// fail to confine another, and a user who allowed the fallback gets a mix.
	sandbox     domainplugin.SandboxMode
	cleanupOnce sync.Once
}

// adopt takes ownership of everything the spawn produced, in one step.
//
// It is one call rather than six assignments because its only caller makes them under the lock that
// decides whether this Start still owns the reservation — a block that has to stay short enough to
// read as the atomic step it is, and that the size ratchet on Start keeps honest.
func (mp *managedProcess) adopt(spawned *spawnedProcess, job pluginJob) {
	mp.sandbox = spawned.sandbox
	mp.child = spawned.child
	mp.cancel = spawned.cancel
	mp.reaper = spawned.reaper
	mp.stderr = spawned.stderr
	mp.job = job
}

func (mp *managedProcess) closeResources(killProcess bool) {
	mp.cleanupOnce.Do(func() {
		if mp.stderr != nil {
			_ = mp.stderr.Close()
		}
		if mp.netProxy != nil {
			mp.netProxy.CloseAll()
		}
		if mp.tunnelDial != nil {
			mp.tunnelDial.CloseAll()
		}
		if mp.tunnelLocal != nil {
			mp.tunnelLocal.CloseAll()
		}
		if mp.channels != nil {
			// Process exit/crash unconditionally tears down every channel this process owned,
			// independently of any session-level handling (ADR-011) — a plugin crash
			// must never leave a remote docker exec / relay conn running with no owner.
			mp.channels.CloseAll()
		}
		if mp.conn != nil {
			mp.conn.Close()
		}
		if killProcess && mp.child != nil && mp.reaper != nil {
			_ = mp.reaper.Kill()
			untrackPluginPID(mp.child.Pid())
		}
		// The process context is cancelled unconditionally, after the kill rather than instead of it:
		// killing is the reaper's job (it also waits), and this only releases the context and the
		// watchdog goroutine exec.CommandContext attached to it. On the !killProcess path the child
		// has already exited, so there is nothing left for the cancellation to reach.
		if mp.cancel != nil {
			mp.cancel()
		}
		closePluginJob(mp.job)
		// Last, because the process must be gone first: on Windows this deletes the AppContainer
		// profile the instance ran in, which is durable state in the user's registry and would
		// otherwise accumulate one entry per session for the life of the installation.
		releaseInstanceContainer(mp.plugin, mp.sessionID)
	})
}

// discardConnResources closes an IPC connection and its capability proxies that were built for a
// managedProcess but never handed to it. It mirrors the order closeResources uses — proxies first,
// connection last — so a channel teardown still has a live connection to notify over.
//
// It exists for the same reason discardSpawnedProcess does: what never reached mp cannot be reached
// by mp's own cleanup, and on this path that cleanup has already run and spent its sync.Once.
func discardConnResources(
	conn *ipc.Conn,
	netProxy *capability.NetProxy,
	tunnelDial *capability.TunnelDialProxy,
	tunnelLocal *capability.TunnelLocalProxy,
	channels *capability.ChannelProxy,
) {
	if netProxy != nil {
		netProxy.CloseAll()
	}
	if tunnelDial != nil {
		tunnelDial.CloseAll()
	}
	if tunnelLocal != nil {
		tunnelLocal.CloseAll()
	}
	if channels != nil {
		channels.CloseAll()
	}
	if conn != nil {
		conn.Close()
	}
}
