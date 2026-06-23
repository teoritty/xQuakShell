package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

// PluginAuditFunc records plugin security events without secrets.
type PluginAuditFunc = domainplugin.AuditRecorder

// HostServer handles plugin→core JSON-RPC requests.
type HostServer struct {
	pluginID   string
	gate       *capability.Gate
	fs         *capability.FSProxy
	net        *capability.NetProxy
	vault      *capability.VaultProxy
	sessions   domainplugin.SessionRPCHandler
	events     *capability.EventsProxy
	views      *capability.ViewProxy
	audit      PluginAuditFunc
	onActivity PluginActivityFunc

	logMu     sync.Mutex
	logCount  int
	logWindow time.Time
}

// PluginActivityFunc records inbound plugin RPC activity (idle suspend, metrics).
type PluginActivityFunc func(pluginID string)

// HostServerConfig configures a plugin host RPC server.
type HostServerConfig struct {
	PluginID string
	Gate     *capability.Gate
	FS       *capability.FSProxy
	Net      *capability.NetProxy
	Vault    *capability.VaultProxy
	Sessions domainplugin.SessionRPCHandler
	Events   *capability.EventsProxy
	Views    *capability.ViewProxy
	Audit    PluginAuditFunc
	OnActivity PluginActivityFunc
}

// NewHostServer creates a host-side RPC dispatcher.
func NewHostServer(cfg HostServerConfig) *HostServer {
	return &HostServer{
		pluginID:   cfg.PluginID,
		gate:       cfg.Gate,
		fs:         cfg.FS,
		net:        cfg.Net,
		vault:      cfg.Vault,
		sessions:   cfg.Sessions,
		events:     cfg.Events,
		views:      cfg.Views,
		audit:      cfg.Audit,
		onActivity: cfg.OnActivity,
	}
}

// HandleRequest dispatches an incoming plugin RPC and returns a result or RPC error.
func (s *HostServer) HandleRequest(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *RPCError) {
	if !s.gate.Allow(method) {
		s.auditDenied(method, "capability denied")
		return nil, capabilityDeniedError(method)
	}
	s.recordActivity()

	var (
		result json.RawMessage
		err    error
	)

	switch method {
	case "log.write":
		if !s.allowLogWrite() {
			return nil, rateLimitedError(method)
		}
		s.handleLogWrite(params)
		result, err = json.Marshal(map[string]bool{"ok": true})
	case "ping":
		result, err = json.Marshal(map[string]string{"pong": "ok"})
	case "fs.read", "fs.write", "fs.list":
		if s.fs == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.fs.Handle(method, params)
	case "net.dial":
		if s.net == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.net.Dial(params)
	case "net.close":
		if s.net == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.net.Close(params)
	case "net.read":
		if s.net == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.net.Read(params)
	case "net.write":
		if s.net == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.net.Write(params)
	case "vault.getConnection":
		if s.vault == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.vault.GetConnection(ctx, s.pluginID, params)
	case "vault.getSecret":
		if s.vault == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.vault.GetSecret(ctx, s.pluginID, params)
	case "session.updateState", "session.writeTerminal":
		if s.sessions == nil {
			return nil, &RPCError{Code: -32603, Message: "session handler unavailable"}
		}
		result, err = s.sessions.Handle(ctx, s.pluginID, method, params)
	case "events.subscribe", "events.publish":
		if s.events == nil {
			return nil, &RPCError{Code: -32603, Message: "event handler unavailable"}
		}
		result, err = s.events.Handle(ctx, s.pluginID, method, params)
	case "view.postMessage":
		if s.views == nil {
			return nil, &RPCError{Code: -32603, Message: "view handler unavailable"}
		}
		result, err = s.views.Handle(ctx, s.pluginID, method, params)
	default:
		s.auditDenied(method, "method not found")
		return nil, &RPCError{Code: -32601, Message: "method not found"}
	}

	if err != nil {
		if isInvalidParams(err) {
			return nil, invalidParamsError(method)
		}
		if isCapabilityDenied(err) {
			s.auditDenied(method, err.Error())
			return nil, capabilityDeniedError(method)
		}
		if errors.Is(err, domainplugin.ErrSessionNotBound) {
			s.auditDenied(method, err.Error())
			return nil, capabilityDeniedError(method)
		}
		if errors.Is(err, domainplugin.ErrRateLimited) {
			return nil, rateLimitedError(method)
		}
		if errors.Is(err, domainplugin.ErrTerminalBackpressure) {
			return nil, rateLimitedError(method)
		}
		if errors.Is(err, domainplugin.ErrNotImplemented) {
			return nil, &RPCError{Code: -32004, Message: "not implemented"}
		}
		if errors.Is(err, domainplugin.ErrHandleNotFound) {
			return nil, &RPCError{Code: -32002, Message: "resource not found"}
		}
		if errors.Is(err, domainplugin.ErrNetworkDialFailed) {
			slog.Debug("plugin net dial failed", "pluginId", s.pluginID, "method", method)
			return nil, &RPCError{Code: -32603, Message: "request failed"}
		}
		slog.Debug("plugin rpc failed", "pluginId", s.pluginID, "method", method, "err", err)
		return nil, &RPCError{Code: -32603, Message: "request failed"}
	}
	return result, nil
}

