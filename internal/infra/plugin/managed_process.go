package plugin

import (
	"context"
	"io"
	"os/exec"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/infra/plugin/ipc"
)

type managedProcess struct {
	key         string
	plugin      domainplugin.InstalledPlugin
	sessionID   string
	cmd         *exec.Cmd
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
	cleanupOnce sync.Once
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
			// independently of any session-level handling (ADR-011 Stage 4b) — a plugin crash
			// must never leave a remote docker exec / relay conn running with no owner.
			mp.channels.CloseAll()
		}
		if mp.conn != nil {
			mp.conn.Close()
		}
		if killProcess && mp.cmd != nil && mp.cmd.Process != nil && mp.reaper != nil {
			_ = mp.reaper.Kill()
			untrackPluginPID(mp.cmd.Process.Pid)
		}
		// The process context is cancelled unconditionally, after the kill rather than instead of it:
		// killing is the reaper's job (it also waits), and this only releases the context and the
		// watchdog goroutine exec.CommandContext attached to it. On the !killProcess path the child
		// has already exited, so there is nothing left for the cancellation to reach.
		if mp.cancel != nil {
			mp.cancel()
		}
		closePluginJob(mp.job)
	})
}
