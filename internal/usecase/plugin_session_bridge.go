package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// SessionConnectParams is sent to plugins via session.connect RPC.
type SessionConnectParams struct {
	SessionID    string            `json:"sessionId"`
	ConnectionID string            `json:"connectionId"`
	Protocol     string            `json:"protocol"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	Username     string            `json:"username,omitempty"`
	Fields       map[string]string `json:"fields,omitempty"`
}

// PluginSessionBridge connects non-SSH sessions through out-of-process plugins (ADR-001).
//
// WHY plugin-session protocol methods live here and not in SessionManager (ADR-009):
// A plugin session's state machine (connect → ready → crash → recover) is
// plugin-subsystem domain logic, not generic session bookkeeping. Before
// ADR-009 these methods lived on SessionManager and reached into
// PluginSessionBridge's internals to do their job — a sign they were in the
// wrong place. They now own that logic directly against SessionRegistry.
type PluginSessionBridge struct {
	plugins *PluginManager
	fields  *PluginFieldsService
	audit   domainplugin.SessionAuditor

	registry                   *SessionRegistry
	connRepo                   domain.ConnectionRepository
	onStateChange              StateChangeFunc
	onStreamReady              OnStreamReadyFunc
	pluginTerminalWriteTimeout time.Duration
}

// PluginSessionBridgeConfig configures the plugin session bridge.
type PluginSessionBridgeConfig struct {
	Plugins *PluginManager
	Fields  *PluginFieldsService
	Audit   domainplugin.SessionAuditor
}

// PluginSessionRuntimeConfig wires session registry and callbacks after SessionManager creation.
type PluginSessionRuntimeConfig struct {
	Registry                   *SessionRegistry
	ConnRepo                   domain.ConnectionRepository
	OnStateChange              StateChangeFunc
	OnStreamReady              OnStreamReadyFunc
	PluginTerminalWriteTimeout time.Duration
}

// WireSessionRuntime binds registry and presentation callbacks (called from NewSessionManager).
func (b *PluginSessionBridge) WireSessionRuntime(cfg PluginSessionRuntimeConfig) {
	if b == nil {
		return
	}
	b.registry = cfg.Registry
	b.connRepo = cfg.ConnRepo
	b.onStateChange = cfg.OnStateChange
	b.onStreamReady = cfg.OnStreamReady
	b.pluginTerminalWriteTimeout = cfg.PluginTerminalWriteTimeout
}

// NewPluginSessionBridge creates a bridge over the plugin manager.
func NewPluginSessionBridge(cfg PluginSessionBridgeConfig) *PluginSessionBridge {
	return &PluginSessionBridge{
		plugins: cfg.Plugins,
		fields:  cfg.Fields,
		audit:   cfg.Audit,
	}
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

	params, err := b.buildConnectParams(ctx, pluginID, sessionID, conn)
	if err != nil {
		b.plugins.UnbindSession(pluginID, sessionID)
		b.plugins.SessionClosed(ctx, pluginID, sessionID)
		return err
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode session.connect: %w", err)
	}

	_, err = b.plugins.CallForSession(ctx, pluginID, sessionID, "session.connect", raw)
	if err != nil {
		b.recordAudit(pluginID, params, false, err.Error())
		b.plugins.UnbindSession(pluginID, sessionID)
		b.plugins.SessionClosed(ctx, pluginID, sessionID)
		return fmt.Errorf("session.connect: %w", err)
	}
	b.recordAudit(pluginID, params, true, "")
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
	params, err := b.buildConnectParams(ctx, pluginID, sessionID, conn)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode session.connect: %w", err)
	}
	_, err = b.plugins.CallForSession(ctx, pluginID, sessionID, "session.connect", raw)
	if err != nil {
		b.recordAudit(pluginID, params, false, err.Error())
		return fmt.Errorf("session.connect: %w", err)
	}
	b.recordAudit(pluginID, params, true, "")
	return nil
}

func (b *PluginSessionBridge) buildConnectParams(ctx context.Context, pluginID, sessionID string, conn *domain.Connection) (SessionConnectParams, error) {
	protocol := conn.GetProtocol()
	protoDef := b.plugins.Registry().GetProtocolDef(pluginID, protocol)
	if protoDef == nil {
		return SessionConnectParams{}, fmt.Errorf("plugin %q not registered for protocol %q", pluginID, protocol)
	}

	var resolvedFields map[string]string
	if b.fields != nil {
		var err error
		resolvedFields, err = b.fields.ResolvePluginFields(ctx, conn, protoDef)
		if err != nil {
			return SessionConnectParams{}, fmt.Errorf("resolve fields: %w", err)
		}
	}

	params := SessionConnectParams{
		SessionID:    sessionID,
		ConnectionID: conn.ID,
		Protocol:     protocol,
		Host:         conn.EffectiveHost(),
		Port:         conn.EffectivePort(b.plugins.Registry()),
		Username:     conn.EffectiveUsername(),
		Fields:       resolvedFields,
	}

	if err := b.capabilityGate(params, protoDef); err != nil {
		return SessionConnectParams{}, err
	}
	return params, nil
}

func (b *PluginSessionBridge) capabilityGate(params SessionConnectParams, protoDef *domainplugin.ProtocolDef) error {
	allowedFields := protoDef.GetFieldIDs()
	for fieldID := range params.Fields {
		if !allowedFields[fieldID] {
			return fmt.Errorf("%w: field %q not declared in manifest", domainplugin.ErrCapabilityDenied, fieldID)
		}
	}
	return nil
}

func (b *PluginSessionBridge) recordAudit(pluginID string, params SessionConnectParams, success bool, errMsg string) {
	if b.audit == nil {
		return
	}
	b.audit.RecordSessionAudit(domainplugin.SessionAuditEntry{
		Timestamp:    time.Now(),
		PluginID:     pluginID,
		Action:       "session.connect",
		ConnectionID: params.ConnectionID,
		Protocol:     params.Protocol,
		FieldCount:   len(params.Fields),
		Success:      success,
		Error:        errMsg,
	})
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
