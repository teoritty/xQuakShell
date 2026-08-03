package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

const pluginTerminalWriteTimeout = 2 * time.Second

// HandlePluginUpdateState applies plugin-reported session state (IDOR-checked).
func (b *PluginSessionBridge) HandlePluginUpdateState(pluginID, sessionID, state, errMsg string) error {
	var entry *sessionEntry
	if !b.registry.View(sessionID, func(e *sessionEntry) {
		if e.pluginID == pluginID {
			entry = e
		}
	}) || entry == nil {
		return domain.ErrSessionNotFound
	}

	switch domain.SessionState(state) {
	case domain.SessionConnecting:
		b.updateSessionState(entry, domain.SessionConnecting, errMsg)
	case domain.SessionReady:
		if entry.sessionSurface == "embed" {
			b.updateSessionState(entry, domain.SessionReady, errMsg)
		} else {
			b.markPluginSessionReady(entry)
			b.updateSessionState(entry, domain.SessionReady, errMsg)
		}
	case domain.SessionError:
		b.updateSessionState(entry, domain.SessionError, errMsg)
	default:
		return fmt.Errorf("unsupported plugin session state %q", state)
	}
	return nil
}

// HandlePluginWriteTerminal pushes terminal bytes from a plugin into the UI stream.
func (b *PluginSessionBridge) HandlePluginWriteTerminal(pluginID, sessionID string, data []byte) error {
	var ch chan []byte
	var ctx context.Context
	if !b.registry.View(sessionID, func(entry *sessionEntry) {
		if entry.pluginID == pluginID && entry.pluginOutput != nil {
			ch = entry.pluginOutput
			ctx = entry.ctx
		}
	}) || ch == nil {
		return domain.ErrSessionNotFound
	}

	select {
	case ch <- data:
		return nil
	case <-ctx.Done():
		return domain.ErrSessionNotFound
	case <-time.After(b.pluginTerminalWriteTimeoutOrDefault()):
		slog.Warn("plugin terminal output backpressure", "sessionID", sessionID, "bytes", len(data))
		return domainplugin.ErrTerminalBackpressure
	}
}

func (b *PluginSessionBridge) pluginTerminalWriteTimeoutOrDefault() time.Duration {
	if b != nil && b.pluginTerminalWriteTimeout > 0 {
		return b.pluginTerminalWriteTimeout
	}
	return pluginTerminalWriteTimeout
}

func (b *PluginSessionBridge) markPluginSessionReady(entry *sessionEntry) {
	sessionID := entry.info.SessionID
	var outputCh chan []byte
	var alreadyReady bool
	b.registry.Mutate(sessionID, func(e *sessionEntry) {
		if e.pluginTerminalReady {
			alreadyReady = true
			return
		}
		e.pluginTerminalReady = true
		outputCh = e.pluginOutput
	})
	if alreadyReady || outputCh == nil || b.onStreamReady == nil {
		return
	}
	b.onStreamReady(sessionID, outputCh)
}

// RunSession starts a plugin-owned session asynchronously.
func (b *PluginSessionBridge) RunSession(sessionID string, conn *domain.Connection) {
	entry, ok := b.registry.Get(sessionID)
	if !ok {
		return
	}
	if b.plugins == nil {
		b.updateSessionState(entry, domain.SessionError, fmt.Sprintf("protocol %s not yet implemented", conn.GetProtocol()))
		return
	}

	pluginID, err := b.PluginIDForProtocol(conn.GetProtocol())
	if err != nil {
		b.updateSessionState(entry, domain.SessionError, fmt.Sprintf("protocol %s not yet implemented", conn.GetProtocol()))
		return
	}

	b.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.pluginID = pluginID
	})

	isEmbed := false
	if plugin, err := b.plugins.Registry().Get(pluginID); err == nil {
		isEmbed = plugin.Manifest.SessionSurface() == "embed"
	}
	if isEmbed {
		b.registry.Mutate(sessionID, func(e *sessionEntry) {
			e.sessionSurface = "embed"
			e.info.Surface = "embed"
		})
	} else {
		b.registry.Mutate(sessionID, func(e *sessionEntry) {
			e.pluginOutput = make(chan []byte, 128)
			e.ptyBridge = &pluginTerminalBridge{
				notify: func(ctx context.Context, method string, params json.RawMessage) error {
					return b.NotifyForSession(ctx, pluginID, sessionID, method, params)
				},
			}
		})
	}

	if err := b.Connect(entry.ctx, pluginID, sessionID, conn); err != nil {
		b.updateSessionState(entry, domain.SessionError, err.Error())
		slog.Debug("plugin session connect failed", "sessionID", sessionID, "err", err)
	}
}

