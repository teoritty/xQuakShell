package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)
// MergedCommand is a plugin-contributed command with owning plugin ID.
type MergedCommand struct {
	PluginID string
	Command  domainplugin.CommandContribution
}

// FullCommandID returns the namespaced command identifier.
func (c MergedCommand) FullCommandID() string {
	return c.PluginID + "." + c.Command.ID
}

// Commands returns merged command contributions from all plugins.
func (r *PluginRegistry) Commands() []MergedCommand {
	return r.CommandsFiltered(nil)
}

// CommandsFiltered returns commands from plugins passing the enabled filter.
func (r *PluginRegistry) CommandsFiltered(enabled func(string) bool) []MergedCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []MergedCommand
	for _, p := range r.plugins {
		if enabled != nil && !enabled(p.Manifest.ID) {
			continue
		}
		for _, cmd := range p.Manifest.Contributions.Commands {
			out = append(out, MergedCommand{
				PluginID: p.Manifest.ID,
				Command:  cmd,
			})
		}
	}
	return out
}

// ExecuteCommand runs a contributed plugin command via command.execute RPC.
func (m *PluginManager) ExecuteCommand(ctx context.Context, pluginID, commandID string, args json.RawMessage) (json.RawMessage, error) {
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return nil, err
	}

	found := false
	for _, c := range plugin.Manifest.Contributions.Commands {
		if c.ID == commandID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: command %s", domainplugin.ErrPluginNotFound, commandID)
	}

	trigger := ActivationTrigger{Kind: ActivationCommand, Value: commandID}
	if MatchesActivation(plugin.Manifest.ActivationEvents, trigger) {
		if err := m.Activate(ctx, pluginID, "onCommand:"+commandID); err != nil {
			return nil, err
		}
	} else if m.host.State(pluginID, "") != domainplugin.ProcessRunning {
		return nil, domainplugin.ErrPluginNotRunning
	}

	m.TouchActivity(pluginID)

	params, err := json.Marshal(map[string]any{
		"commandId": commandID,
		"args":      args,
	})
	if err != nil {
		return nil, fmt.Errorf("encode command.execute: %w", err)
	}
	raw, err := m.Call(ctx, pluginID, "command.execute", params)
	if err != nil {
		return nil, fmt.Errorf("command.execute: %w", err)
	}
	return raw, nil
}

// Activate starts the plugin if needed and sends the activate RPC.
func (m *PluginManager) Activate(ctx context.Context, pluginID, reason string) error {
	sr, detail := parseStartReason(reason)
	if err := m.AuthorizeStart(pluginID, sr, detail); err != nil {
		return err
	}
	m.TouchActivity(pluginID)
	if err := m.EnsureRunning(ctx, pluginID); err != nil {
		return err
	}
	params, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	_, err = m.Call(ctx, pluginID, "activate", params)
	return err
}

// ActivateForSession starts the scoped plugin process and sends activate RPC.
func (m *PluginManager) ActivateForSession(ctx context.Context, pluginID, sessionID, reason string) error {
	sr, detail := parseStartReason(reason)
	if err := m.AuthorizeStart(pluginID, sr, detail); err != nil {
		return err
	}
	m.TouchActivity(pluginID)
	if err := m.EnsureRunningForSession(ctx, pluginID, sessionID); err != nil {
		return err
	}
	params, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	_, err = m.CallProcess(ctx, pluginID, sessionID, "activate", params)
	return err
}

// StopPlugin stops every running OS process for a plugin.
func (m *PluginManager) StopPlugin(ctx context.Context, pluginID string) error {
	if err := m.stopAllPluginInstances(ctx, pluginID); err != nil {
		return err
	}
	m.emitStateChange(pluginID, "stopped", "")
	return nil
}

func parseStartReason(reason string) (StartReason, string) {
	switch {
	case reason == "onStartup":
		return StartReasonStartup, ""
	case strings.HasPrefix(reason, "onCommand:"):
		return StartReasonCommand, strings.TrimPrefix(reason, "onCommand:")
	case strings.HasPrefix(reason, "onProtocol:"):
		return StartReasonProtocol, strings.TrimPrefix(reason, "onProtocol:")
	default:
		return StartReasonManual, reason
	}
}

// StartPluginManual starts a plugin from the UI with audit and state checks (per-plugin isolation only).
func (m *PluginManager) StartPluginManual(ctx context.Context, pluginID string) error {
	plugin, err := m.registry.Get(pluginID)
	if err != nil {
		return err
	}
	if plugin.Manifest.RequiresSessionScopedProcess() {
		return domainplugin.ErrSessionScopeRequired
	}
	st := m.host.State(pluginID, "")
	if st == domainplugin.ProcessRunning {
		return nil
	}
	if st == domainplugin.ProcessStarting {
		return domainplugin.ErrPluginAlreadyRunning
	}
	if err := m.AuthorizeStart(pluginID, StartReasonManual, "ui"); err != nil {
		return err
	}
	return m.EnsureRunning(ctx, pluginID)
}
// ActivateStartupPlugins activates plugins with onStartup activation events.
func (m *PluginManager) ActivateStartupPlugins(ctx context.Context) {
	if m == nil {
		return
	}
	for _, p := range m.registry.PluginsForActivation(ActivationTrigger{Kind: ActivationStartup}) {
		if err := m.Activate(ctx, p.Manifest.ID, "onStartup"); err != nil {
			// Startup plugins are optional; log and continue.
			continue
		}
	}
}

// PublishCoreEvent forwards a core bus event to subscribed plugins.
func (m *PluginManager) PublishCoreEvent(ctx context.Context, channel string, payload any) {
	if m != nil && m.events != nil {
		m.events.PublishCore(ctx, channel, payload)
	}
}
