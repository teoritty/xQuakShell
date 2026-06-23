package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/domain"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

const (
	coreAPIVersion      = "1.0.0"
	initTimeout         = 10 * time.Second
	callTimeout         = 5 * time.Second
	shutdownCallTimeout = 2 * time.Second
	stopGracePeriod     = 3 * time.Second
)

// ProcessCrashHandler is notified when a plugin process exits abnormally.
type ProcessCrashHandler func(pluginID, sessionID string)

// HostConfig configures the out-of-process plugin host.
type HostConfig struct {
	DataRoot          string
	Portable          domain.PortableRuntime
	Vault             domainplugin.VaultInboundPort
	SessionRPC        domainplugin.SessionRPCHandlerFactory
	Events            domainplugin.EventInboundPort
	Views             domainplugin.ViewInboundPort
	SessionAuthorizer domainplugin.SessionRPCAuthorizer
	Audit             ipc.PluginAuditFunc
	OnCrash           ProcessCrashHandler
	OnPluginActivity  func(pluginID string)
}

// ProcessHost implements domainplugin.ProcessHost using OS child processes.
type ProcessHost struct {
	cfg       HostConfig
	mu        sync.Mutex
	processes map[string]*managedProcess
}

type managedProcess struct {
	key       string
	plugin    domainplugin.InstalledPlugin
	sessionID string
	cmd       *exec.Cmd
	reaper    *processReaper
	stderr    io.WriteCloser
	conn      *ipc.Conn
	netProxy  *capability.NetProxy
	state     domainplugin.ProcessState
	job       pluginJob // platform-specific; closed on finalize
}

// NewProcessHost creates a process host with capability proxies and audit hooks.
func NewProcessHost(cfg HostConfig) *ProcessHost {
	return &ProcessHost{
		cfg:       cfg,
		processes: make(map[string]*managedProcess),
	}
}

