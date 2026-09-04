package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/loghub"
	"xquakshell/internal/infra/plugin/capability"
)

type PluginAuditFunc = domainplugin.AuditRecorder

type HostServer struct {
	pluginID    string
	gate        *capability.Gate
	fs          *capability.FSProxy
	net         *capability.NetProxy
	vault       *capability.VaultProxy
	sessions    domainplugin.SessionRPCHandler
	events      *capability.EventsProxy
	views       *capability.ViewProxy
	tunnelDial  *capability.TunnelDialProxy
	tunnelLocal *capability.TunnelLocalProxy
	audit       PluginAuditFunc
	onActivity  PluginActivityFunc

	logMu      sync.Mutex
	logCount   int
	logDropped int
	logWindow  time.Time
}

type PluginActivityFunc func(pluginID string)

type HostServerConfig struct {
	PluginID    string
	Gate        *capability.Gate
	FS          *capability.FSProxy
	Net         *capability.NetProxy
	Vault       *capability.VaultProxy
	Sessions    domainplugin.SessionRPCHandler
	Events      *capability.EventsProxy
	Views       *capability.ViewProxy
	TunnelDial  *capability.TunnelDialProxy
	TunnelLocal *capability.TunnelLocalProxy
	Audit       PluginAuditFunc
	OnActivity  PluginActivityFunc
}

func NewHostServer(cfg HostServerConfig) *HostServer {
	return &HostServer{
		pluginID:    cfg.PluginID,
		gate:        cfg.Gate,
		fs:          cfg.FS,
		net:         cfg.Net,
		vault:       cfg.Vault,
		sessions:    cfg.Sessions,
		events:      cfg.Events,
		views:       cfg.Views,
		tunnelDial:  cfg.TunnelDial,
		tunnelLocal: cfg.TunnelLocal,
		audit:       cfg.Audit,
		onActivity:  cfg.OnActivity,
	}
}

