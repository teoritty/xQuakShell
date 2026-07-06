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
	key             string
	plugin          domainplugin.InstalledPlugin
	sessionID       string
	cmd             *exec.Cmd
	reaper          *processReaper
	stderr          io.WriteCloser
	conn            *ipc.Conn
	netProxy        *capability.NetProxy
	releaseDialSlot func()
	state           domainplugin.ProcessState
	job             pluginJob
	cleanupOnce     sync.Once
}

func (mp *managedProcess) closeResources(killProcess bool) {
	mp.cleanupOnce.Do(func() {
		if mp.stderr != nil {
			_ = mp.stderr.Close()
		}
		if mp.netProxy != nil {
			mp.netProxy.CloseAll()
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
