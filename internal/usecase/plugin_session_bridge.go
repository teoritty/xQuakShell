package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// SessionConnectParams is sent to plugins via session.connect RPC.
type SessionConnectParams struct {
	SessionID    string `json:"sessionId"`
	ConnectionID string `json:"connectionId"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username,omitempty"`
}

// PluginSessionBridge connects non-SSH sessions through out-of-process plugins (ADR-001).
type PluginSessionBridge struct {
	plugins *PluginManager
}

// NewPluginSessionBridge creates a bridge over the plugin manager.
func NewPluginSessionBridge(plugins *PluginManager) *PluginSessionBridge {
	return &PluginSessionBridge{plugins: plugins}
}

// SupportsProtocol reports whether a plugin owns the protocol.
func (b *PluginSessionBridge) SupportsProtocol(protocol string) bool {
	if b == nil || b.plugins == nil {
		return false
	}
	return b.plugins.Registry().HasProtocol(protocol)
}

// Connect starts a plugin session asynchronously via session.connect RPC.
func (b *PluginSessionBridge) Connect(ctx context.Context, pluginID, sessionID string, conn *domain.Connection) error {
	protocol := conn.GetProtocol()
	plugin, err := b.plugins.Registry().Get(pluginID)
	if err != nil {
		return err
	}
	if !plugin.Manifest.AllowsConnectProtocol(protocol) {
		return fmt.Errorf("%w: protocol %q not permitted for plugin %s", domainplugin.ErrCapabilityDenied, protocol, pluginID)
	}

	reason := "onProtocol:" + protocol
	if err := b.plugins.ActivateForSession(ctx, pluginID, sessionID, reason); err != nil {
		return err
	}
	if err := b.plugins.BindSession(pluginID, sessionID); err != nil {
		b.plugins.SessionClosed(ctx, pluginID, sessionID)
		return err
	}
	b.plugins.SessionOpened(pluginID)

	params := SessionConnectParams{
		SessionID:    sessionID,
		ConnectionID: conn.ID,
		Protocol:     conn.GetProtocol(),
		Host:         conn.Host,
		Port:         conn.EffectivePort(),
		Username:     conn.EffectiveUsername(),
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode session.connect: %w", err)
	}

	_, err = b.plugins.CallForSession(ctx, pluginID, sessionID, "session.connect", raw)
	if err != nil {
		b.plugins.UnbindSession(pluginID, sessionID)
		b.plugins.SessionClosed(ctx, pluginID, sessionID)
		return fmt.Errorf("session.connect: %w", err)
	}
	return nil
}

// Disconnect notifies the plugin that a session ended.
func (b *PluginSessionBridge) Disconnect(ctx context.Context, pluginID, sessionID string) {
	params, _ := json.Marshal(map[string]string{"sessionId": sessionID})
	_ = b.plugins.NotifyForSession(ctx, pluginID, sessionID, "session.disconnect", params)
	b.plugins.SessionClosed(ctx, pluginID, sessionID)
	b.plugins.UnbindSession(pluginID, sessionID)
}

// Reconnect re-sends session.connect after a plugin process restart without changing session counts.
func (b *PluginSessionBridge) Reconnect(ctx context.Context, pluginID, sessionID string, conn *domain.Connection) error {
	if b == nil || b.plugins == nil {
		return fmt.Errorf("plugin bridge unavailable")
	}
	protocol := conn.GetProtocol()
	if !connAllowsPluginProtocol(b, pluginID, protocol) {
		return fmt.Errorf("%w: protocol %q not permitted for plugin %s", domainplugin.ErrCapabilityDenied, protocol, pluginID)
	}
	if err := b.plugins.BindSession(pluginID, sessionID); err != nil {
		return err
	}
	params := SessionConnectParams{
		SessionID:    sessionID,
		ConnectionID: conn.ID,
		Protocol:     protocol,
		Host:         conn.Host,
		Port:         conn.EffectivePort(),
		Username:     conn.EffectiveUsername(),
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode session.connect: %w", err)
	}
	_, err = b.plugins.CallForSession(ctx, pluginID, sessionID, "session.connect", raw)
	if err != nil {
		return fmt.Errorf("session.connect: %w", err)
	}
	return nil
}

func connAllowsPluginProtocol(b *PluginSessionBridge, pluginID, protocol string) bool {
	plugin, err := b.plugins.Registry().Get(pluginID)
	if err != nil {
		return false
	}
	return plugin.Manifest.AllowsConnectProtocol(protocol)
}

// CallPlugin sends a JSON-RPC request to a plugin.
func (b *PluginSessionBridge) CallPlugin(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	return b.plugins.Call(ctx, pluginID, method, params)
}

// NotifyForSession sends a JSON-RPC notification to a session-scoped plugin process.
func (b *PluginSessionBridge) NotifyForSession(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error {
	return b.plugins.NotifyForSession(ctx, pluginID, sessionID, method, params)
}

// PluginIDForProtocol resolves the owning plugin for a protocol.
func (b *PluginSessionBridge) PluginIDForProtocol(protocol string) (string, error) {
	return b.plugins.Registry().PluginIDForProtocol(protocol)
}

func decodeTerminalPayload(dataBase64 string) ([]byte, error) {
	if dataBase64 == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid terminal payload", domainplugin.ErrRPC)
	}
	return data, nil
}
