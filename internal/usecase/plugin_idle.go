package usecase

import (
	"context"
	"log/slog"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

const defaultPluginIdleTimeout = 5 * time.Minute

// TouchActivity records plugin activity for idle suspend.
func (m *PluginManager) TouchActivity(pluginID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.touchActivityLocked(pluginID)
}

func (m *PluginManager) touchActivityLocked(pluginID string) {
	if m.lastActivity == nil {
		m.lastActivity = make(map[string]time.Time)
	}
	m.lastActivity[pluginID] = time.Now()
}

func (m *PluginManager) effectiveIdleTimeout() time.Duration {
	if m.idleTimeout > 0 {
		return m.idleTimeout
	}
	return defaultPluginIdleTimeout
}

// SuspendIdlePlugins hard-stops plugins with no sessions past the idle threshold.
func (m *PluginManager) SuspendIdlePlugins(ctx context.Context, idleAfter time.Duration) {
	if m == nil {
		return
	}
	now := time.Now()
	var idle []domainplugin.ProcessInstance

	m.mu.Lock()
	instances := m.host.RunningInstances()
	for _, inst := range instances {
		if inst.State != domainplugin.ProcessRunning {
			continue
		}
		if m.sessionCounts[inst.PluginID] > 0 {
			continue
		}
		if m.hasActiveViewPanelsLocked(inst.PluginID) {
			continue
		}
		last, ok := m.lastActivity[inst.PluginID]
		if !ok {
			last = now
			m.lastActivity[inst.PluginID] = now
		}
		if now.Sub(last) >= idleAfter {
			idle = append(idle, inst)
		}
	}
	m.mu.Unlock()

	// The retention check runs outside the lock, because it reaches into another use case and this
	// sweep must not hold the manager's mutex across that (ADR-009). Idleness alone is not grounds
	// for reclaiming a plugin: a discovery plugin holding bindings has no sessions and no view
	// panels, and receives no traffic at all once the user has finished expanding the tree — so
	// "quiet for five minutes" describes it perfectly while it is doing exactly what it should.
	var toStop []domainplugin.ProcessInstance
	for _, inst := range idle {
		if m.PluginInUse(inst.PluginID) {
			continue
		}
		toStop = append(toStop, inst)
	}

	for _, inst := range toStop {
		if err := m.hardSuspend(ctx, inst.PluginID, inst.SessionID); err != nil {
			slog.Warn("idle suspend failed", "pluginId", inst.PluginID, "sessionId", inst.SessionID, "err", err)
		}
	}
}

// hardSuspend deliberately does NOT emit "suspended" when the stop failed, which is the opposite of
// StopPlugin's choice to emit "stopped" regardless. The asymmetry is intended, and it turns on who
// asked and what the failure means:
//
//   - StopPlugin runs because the USER disabled or uninstalled the plugin. That intent stands
//     whatever the OS did, presentation discards the error anyway, and a subtree left standing under
//     a plugin the UI shows as disabled is a contradiction the user cannot resolve.
//   - hardSuspend runs because a timer noticed idleness. Nobody asked for anything, and a failed
//     stop means the process is most likely still alive and still answering — announcing it as
//     suspended would mark its discovery branches stale while they are in fact current, degrading
//     a healthy plugin over a housekeeping hiccup. The next sweep tries again in a minute.
//
// In short: a failed deliberate stop still means "gone as far as anyone can tell"; a failed idle
// suspend means "still here". The state emitted follows that, not the call that failed.
func (m *PluginManager) hardSuspend(ctx context.Context, pluginID, sessionID string) error {
	if m.events != nil {
		m.events.ClearPlugin(pluginID)
	}
	if err := m.host.Stop(ctx, pluginID, sessionID); err != nil {
		return err
	}
	m.emitStateChange(pluginID, "suspended", sessionID)
	slog.Info("plugin idle suspended", "pluginId", pluginID, "sessionId", sessionID)
	return nil
}
