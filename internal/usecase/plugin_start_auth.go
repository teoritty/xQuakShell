package usecase

import (
	"context"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// StartReason identifies why a plugin process may be started.
type StartReason string

const (
	StartReasonStartup  StartReason = "startup"
	StartReasonProtocol StartReason = "protocol"
	StartReasonCommand  StartReason = "command"
	StartReasonManual   StartReason = "manual"
	StartReasonView     StartReason = "view"
	StartReasonAuth     StartReason = "auth"
)

// PluginStartAuditFunc records plugin start authorization attempts.
type PluginStartAuditFunc func(pluginID, reason, detail string, denied bool)

// AuthorizeStart checks whether a plugin may be started for the given reason.
func (m *PluginManager) AuthorizeStart(pluginID string, reason StartReason, detail string) error {
	if m == nil {
		return fmt.Errorf("plugin manager unavailable")
	}
	if _, err := m.registry.Get(pluginID); err != nil {
		m.auditStart(pluginID, string(reason), detail, true)
		return err
	}
	if !m.isPluginEnabled(pluginID) {
		m.auditStart(pluginID, string(reason), detail, true)
		return domainplugin.ErrPluginDisabled
	}

	plugin, _ := m.registry.Get(pluginID)
	var allowed bool
	switch reason {
	case StartReasonStartup:
		allowed = MatchesActivation(plugin.Manifest.ActivationEvents, ActivationTrigger{Kind: ActivationStartup})
	case StartReasonProtocol:
		allowed = MatchesActivation(plugin.Manifest.ActivationEvents, ActivationTrigger{
			Kind:  ActivationProtocol,
			Value: detail,
		})
	case StartReasonCommand:
		allowed = MatchesActivation(plugin.Manifest.ActivationEvents, ActivationTrigger{
			Kind:  ActivationCommand,
			Value: detail,
		})
	case StartReasonManual:
		allowed = MatchesActivation(plugin.Manifest.ActivationEvents, ActivationTrigger{Kind: ActivationManual})
	case StartReasonView:
		allowed = plugin.Manifest.HasViews() && MatchesActivation(plugin.Manifest.ActivationEvents, ActivationTrigger{
			Kind:  ActivationView,
			Value: detail,
		})
	case StartReasonAuth:
		allowed = MatchesActivation(plugin.Manifest.ActivationEvents, ActivationTrigger{
			Kind:  ActivationAuth,
			Value: detail,
		})
	default:
		allowed = false
	}

	if !allowed {
		m.auditStart(pluginID, string(reason), detail, true)
		return domainplugin.ErrCapabilityDenied
	}
	m.auditStart(pluginID, string(reason), detail, false)
	return nil
}

func (m *PluginManager) isPluginEnabled(pluginID string) bool {
	if m.settingsReader == nil {
		return true
	}
	settings, err := m.settingsReader.PluginSettings()
	if err != nil {
		return false
	}
	if settings.Disabled == nil {
		return true
	}
	return !settings.Disabled[pluginID]
}

// sandboxPolicy is this host's decision about what to do when a platform that can confine a plugin
// fails to.
//
// The decision lives here and only here. Infra reports two facts — can this platform confine, and
// did this attempt succeed — and must not learn which setting governs the answer; that is what
// keeps "may a plugin run unconfined" a policy question rather than a property of a syscall
// wrapper.
//
// An unreadable settings store answers no. A read error is not consent, and defaulting to the
// permissive side of a security setting because the file would not open is how an escape hatch
// becomes the normal path.
func (m *PluginManager) sandboxPolicy() domainplugin.SandboxPolicy {
	if m.settingsReader == nil {
		return domainplugin.SandboxPolicy{}
	}
	settings, err := m.settingsReader.PluginSettings()
	if err != nil {
		return domainplugin.SandboxPolicy{}
	}
	return domainplugin.SandboxPolicy{AllowUnsandboxedFallback: settings.AllowUnsandboxedFallback}
}

// auditPluginSandbox records what boundary a plugin process actually came up behind.
//
// It is written on every successful start, including — especially — when the answer is that there
// is none. An audit log that only recorded the confined starts would answer "was this plugin
// sandboxed?" with silence in exactly the case where the question matters, and silence reads as
// "no entry" rather than as "no sandbox".
func (m *PluginManager) auditPluginSandbox(pluginID, scope string) {
	mode := m.sandboxModeForScope(pluginID, scope)
	if mode == "" {
		return
	}
	m.auditStart(pluginID, "start", "sandbox="+string(mode), false)
}

func (m *PluginManager) auditStart(pluginID, reason, detail string, denied bool) {
	if m.startAudit == nil {
		return
	}
	line := detail
	if line == "" {
		line = reason
	}
	m.startAudit(pluginID, reason, line, denied)
}

// IsPluginEnabled reports whether the user has not disabled the plugin.
func (m *PluginManager) IsPluginEnabled(pluginID string) bool {
	return m.isPluginEnabled(pluginID)
}

// SetPluginEnabled persists the user enable/disable toggle.
func (m *PluginManager) SetPluginEnabled(ctx context.Context, pluginID string, enabled bool) error {
	if m.pluginSettings == nil {
		return fmt.Errorf("plugin settings unavailable")
	}
	return m.pluginSettings.SetPluginEnabled(ctx, pluginID, enabled)
}