func (s *HostServer) handleLogWrite(params json.RawMessage) {
	payload, redacted := domainplugin.SanitizeLogWriteParams(params)
	if payload.Level == "" && payload.Message == "" && len(payload.Fields) == 0 {
		return
	}
	if redacted {
		slog.Info("plugin log", "pluginId", s.pluginID, "level", payload.Level, "message", payload.Message, "fields", payload.Fields, "redacted", true)
		return
	}
	if len(payload.Fields) > 0 {
		slog.Info("plugin log", "pluginId", s.pluginID, "level", payload.Level, "message", payload.Message, "fields", payload.Fields)
		return
	}
	slog.Info("plugin log", "pluginId", s.pluginID, "level", payload.Level, "message", payload.Message)
}

func (s *HostServer) allowLogWrite() bool {
	s.logMu.Lock()
	defer s.logMu.Unlock()

	now := time.Now()
	if now.Sub(s.logWindow) >= time.Second {
		s.logWindow = now
		s.logCount = 0
	}
	s.logCount++
	return s.logCount <= domainplugin.MaxPluginLogLinesPerSecond
}

func (s *HostServer) auditDenied(method, detail string) {
	if s.audit != nil {
		s.audit(s.pluginID, method, true, domainplugin.RedactAuditDetail(detail))
	}
}

func (s *HostServer) recordActivity() {
	if s.onActivity != nil {
		s.onActivity(s.pluginID)
	}
}

func proxyUnavailableError(method string) *RPCError {
	return &RPCError{
		Code:    -32603,
		Message: "request failed",
		Data:    mustJSON(map[string]string{"method": method}),
	}
}

func capabilityDeniedError(method string) *RPCError {
	return &RPCError{
		Code:    -32001,
		Message: "capability denied",
		Data:    mustJSON(map[string]string{"method": method}),
	}
}

func invalidParamsError(method string) *RPCError {
	return &RPCError{
		Code:    -32602,
		Message: "invalid params",
		Data:    mustJSON(map[string]string{"method": method}),
	}
}

func rateLimitedError(method string) *RPCError {
	return &RPCError{
		Code:    -32003,
		Message: "rate limited",
		Data:    mustJSON(map[string]string{"method": method}),
	}
}

func isInvalidParams(err error) bool {
	var syntaxErr *json.SyntaxError
	var unmarshalErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &unmarshalErr)
}

func isCapabilityDenied(err error) bool {
	return errors.Is(err, domainplugin.ErrCapabilityDenied)
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// RequestHandler adapts HostServer to Conn request callbacks.
func (s *HostServer) RequestHandler() RequestHandler {
	return func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *RPCError) {
		return s.HandleRequest(ctx, method, params)
	}
}