// HandleRequest dispatches an incoming plugin RPC and returns a result or RPC error.
func (s *HostServer) HandleRequest(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *RPCError) {
	if !s.gate.Allow(method) {
		s.auditDenied(method, "capability denied")
		return nil, capabilityDeniedError(method)
	}

	var (
		result json.RawMessage
		err    error
	)

	switch method {
	case "log.write":
		allowed, dropped := s.allowLogWrite()
		if dropped > 0 {
			loghub.PublishPluginLog(s.pluginID, "warn",
				"log rate-limited, "+strconv.Itoa(dropped)+" lines dropped",
				map[string]string{"dropped": strconv.Itoa(dropped)})
		}
		if !allowed {
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
		result, err = s.net.Read(ctx, params)
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
	// discovery.publish rides the session RPC handler for one reason: it names a sessionId, and the
	// usecase layer is where "does this plugin hold that session" is decided (ADR-014, and the same
	// path channel.open takes). Nothing about it is decoded here.
	// surface.* rides the session RPC handler for the same reason discovery.publish does:
	// surface.open names a parentSessionId, and "does this plugin hold that session" is a usecase
	// decision. The later verbs name a surfaceId instead, but routing them anywhere else would put
	// one verb family behind two dispatchers.
	case "session.updateState", "session.writeTerminal", "session.registerEmbed", "session.tunnelOpen", "session.tunnelFrame", "session.tunnelClose", "discovery.publish",
		// trust.verifyPeer names a session, so "does this plugin hold this session" is a use case
		// decision, exactly as it is for channel.open.
		"trust.verifyPeer",
		"surface.open", "surface.write", "surface.updateState", "surface.setTitle", "surface.close",
		"dialog.open", "dialog.setError", "dialog.close", "discovery.publishDetails":
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
	case "tunnel.dial":
		if s.tunnelDial == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.tunnelDial.Dial(ctx, params)
	case "tunnel.close":
		if s.tunnelDial == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.tunnelDial.Close(ctx, params)
	case "tunnel.localWrite":
		if s.tunnelLocal == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.tunnelLocal.LocalWrite(ctx, params)
	case "tunnel.localClose":
		if s.tunnelLocal == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.tunnelLocal.LocalClose(ctx, params)
	case "tunnel.bind":
		if s.tunnelLocal == nil {
			return nil, proxyUnavailableError(method)
		}
		result, err = s.tunnelLocal.Bind(ctx, params)
	case "channel.open":
		if s.sessions == nil {
			return nil, &RPCError{Code: -32603, Message: "session handler unavailable"}
		}
		// channel.open dials a real backend (exec spawn / relay dial), so it gets the same
		// 10s allowance as initialize instead of the default inbound RPC timeout.
		slog.Debug("host_server: channel.open received, dispatching to sessions.Handle", "pluginId", s.pluginID)
		openCtx, cancel := context.WithTimeout(ctx, domainplugin.ChannelOpenTimeout)
		result, err = s.sessions.Handle(openCtx, s.pluginID, method, params)
		cancel()
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		slog.Debug("host_server: sessions.Handle(channel.open) returned", "pluginId", s.pluginID, "err", errMsg)
	case "channel.close":
		if s.sessions == nil {
			return nil, &RPCError{Code: -32603, Message: "session handler unavailable"}
		}
		result, err = s.sessions.Handle(ctx, s.pluginID, method, params)
	default:
		s.auditDenied(method, "method not found")
		return nil, &RPCError{Code: -32601, Message: "method not found"}
	}

	rpcErr := s.errorFor(method, err)
	// Recorded here rather than before the dispatch, and never for a refusal. See isRefusal.
	if !isRefusal(rpcErr) {
		s.recordActivity()
	}
	if rpcErr != nil {
		return nil, rpcErr
	}
	return result, nil
}

// errorFor maps a handler's error onto the wire error the plugin sees. Nil in, nil out.
func (s *HostServer) errorFor(method string, err error) *RPCError {
	if err == nil {
		return nil
	}
	if isInvalidParams(err) {
		return invalidParamsError(method)
	}
	if isCapabilityDenied(err) {
		s.auditDenied(method, err.Error())
		return capabilityDeniedError(method)
	}
	if errors.Is(err, domainplugin.ErrSessionNotBound) {
		s.auditDenied(method, err.Error())
		if strings.HasPrefix(method, "auth.") {
			return &RPCError{Code: -32006, Message: "auth attempt not found"}
		}
		return capabilityDeniedError(method)
	}
	if errors.Is(err, domainplugin.ErrAuthChallengeTimeout) {
		return &RPCError{Code: -32007, Message: "auth challenge timeout"}
	}
	if errors.Is(err, domainplugin.ErrAuthProviderBusy) {
		return &RPCError{Code: -32005, Message: "auth provider busy"}
	}
	if errors.Is(err, domainplugin.ErrTunnelAlreadyExists) {
		return &RPCError{Code: -32008, Message: "tunnel already exists"}
	}
	if errors.Is(err, domainplugin.ErrTunnelNotFound) {
		return &RPCError{Code: -32002, Message: "resource not found"}
	}
	if errors.Is(err, domainplugin.ErrRateLimited) {
		return rateLimitedError(method)
	}
	if errors.Is(err, domainplugin.ErrTerminalBackpressure) {
		return rateLimitedError(method)
	}
	if errors.Is(err, domainplugin.ErrNotImplemented) {
		return &RPCError{Code: -32004, Message: "not implemented"}
	}
	if errors.Is(err, domainplugin.ErrHandleNotFound) {
		return &RPCError{Code: -32002, Message: "resource not found"}
	}
	if errors.Is(err, domainplugin.ErrNetworkDialFailed) {
		slog.Debug("plugin net dial failed", "component", "plugin.rpc", "pluginId", s.pluginID, "method", method)
		return &RPCError{Code: -32603, Message: "request failed"}
	}
	slog.Warn("plugin rpc failed", "component", "plugin.rpc", "pluginId", s.pluginID, "method", method, "err", err)
	return &RPCError{Code: -32603, Message: "request failed"}
}

// isRefusal reports whether the host declined to do the work rather than attempted it and failed.
//
// A refusal must never count as plugin activity. The idle sweep reclaims a plugin process that has
// been quiet for five minutes, and activity was recorded the moment a call passed the capability
// gate - before the deeper checks that decide whether this plugin holds the session it named. So a
// plugin retrying a call the host refuses stayed alive forever by being refused: the docker
// discovery plugin, retrying channel.open every five seconds against a session that had closed,
// held its process and wrote an audit row per attempt for as long as the application ran. Being
// denied is the opposite of being useful, and it must not buy immunity from the sweep.
//
// A request that was attempted and failed still counts. A plugin whose fs.read hits a missing file
// is doing real work badly, not asking for something it may not have, and suspending it would
// reclaim a healthy plugin over an ordinary error.
func isRefusal(err *RPCError) bool {
	if err == nil {
		return false
	}
	// Rate limiting joins capability denial for the same reason: it is the host saying no, and a
	// plugin that floods hard enough to be throttled must not be rewarded with a fresh idle clock.
	return err.Code == codeCapabilityDenied || err.Code == codeRateLimited
}

func (s *HostServer) handleLogWrite(params json.RawMessage) {
	payload, redacted := domainplugin.SanitizeLogWriteParams(params)
	if payload.Level == "" && payload.Message == "" && len(payload.Fields) == 0 {
		return
	}
	fields := map[string]string{}
	for k, v := range payload.Fields {
		fields[k] = v
	}
	if redacted {
		fields["redacted"] = "true"
	}
	// Publish exactly once, honoring the plugin's declared level. loghub gates
	// it against the process-wide level floor and tags it plugin:<id>.
	loghub.PublishPluginLog(s.pluginID, payload.Level, payload.Message, fields)
}

// allowLogWrite applies the per-second rate limit. It returns whether the
// current line is allowed and, on a window rollover, how many lines were
// dropped in the previous window so the caller can surface a visible marker
// instead of losing them silently.
func (s *HostServer) allowLogWrite() (allowed bool, droppedToReport int) {
	s.logMu.Lock()
	defer s.logMu.Unlock()

	now := time.Now()
	if now.Sub(s.logWindow) >= time.Second {
		s.logWindow = now
		s.logCount = 0
		droppedToReport = s.logDropped
		s.logDropped = 0
	}
	s.logCount++
	if s.logCount > domainplugin.MaxPluginLogLinesPerSecond {
		s.logDropped++
		return false, droppedToReport
	}
	return true, droppedToReport
}

func (s *HostServer) auditDenied(method, detail string) {
	// Mirror every capability denial into the debug log (in addition to the
	// durable audit trail) so a blocked plugin is visible while investigating.
	slog.Warn("plugin rpc denied", "component", "plugin.rpc", "pluginId", s.pluginID, "method", method,
		"reason", domainplugin.RedactAuditDetail(detail))
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

// The two wire codes that mean the host refused. Named because isRefusal has to recognise them,
// and a bare -32001 repeated in three places is a rule nobody can grep for. The full table lives in
// docs/plugin-api.md; only the codes with a decision attached are named here.
const (
	codeCapabilityDenied = -32001
	codeRateLimited      = -32003
)

func capabilityDeniedError(method string) *RPCError {
	return &RPCError{
		Code:    codeCapabilityDenied,
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
		Code:    codeRateLimited,
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

func (s *HostServer) RequestHandler() RequestHandler {
	return func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *RPCError) {
		return s.HandleRequest(ctx, method, params)
	}
}