// Start launches the plugin binary and sends initialize.
func (h *ProcessHost) Start(ctx context.Context, plugin domainplugin.InstalledPlugin, sessionID string) error {
	key := processKey(plugin, sessionID)

	h.mu.Lock()
	if existing, ok := h.processes[key]; ok && existing.state == domainplugin.ProcessRunning {
		h.mu.Unlock()
		return domainplugin.ErrPluginAlreadyRunning
	}
	h.mu.Unlock()

	entryPath, err := ResolveEngineEntryPath(plugin.RootDir, plugin.Manifest.Engine.Entry)
	if err != nil {
		return fmt.Errorf("resolve plugin entry: %w", err)
	}
	if err := plugin.Manifest.CompatibleWithCore(domainplugin.HostCoreVersion); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, entryPath)
	cmd.Env = PluginProcessEnv(h.cfg.DataRoot, plugin.Manifest.ID, sessionID)
	stderrLog := NewRedactingStderrWriter(plugin.Manifest.ID)
	cmd.Stderr = stderrLog
	if err := configurePluginCmd(cmd); err != nil {
		_ = stderrLog.Close()
		return fmt.Errorf("configure plugin process: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stderrLog.Close()
		return fmt.Errorf("plugin stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stderrLog.Close()
		return fmt.Errorf("plugin stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stderrLog.Close()
		return fmt.Errorf("start plugin %s: %w", plugin.Manifest.ID, err)
	}

	reaper := newProcessReaper(cmd)
	reaper.Start()

	abortStart := func() {
		_ = reaper.Kill()
		_ = stderrLog.Close()
	}

	job, err := createPluginJob()
	if err != nil {
		abortStart()
		return fmt.Errorf("create plugin job: %w", err)
	}
	if err := assignProcessToJob(job, cmd.Process.Pid); err != nil {
		closePluginJob(job)
		abortStart()
		return fmt.Errorf("assign plugin %s to job: %w", plugin.Manifest.ID, err)
	}
	if err := applyPluginResourceLimits(cmd.Process.Pid, job); err != nil {
		closePluginJob(job)
		abortStart()
		return fmt.Errorf("apply plugin %s resource limits: %w", plugin.Manifest.ID, err)
	}

	isolation := plugin.Manifest.EffectiveIsolation()
	dataDir, err := EnsurePluginInstanceDataDir(h.cfg.DataRoot, plugin.Manifest.ID, sessionID, isolation)
	if err != nil {
		closePluginJob(job)
		abortStart()
		return fmt.Errorf("create plugin data dir: %w", err)
	}

	conn, netProxy, err := h.newConn(plugin, dataDir, sessionID, stdout, stdin)
	if err != nil {
		closePluginJob(job)
		abortStart()
		return err
	}

	mp := &managedProcess{
		key:       key,
		plugin:    plugin,
		sessionID: sessionID,
		cmd:       cmd,
		reaper:    reaper,
		stderr:    stderrLog,
		conn:      conn,
		netProxy:  netProxy,
		state:     domainplugin.ProcessStarting,
		job:       job,
	}

	h.mu.Lock()
	h.processes[key] = mp
	h.mu.Unlock()

	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	if h.cfg.Portable != nil && h.cfg.Portable.DataRootReadOnly() {
		slog.Warn("portable data root is read-only", "pluginId", plugin.Manifest.ID)
	}

	initParams := domainplugin.InitializeParams{
		PluginID:     plugin.Manifest.ID,
		APIVersion:   coreAPIVersion,
		Capabilities: plugin.Manifest.Capabilities,
		DataDir:      dataDir,
		CoreVersion:  domainplugin.HostCoreVersion,
	}
	params, err := ipc.EncodeParams(initParams)
	if err != nil {
		h.finalizeProcess(key, mp)
		return err
	}

	if _, err := conn.Call(initCtx, "initialize", params); err != nil {
		h.finalizeProcess(key, mp)
		return fmt.Errorf("plugin initialize: %w", err)
	}

	h.mu.Lock()
	mp.state = domainplugin.ProcessRunning
	h.mu.Unlock()

	go h.waitProcess(key, mp)
	return nil
}

func (h *ProcessHost) newConn(plugin domainplugin.InstalledPlugin, dataDir, sessionID string, stdout io.Reader, stdin io.Writer) (*ipc.Conn, *capability.NetProxy, error) {
	fs, err := capability.NewFSProxy(plugin.Manifest.Capabilities.FS, dataDir)
	if err != nil {
		return nil, nil, err
	}
	netProxy := capability.NewNetProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Network)
	var sessions domainplugin.SessionRPCHandler
	if h.cfg.SessionRPC != nil {
		sessions = h.cfg.SessionRPC(plugin, sessionID)
	}
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: plugin.Manifest.ID,
		Gate:     capability.NewGate(plugin.Manifest),
		FS:       fs,
		Net:      netProxy,
		Vault:    capability.NewVaultProxy(h.cfg.Vault),
		Sessions: sessions,
		Events:   capability.NewEventsProxy(h.cfg.Events),
		Views:    capability.NewViewProxy(h.cfg.Views),
		Audit:    h.cfg.Audit,
		OnActivity: h.cfg.OnPluginActivity,
	})
	conn := ipc.NewConn(stdout, stdin, nil, server.RequestHandler())
	return conn, netProxy, nil
}

// Stop gracefully shuts down a plugin process.
func (h *ProcessHost) Stop(ctx context.Context, pluginID, sessionID string) error {
	key := h.resolveKey(pluginID, sessionID)
	h.mu.Lock()
	mp, ok := h.processes[key]
	if !ok {
		h.mu.Unlock()
		return nil
	}
	mp.state = domainplugin.ProcessStopping
	h.mu.Unlock()

	if mp.conn != nil {
		_ = mp.conn.Notify("deactivate", nil)

		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownCallTimeout)
		_, _ = mp.conn.Call(shutdownCtx, "shutdown", nil)
		shutdownCancel()

		mp.conn.CloseWrite()
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, stopGracePeriod)
	waitErr := mp.reaper.Wait(waitCtx)
	waitCancel()
	if waitErr != nil {
		_ = mp.reaper.Kill()
	}

	h.finalizeProcess(key, mp)
	return nil
}

// Call invokes a JSON-RPC method on a running plugin.
func (h *ProcessHost) Call(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	mp, err := h.runningProcess(pluginID, sessionID)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return mp.conn.Call(callCtx, method, params)
}

