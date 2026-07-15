package plugin

import (
	"io"
	"os/exec"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

type managedProcess struct {
	key         string
	plugin      domainplugin.InstalledPlugin
	sessionID   string
	cmd         *exec.Cmd
	reaper      *processReaper
	stderr      io.WriteCloser
	conn        *ipc.Conn
	netProxy    *capability.NetProxy
	tunnelDial  *capability.TunnelDialProxy
	tunnelLocal *capability.TunnelLocalProxy
	channels    *capability.ChannelProxy
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
		closePluginJob(mp.job)
	})
}
