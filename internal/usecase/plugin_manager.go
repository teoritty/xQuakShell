package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/domain"
)

// PluginCrashHandler reacts to abnormal plugin process exits.
type PluginCrashHandler interface {
	HandlePluginProcessCrashed(pluginID, sessionID string)
}

// OutboundAuthAuditFunc records host→plugin auth.* RPC calls with sanitized params.
type OutboundAuthAuditFunc func(pluginID, method, sanitizedParams string)

// PluginManager orchestrates plugin discovery and process lifecycle (ADR-003).
type PluginManager struct {
	registry       *PluginRegistry
	host           domainplugin.ProcessHost
	loadBundle     BundleLoader
	installBundle  BundleInstaller
	installRoot    string
	portableData   domain.PortableDataStore
	bundle         domainplugin.BundlePort
	portable       domain.PortableRuntime
	events         *PluginEventBus
	crashHandler   PluginCrashHandler
	settingsReader PluginSettingsReader
	pluginSettings *PluginVaultSettings
	startAudit        PluginStartAuditFunc
	outboundAuthAudit OutboundAuthAuditFunc
	stateChange       func(pluginID, state, sessionID string)
	processStarted    func(pluginID string)
	processStopped    func(pluginID string)
	processCrashed    func(pluginID string)
	processSuspended  func(pluginID string)
	connChecker    PluginConnectionChecker

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
			InstalledReleaseTag:  installedReleaseTag(p),
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
	InstalledReleaseTag  string
}

func installedReleaseTag(p domainplugin.InstalledPlugin) string {
	if p.InstallMeta == nil {
		return ""
	}
	return p.InstallMeta.ReleaseTag
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

// CallWithTimeout sends a JSON-RPC request with an explicit timeout override.
func (m *PluginManager) CallWithTimeout(ctx context.Context, pluginID, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	scope, err := m.resolveCommandScope(pluginID)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(method, "auth.") && m.outboundAuthAudit != nil {
		m.outboundAuthAudit(pluginID, method, string(domainplugin.SanitizeAuthRPCParams(method, params)))
	}
	raw, err := m.host.CallWithTimeout(ctx, pluginID, scope, method, params, timeout)
	if err != nil {
		if method == "auth.answerChallenge" && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
			return nil, domainplugin.ErrAuthChallengeTimeout
		}
		return nil, err
	}
	if strings.HasPrefix(method, "auth.") && m.outboundAuthAudit != nil {
		m.outboundAuthAudit(pluginID, method+"#result", string(domainplugin.SanitizeAuthRPCResult(method, raw)))
	}
	return raw, nil
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

// SetOutboundAuthAudit binds audit logging for host→plugin auth.* RPC (sanitized params only).
func (m *PluginManager) SetOutboundAuthAudit(fn OutboundAuthAuditFunc) {
	m.outboundAuthAudit = fn
}

// SetStateChangeHandler emits plugin lifecycle state changes to presentation.
func (m *PluginManager) SetStateChangeHandler(fn func(pluginID, state, sessionID string)) {
	m.stateChange = fn
}

// SetProcessStartedHandler registers a host-internal observer of plugin process starts. It is
// separate from SetStateChangeHandler, which belongs to presentation: this one exists so
// level-triggered subscriptions (discovery's observed set, ADR-014) can be replayed to a plugin
// that just came up, and that must not depend on whether a UI is listening.
func (m *PluginManager) SetProcessStartedHandler(fn func(pluginID string)) {
	m.processStarted = fn
}

// SetProcessStoppedHandler registers a host-internal observer of a plugin being stopped outright —
// disabled by the user, or uninstalled, both of which route through StopPlugin.
//
// It is the mirror of SetProcessStartedHandler and, like it, is separate from the presentation
// state handler: discovery must drop a stopped plugin's subtree whether or not a UI is listening
// (ADR-014). Crashes deliberately do NOT reach here — the supervisor restarts the process and the
// replayed observed set refills the tree, so tearing it down would only make a recoverable blip
// look like a disappearance.
func (m *PluginManager) SetProcessStoppedHandler(fn func(pluginID string)) {
	m.processStopped = fn
}

// SetProcessCrashedHandler registers a host-internal observer of a plugin process dying
// unexpectedly.
//
// It is separate from the stopped handler because the two demand opposite treatments and always
// will: a stop is final and its state should be torn down, while a crash is transient — the
// supervisor restarts the process and the replayed level-triggered state refills it — so a crash
// must only be MARKED, never cleared (ADR-014).
func (m *PluginManager) SetProcessCrashedHandler(fn func(pluginID string)) {
	m.processCrashed = fn
}

// SetProcessSuspendedHandler registers a host-internal observer of idle suspension.
//
// Suspension is deliberate where a crash is not, but to anything holding state the plugin vouched
// for the two are the same event: the process is gone and can confirm nothing until it is started
// again. It gets its own hook rather than reusing the crash one so the manager keeps saying what
// actually happened, and so a future consumer that DOES care about the difference has somewhere to
// hang that off.
func (m *PluginManager) SetProcessSuspendedHandler(fn func(pluginID string)) {
	m.processSuspended = fn
}

func (m *PluginManager) emitStateChange(pluginID, state, sessionID string) {
	// Single choke point for every lifecycle transition, so start/running/
	// suspended/stopped/crashed are all visible in the debug log (previously
	// they only reached the frontend state event).
	slog.Info("plugin state change", "component", "plugin", "pluginId", pluginID, "state", state, "sessionId", sessionID)
	if state == "running" && m.processStarted != nil {
		m.processStarted(pluginID)
	}
	if state == "stopped" && m.processStopped != nil {
		m.processStopped(pluginID)
	}
	if state == "crashed" && m.processCrashed != nil {
		m.processCrashed(pluginID)
	}
	if state == "suspended" && m.processSuspended != nil {
		m.processSuspended(pluginID)
	}
	if m.stateChange != nil {
		m.stateChange(pluginID, state, sessionID)
	}
}