// Notify sends a JSON-RPC notification to a running plugin.
func (h *ProcessHost) Notify(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
	mp, err := h.runningProcess(pluginID, sessionID)
	if err != nil {
		return err
	}
	_ = ctx
	return mp.conn.Notify(method, params)
}

// State returns the current process state for a plugin instance.
func (h *ProcessHost) State(pluginID, sessionID string) domainplugin.ProcessState {
	h.mu.Lock()
	defer h.mu.Unlock()
	mp, ok := h.processes[h.resolveKey(pluginID, sessionID)]
	if !ok {
		return domainplugin.ProcessDiscovered
	}
	return mp.state
}

// RunningInstances returns a snapshot of tracked plugin processes and their states.
func (h *ProcessHost) RunningInstances() []domainplugin.ProcessInstance {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]domainplugin.ProcessInstance, 0, len(h.processes))
	for _, mp := range h.processes {
		out = append(out, domainplugin.ProcessInstance{
			PluginID:  mp.plugin.Manifest.ID,
			SessionID: mp.sessionID,
			State:     mp.state,
		})
	}
	return out
}

// StopAll stops every running plugin (app shutdown).
func (h *ProcessHost) StopAll(ctx context.Context) {
	h.mu.Lock()
	targets := make([]struct {
		pluginID  string
		sessionID string
	}, 0, len(h.processes))
	for _, mp := range h.processes {
		targets = append(targets, struct {
			pluginID  string
			sessionID string
		}{mp.plugin.Manifest.ID, mp.sessionID})
	}
	h.mu.Unlock()

	for _, target := range targets {
		if err := h.Stop(ctx, target.pluginID, target.sessionID); err != nil {
			slog.Warn("stop plugin failed", "pluginId", target.pluginID, "err", err)
		}
	}
	KillAllTrackedPlugins()
}

func (h *ProcessHost) resolveKey(pluginID, sessionID string) string {
	if sessionID != "" {
		return pluginID + "\x00" + sessionID
	}
	return pluginID
}

func (h *ProcessHost) runningProcess(pluginID, sessionID string) (*managedProcess, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	mp, ok := h.processes[h.resolveKey(pluginID, sessionID)]
	if !ok || mp.state != domainplugin.ProcessRunning {
		return nil, domainplugin.ErrPluginNotRunning
	}
	return mp, nil
}

func (h *ProcessHost) waitProcess(key string, mp *managedProcess) {
	<-mp.reaper.Done()
	exitErr := mp.reaper.ExitErr()

	if mp.stderr != nil {
		_ = mp.stderr.Close()
	}
	if mp.conn != nil {
		mp.conn.Close()
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	current, ok := h.processes[key]
	if !ok || current != mp {
		closePluginJob(mp.job)
		return
	}

	crashed := exitErr != nil
	if crashed {
		mp.state = domainplugin.ProcessCrashed
		slog.Warn("plugin process exited", "pluginId", mp.plugin.Manifest.ID, "sessionId", mp.sessionID, "err", exitErr)
	} else if mp.state != domainplugin.ProcessStopping {
		mp.state = domainplugin.ProcessStopped
	}
	delete(h.processes, key)
	closePluginJob(mp.job)
	if mp.cmd != nil && mp.cmd.Process != nil {
		untrackPluginPID(mp.cmd.Process.Pid)
	}

	if crashed && h.cfg.OnCrash != nil {
		h.cfg.OnCrash(mp.plugin.Manifest.ID, mp.sessionID)
	}
}

// finalizeProcess releases IPC and job resources without calling Wait (reaper owns Wait).
func (h *ProcessHost) finalizeProcess(key string, mp *managedProcess) {
	if mp.stderr != nil {
		_ = mp.stderr.Close()
	}
	if mp.netProxy != nil {
		mp.netProxy.CloseAll()
	}
	if mp.conn != nil {
		mp.conn.Close()
	}
	if mp.cmd.Process != nil && mp.reaper != nil {
		_ = mp.reaper.Kill()
		untrackPluginPID(mp.cmd.Process.Pid)
	}
	closePluginJob(mp.job)

	h.mu.Lock()
	if current, ok := h.processes[key]; ok && current == mp {
		delete(h.processes, key)
	}
	h.mu.Unlock()
}

var _ domainplugin.ProcessHost = (*ProcessHost)(nil)