func (b *PluginSessionBridge) updateSessionState(entry *sessionEntry, state domain.SessionState, errMsg string) {
	sessionID := entry.info.SessionID
	var info domain.ConnectionSession
	if !b.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.info.State = state
		e.info.ErrorMessage = errMsg
		info = e.info
		e.signalReadyIfTerminal(state)
	}) {
		return
	}
	if b.onStateChange != nil {
		b.onStateChange(info)
	}
}

var _ PluginSessionSink = (*PluginSessionBridge)(nil)

// PluginOwnsConnection reports whether the plugin has an active session for the connection.
func (b *PluginSessionBridge) PluginOwnsConnection(pluginID, connectionID string) bool {
	for _, entry := range b.registry.All() {
		if entry.pluginID != pluginID || entry.connectionID != connectionID {
			continue
		}
		if entry.info.State == domain.SessionClosed {
			continue
		}
		return true
	}
	return false
}

// PluginOwnsSession reports whether the plugin owns an active session by ID.
func (b *PluginSessionBridge) PluginOwnsSession(pluginID, sessionID string) bool {
	if pluginID == "" || sessionID == "" {
		return false
	}
	entry, ok := b.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return false
	}
	return entry.info.State != domain.SessionClosed
}

// BindPluginSessionForTest assigns plugin ownership on a session (unit tests).
func (b *PluginSessionBridge) BindPluginSessionForTest(sessionID, pluginID string, outputBuffer ...int) error {
	buf := 8
	if len(outputBuffer) > 0 {
		buf = outputBuffer[0]
	}
	if _, ok := b.registry.Get(sessionID); !ok {
		ctx, cancel := context.WithCancel(context.Background())
		entry := newSessionEntry(domain.ConnectionSession{
			SessionID: sessionID,
			State:     domain.SessionReady,
		}, ctx, cancel, "")
		b.registry.Put(sessionID, entry)
	}
	b.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.pluginID = pluginID
		e.pluginOutput = make(chan []byte, buf)
	})
	return nil
}

// HandlePluginProcessCrashed marks plugin-owned sessions for recovery.
func (b *PluginSessionBridge) HandlePluginProcessCrashed(pluginID, sessionID string) {
	var targets []*sessionEntry
	for _, entry := range b.registry.All() {
		if entry.pluginID != pluginID {
			continue
		}
		if sessionID != "" && entry.info.SessionID != sessionID {
			continue
		}
		if entry.info.State == domain.SessionClosed {
			continue
		}
		targets = append(targets, entry)
	}

	for _, entry := range targets {
		b.updateSessionState(entry, domain.SessionConnecting, "Recovering from plugin crash")
	}
}

// RecoverPluginSession re-sends session.connect after a plugin process restart.
func (b *PluginSessionBridge) RecoverPluginSession(ctx context.Context, pluginID, sessionID string) error {
	entry, ok := b.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return domain.ErrSessionNotFound
	}
	if entry.info.State == domain.SessionClosed {
		return domain.ErrSessionNotFound
	}
	connectionID := entry.connectionID

	if b.plugins == nil {
		return fmt.Errorf("plugin bridge unavailable")
	}

	conn, err := b.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return err
	}
	if conn == nil {
		return domain.ErrConnectionNotFound
	}

	return b.Reconnect(ctx, pluginID, sessionID, conn)
}

// FailPluginSessionRecovery marks a session as failed after recovery attempts are exhausted.
func (b *PluginSessionBridge) FailPluginSessionRecovery(pluginID, sessionID string) {
	entry, ok := b.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return
	}
	if entry.info.State == domain.SessionClosed {
		return
	}
	b.updateSessionState(entry, domain.SessionError, "Plugin process crashed (recovery failed)")
}

var _ PluginSessionRecoverer = (*PluginSessionBridge)(nil)

var _ PluginCrashHandler = (*PluginSessionBridge)(nil)
