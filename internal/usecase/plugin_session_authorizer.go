package usecase

import (
	"log/slog"
	"strings"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// SessionBindAuditFunc records plugin session bind/unbind security events.
type SessionBindAuditFunc func(pluginID, sessionID, action string, allowed bool, detail string)

// PluginSessionAuthorizer enforces session RPC scope and bound-session policy in the usecase layer.
type PluginSessionAuthorizer struct {
	registry *PluginRegistry
	settings PluginSettingsReader
	audit    SessionBindAuditFunc
	mu       sync.Mutex
	bound    map[string]map[string]struct{} // pluginID -> sessionIDs
}

// NewPluginSessionAuthorizer creates a session RPC authorizer backed by the plugin registry.
func NewPluginSessionAuthorizer(registry *PluginRegistry) *PluginSessionAuthorizer {
	return &PluginSessionAuthorizer{
		registry: registry,
		bound:    make(map[string]map[string]struct{}),
	}
}

// SetSettingsReader binds install-time multi-session consent from vault settings.
func (a *PluginSessionAuthorizer) SetSettingsReader(reader PluginSettingsReader) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.settings = reader
	a.mu.Unlock()
}

// SetBindAudit binds immutable audit logging for session bind/unbind.
func (a *PluginSessionAuthorizer) SetBindAudit(fn SessionBindAuditFunc) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.audit = fn
	a.mu.Unlock()
}

// BindSession registers a session authorized for session.* RPC from the plugin.
func (a *PluginSessionAuthorizer) BindSession(pluginID, sessionID string) error {
	if a == nil {
		return domainplugin.ErrSessionNotBound
	}
	pluginID = strings.TrimSpace(pluginID)
	sessionID = strings.TrimSpace(sessionID)
	if pluginID == "" || sessionID == "" {
		a.auditBind(pluginID, sessionID, "bind", false, "empty plugin or session id")
		return domainplugin.ErrSessionNotBound
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	sessions := a.bound[pluginID]
	if sessions == nil {
		sessions = make(map[string]struct{})
		a.bound[pluginID] = sessions
	}
	if _, ok := sessions[sessionID]; ok {
		a.auditBindLocked(pluginID, sessionID, "bind", true, "already bound")
		return nil
	}

	if err := a.enforceMultiSessionPolicyLocked(pluginID, len(sessions)); err != nil {
		a.auditBindLocked(pluginID, sessionID, "bind", false, err.Error())
		return err
	}

	if allowMulti, count := a.multiSessionStateLocked(pluginID, len(sessions)); allowMulti && count > 0 {
		slog.Warn("plugin bound additional session in allowMultiSession mode",
			"pluginId", pluginID, "sessionId", sessionID, "boundCount", count+1)
	}

	sessions[sessionID] = struct{}{}
	a.auditBindLocked(pluginID, sessionID, "bind", true, "")
	return nil
}

// UnbindSession removes a session from the bound-session registry.
func (a *PluginSessionAuthorizer) UnbindSession(pluginID, sessionID string) {
	if a == nil {
		return
	}
	pluginID = strings.TrimSpace(pluginID)
	sessionID = strings.TrimSpace(sessionID)
	if pluginID == "" || sessionID == "" {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	sessions, ok := a.bound[pluginID]
	if !ok {
		a.auditBindLocked(pluginID, sessionID, "unbind", true, "not bound")
		return
	}
	if _, ok := sessions[sessionID]; !ok {
		a.auditBindLocked(pluginID, sessionID, "unbind", true, "not bound")
		return
	}
	delete(sessions, sessionID)
	if len(sessions) == 0 {
		delete(a.bound, pluginID)
	}
	a.auditBindLocked(pluginID, sessionID, "unbind", true, "")
}

// AuthorizeSessionRPC validates a plugin session RPC target against process scope and bound sessions.
func (a *PluginSessionAuthorizer) AuthorizeSessionRPC(
	pluginID, processSessionID string,
	isolation domainplugin.IsolationMode,
	_ bool,
	targetSessionID string,
) error {
	if a == nil {
		return domainplugin.ErrSessionNotBound
	}
	targetSessionID = strings.TrimSpace(targetSessionID)
	if targetSessionID == "" {
		return domainplugin.ErrSessionNotBound
	}

	switch isolation {
	case domainplugin.IsolationPerSession:
		if processSessionID == "" || targetSessionID != processSessionID {
			return domainplugin.ErrSessionNotBound
		}
		return nil
	case domainplugin.IsolationPerPlugin:
		a.mu.Lock()
		defer a.mu.Unlock()
		sessions, ok := a.bound[pluginID]
		if !ok {
			return domainplugin.ErrSessionNotBound
		}
		if _, ok := sessions[targetSessionID]; !ok {
			return domainplugin.ErrSessionNotBound
		}
		return nil
	default:
		return domainplugin.ErrSessionNotBound
	}
}

func (a *PluginSessionAuthorizer) enforceMultiSessionPolicyLocked(pluginID string, boundCount int) error {
	if boundCount == 0 {
		return nil
	}
	allowMulti, _ := a.multiSessionStateLocked(pluginID, boundCount)
	if !allowMulti {
		isolation := a.effectiveIsolationLocked(pluginID)
		if isolation == domainplugin.IsolationPerPlugin {
			return domainplugin.ErrSessionNotBound
		}
		return nil
	}
	if !a.multiSessionGrantedLocked(pluginID) {
		return domainplugin.ErrSessionNotBound
	}
	return nil
}

func (a *PluginSessionAuthorizer) multiSessionGrantedLocked(pluginID string) bool {
	if a.settings == nil {
		return false
	}
	settings, err := a.settings.PluginSettings()
	if err != nil {
		return false
	}
	if settings.MultiSessionAccessGranted == nil {
		return false
	}
	return settings.MultiSessionAccessGranted[pluginID]
}

func (a *PluginSessionAuthorizer) multiSessionStateLocked(pluginID string, boundCount int) (allowMulti bool, count int) {
	_ = boundCount
	if a.registry == nil {
		return false, 0
	}
	plugin, err := a.registry.Get(pluginID)
	if err != nil {
		return false, 0
	}
	if plugin.Manifest.Capabilities.Session != nil {
		allowMulti = plugin.Manifest.Capabilities.Session.AllowMultiSession
	}
	return allowMulti, len(a.bound[pluginID])
}

func (a *PluginSessionAuthorizer) effectiveIsolationLocked(pluginID string) domainplugin.IsolationMode {
	if a.registry == nil {
		return domainplugin.DefaultIsolation
	}
	plugin, err := a.registry.Get(pluginID)
	if err != nil {
		return domainplugin.DefaultIsolation
	}
	return plugin.Manifest.EffectiveIsolation()
}

func (a *PluginSessionAuthorizer) auditBind(pluginID, sessionID, action string, allowed bool, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.auditBindLocked(pluginID, sessionID, action, allowed, detail)
}

func (a *PluginSessionAuthorizer) auditBindLocked(pluginID, sessionID, action string, allowed bool, detail string) {
	if a.audit == nil {
		return
	}
	a.audit(pluginID, sessionID, action, allowed, detail)
}

var _ domainplugin.SessionRPCAuthorizer = (*PluginSessionAuthorizer)(nil)
