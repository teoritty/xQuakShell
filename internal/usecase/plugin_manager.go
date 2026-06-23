package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/domain"
)

// PluginCrashHandler reacts to abnormal plugin process exits.
type PluginCrashHandler interface {
	HandlePluginProcessCrashed(pluginID, sessionID string)
}

// PluginManager orchestrates plugin discovery and process lifecycle (ADR-003).
type PluginManager struct {
	registry       *PluginRegistry
	host           domainplugin.ProcessHost
	loadBundle     BundleLoader
	installBundle  BundleInstaller
	installRoot    string
	bundle         domainplugin.BundlePort
	portable       domain.PortableRuntime
	events         *PluginEventBus
	crashHandler   PluginCrashHandler
	settingsReader PluginSettingsReader
	pluginSettings *PluginVaultSettings
	startAudit     PluginStartAuditFunc
	stateChange    func(pluginID, state, sessionID string)

	mu              sync.Mutex
	sessionCounts   map[string]int
	viewPanelCounts map[string]int
	lastActivity    map[string]time.Time
	idleTimeout     time.Duration
}

// NewPluginManager creates a plugin manager with the given registry and process host port.
func NewPluginManager(registry *PluginRegistry, host domainplugin.ProcessHost) *PluginManager {
	return NewPluginManagerWithConfig(PluginManagerConfig{
		Registry: registry,
		Host:     host,
	})
}

// DiscoverPlugins loads manifests via the provided discover function.
func (m *PluginManager) DiscoverPlugins(discover func() ([]domainplugin.InstalledPlugin, error)) error {
	plugins, err := discover()
	if err != nil {
		return fmt.Errorf("discover plugins: %w", err)
	}
	if err := m.registry.Load(plugins); err != nil {
		return fmt.Errorf("discover plugins: %w", err)
	}
	slog.Info("plugins discovered", "count", len(plugins))
	return nil
}

// List returns installed plugins with runtime process state.
func (m *PluginManager) List() []PluginInfo {
	installed := m.registry.List()
	result := make([]PluginInfo, 0, len(installed))
	for _, p := range installed {
		result = append(result, PluginInfo{
			ID:                   p.Manifest.ID,
			Name:                 p.Manifest.Name,
			Version:              p.Manifest.Version,
			Description:          p.Manifest.Description,
			Source:               string(p.Source),
			State:                string(m.aggregateProcessState(p.Manifest.ID)),
			RequiresSecretAccess: p.Manifest.RequiresSecretAccess(),
			Signed:               p.Manifest.Signature != "",
			Enabled:              m.isPluginEnabled(p.Manifest.ID),
		})
	}
	return result
}

// PluginInfo is a read model for presentation layer mapping.
type PluginInfo struct {
	ID                   string
	Name                 string
	Version              string
	Description          string
	Source               string
	State                string
	RequiresSecretAccess bool
	Signed               bool
	Enabled              bool
}

// EnsureRunning starts the plugin process if not already running (per-plugin isolation only).
func (m *PluginManager) EnsureRunning(ctx context.Context, pluginID string) error {
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return err
	}
	if plugin.Manifest.RequiresSessionScopedProcess() {
		return domainplugin.ErrSessionScopeRequired
	}
	return m.EnsureRunningForSession(ctx, pluginID, "")
}

// SetCrashHandler binds session error handling for plugin crashes.
func (m *PluginManager) SetCrashHandler(h PluginCrashHandler) {
	m.crashHandler = h
}

// Ping verifies a plugin responds over IPC when already running.
func (m *PluginManager) Ping(ctx context.Context, pluginID string) (map[string]string, error) {
	scope, err := m.resolvePingScope(pluginID)
	if err != nil {
		return nil, err
	}
	if m.host.State(pluginID, scope) != domainplugin.ProcessRunning {
		return nil, domainplugin.ErrPluginNotRunning
	}
	raw, err := m.host.Call(ctx, pluginID, scope, "ping", nil)
	if err != nil {
		return nil, fmt.Errorf("plugin ping: %w", err)
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode ping result: %w", err)
	}
	return result, nil
}

// Call sends a JSON-RPC request to a running plugin process (per-plugin scope or single session instance).
func (m *PluginManager) Call(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	scope, err := m.resolveCommandScope(pluginID)
	if err != nil {
		return nil, err
	}
	return m.host.Call(ctx, pluginID, scope, method, params)
}

// Notify sends a JSON-RPC notification to a running plugin process (per-plugin scope or single session instance).
func (m *PluginManager) Notify(ctx context.Context, pluginID, method string, params json.RawMessage) error {
	scope, err := m.resolveCommandScope(pluginID)
	if err != nil {
		return err
	}
	return m.host.Notify(ctx, pluginID, scope, method, params)
}

func (m *PluginManager) SessionOpened(pluginID string) {
	m.mu.Lock()
	m.sessionCounts[pluginID]++
	m.touchActivityLocked(pluginID)
	m.mu.Unlock()
}

// ActiveSessionCount returns open sessions owned by a plugin.
func (m *PluginManager) ActiveSessionCount(pluginID string) int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionCounts[pluginID]
}

// SessionClosed decrements session count; idle suspender stops the process later.
func (m *PluginManager) SessionClosed(ctx context.Context, pluginID, sessionID string) {
	m.mu.Lock()
	if count := m.sessionCounts[pluginID]; count > 0 {
		m.sessionCounts[pluginID] = count - 1
	}
	m.mu.Unlock()
	m.TouchActivity(pluginID)

	scope := m.sessionScope(pluginID, sessionID)
	if scope != "" {
		_ = m.host.Stop(ctx, pluginID, scope)
	}
}

// StopAll stops all plugin processes during app shutdown.
func (m *PluginManager) StopAll(ctx context.Context) {
	m.host.StopAll(ctx)
}

// Registry returns the underlying registry (read-only use from presentation).
func (m *PluginManager) Registry() *PluginRegistry {
	return m.registry
}

// SetEventBus attaches the hub-and-spoke event bus.
func (m *PluginManager) SetEventBus(bus *PluginEventBus) {
	m.events = bus
}

// SetSessionOwnershipChecker binds session ownership checks for core event delivery.
func (m *PluginManager) SetSessionOwnershipChecker(checker PluginSessionOwnershipChecker) {
	if m != nil && m.events != nil {
		m.events.SetSessionOwnershipChecker(checker)
	}
}

// SetIdleTimeout configures hard-suspend idle threshold (default 5 minutes).
func (m *PluginManager) SetIdleTimeout(d time.Duration) {
	if d > 0 {
		m.idleTimeout = d
	}
}

// SetSettingsReader binds plugin settings for enable/disable and secret grants.
func (m *PluginManager) SetSettingsReader(r PluginSettingsReader) {
	m.settingsReader = r
}

// SetPluginSettings binds mutable plugin settings persistence.
func (m *PluginManager) SetPluginSettings(s *PluginVaultSettings) {
	m.pluginSettings = s
	if s != nil {
		m.settingsReader = s
	}
}

// SetStartAudit binds audit logging for plugin start authorization.
func (m *PluginManager) SetStartAudit(fn PluginStartAuditFunc) {
	m.startAudit = fn
}

// SetStateChangeHandler emits plugin lifecycle state changes to presentation.
func (m *PluginManager) SetStateChangeHandler(fn func(pluginID, state, sessionID string)) {
	m.stateChange = fn
}

func (m *PluginManager) emitStateChange(pluginID, state, sessionID string) {
	if m.stateChange != nil {
		m.stateChange(pluginID, state, sessionID)
	}
}
