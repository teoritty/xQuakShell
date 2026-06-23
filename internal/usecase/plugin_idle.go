package usecase

import (
	"context"
	"log/slog"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
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
	var toStop []domainplugin.ProcessInstance

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
			toStop = append(toStop, inst)
		}
	}
	m.mu.Unlock()

	for _, inst := range toStop {
		if err := m.hardSuspend(ctx, inst.PluginID, inst.SessionID); err != nil {
			slog.Warn("idle suspend failed", "pluginId", inst.PluginID, "sessionId", inst.SessionID, "err", err)
		}
	}
}

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
