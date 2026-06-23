package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
)

// hostProcessScope resolves the ProcessHost session key for a plugin IPC target.
func (m *PluginManager) hostProcessScope(pluginID, sessionID string) (string, error) {
	if m == nil || m.registry == nil {
		return "", fmt.Errorf("plugin manager unavailable")
	}
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return "", err
	}
	return plugin.Manifest.HostProcessScope(sessionID)
}

// CallProcess sends a JSON-RPC request using manifest-aware process scope.
func (m *PluginManager) CallProcess(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	if m == nil || m.host == nil {
		return nil, fmt.Errorf("plugin manager unavailable")
	}
	scope, err := m.hostProcessScope(pluginID, sessionID)
	if err != nil {
		return nil, err
	}
	return m.host.Call(ctx, pluginID, scope, method, params)
}

// NotifyProcess sends a JSON-RPC notification using manifest-aware process scope.
func (m *PluginManager) NotifyProcess(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
	if m == nil || m.host == nil {
		return fmt.Errorf("plugin manager unavailable")
	}
	scope, err := m.hostProcessScope(pluginID, sessionID)
	if err != nil {
		return err
	}
	return m.host.Notify(ctx, pluginID, scope, method, params)
}

// aggregateProcessState returns the highest-priority runtime state across all instances of a plugin.
func (m *PluginManager) aggregateProcessState(pluginID string) domainplugin.ProcessState {
	if m == nil || m.host == nil {
		return domainplugin.ProcessDiscovered
	}

	best := domainplugin.ProcessDiscovered
	bestRank := processStateRank(best)
	found := false

	for _, inst := range m.host.RunningInstances() {
		if inst.PluginID != pluginID {
			continue
		}
		found = true
		if rank := processStateRank(inst.State); rank > bestRank {
			best = inst.State
			bestRank = rank
		}
	}
	if found {
		return best
	}
	return m.host.State(pluginID, "")
}

func processStateRank(state domainplugin.ProcessState) int {
	switch state {
	case domainplugin.ProcessRunning:
		return 5
	case domainplugin.ProcessStarting:
		return 4
	case domainplugin.ProcessStopping:
		return 3
	case domainplugin.ProcessCrashed:
		return 2
	case domainplugin.ProcessStopped, domainplugin.ProcessSuspended:
		return 1
	default:
		return 0
	}
}

// resolvePingScope picks the ProcessHost session key for ping on a plugin that may be session-scoped.
func (m *PluginManager) resolvePingScope(pluginID string) (string, error) {
	if m == nil || m.registry == nil {
		return "", fmt.Errorf("plugin manager unavailable")
	}
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return "", err
	}
	if !plugin.Manifest.RequiresSessionScopedProcess() {
		return "", nil
	}
	return m.singleRunningSessionScope(pluginID)
}

// resolveCommandScope picks the ProcessHost session key for command.execute on session-scoped plugins.
func (m *PluginManager) resolveCommandScope(pluginID string) (string, error) {
	if m == nil || m.registry == nil {
		return "", fmt.Errorf("plugin manager unavailable")
	}
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return "", err
	}
	if !plugin.Manifest.RequiresSessionScopedProcess() {
		return "", nil
	}
	return m.singleRunningSessionScope(pluginID)
}

func (m *PluginManager) singleRunningSessionScope(pluginID string) (string, error) {
	if m == nil || m.host == nil {
		return "", domainplugin.ErrPluginNotRunning
	}
	var (
		scope   string
		matches int
	)
	for _, inst := range m.host.RunningInstances() {
		if inst.PluginID != pluginID || inst.State != domainplugin.ProcessRunning {
			continue
		}
		matches++
		scope = inst.SessionID
	}
	switch matches {
	case 0:
		return "", domainplugin.ErrPluginNotRunning
	case 1:
		return scope, nil
	default:
		return "", fmt.Errorf("%w: multiple session-scoped instances running", domainplugin.ErrSessionScopeRequired)
	}
}

// stopAllPluginInstances stops every tracked OS process for pluginID.
func (m *PluginManager) stopAllPluginInstances(ctx context.Context, pluginID string) error {
	if m == nil || m.host == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, inst := range m.host.RunningInstances() {
		if inst.PluginID != pluginID {
			continue
		}
		key := inst.SessionID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := m.host.Stop(ctx, pluginID, inst.SessionID); err != nil {
			return err
		}
	}
	if len(seen) == 0 {
		return m.host.Stop(ctx, pluginID, "")
	}
	return nil
}
