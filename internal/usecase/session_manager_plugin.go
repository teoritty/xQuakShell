package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

const pluginTerminalWriteTimeout = 2 * time.Second

// HandlePluginUpdateState applies plugin-reported session state (IDOR-checked).
func (m *SessionManager) HandlePluginUpdateState(pluginID, sessionID, state, errMsg string) error {
	var entry *sessionEntry
	if !m.registry.View(sessionID, func(e *sessionEntry) {
		if e.pluginID == pluginID {
			entry = e
		}
	}) || entry == nil {
		return domain.ErrSessionNotFound
	}

	switch domain.SessionState(state) {
	case domain.SessionConnecting:
		m.updateState(entry, domain.SessionConnecting, errMsg)
	case domain.SessionReady:
		if entry.sessionSurface == "embed" {
			m.updateState(entry, domain.SessionReady, errMsg)
		} else {
			m.markPluginSessionReady(entry)
			m.updateState(entry, domain.SessionReady, errMsg)
		}
	case domain.SessionError:
		m.updateState(entry, domain.SessionError, errMsg)
	default:
		return fmt.Errorf("unsupported plugin session state %q", state)
	}
	return nil
}

// HandlePluginWriteTerminal pushes terminal bytes from a plugin into the UI stream.
func (m *SessionManager) HandlePluginWriteTerminal(pluginID, sessionID string, data []byte) error {
	var ch chan []byte
	var ctx context.Context
	if !m.registry.View(sessionID, func(entry *sessionEntry) {
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
	case <-time.After(m.pluginTerminalWriteTimeoutOrDefault()):
		slog.Warn("plugin terminal output backpressure", "sessionID", sessionID, "bytes", len(data))
		return domainplugin.ErrTerminalBackpressure
	}
}

func (m *SessionManager) pluginTerminalWriteTimeoutOrDefault() time.Duration {
	if m != nil && m.pluginTerminalWriteTimeout > 0 {
		return m.pluginTerminalWriteTimeout
	}
	return pluginTerminalWriteTimeout
}

func (m *SessionManager) markPluginSessionReady(entry *sessionEntry) {
	sessionID := entry.info.SessionID
	var outputCh chan []byte
	var alreadyReady bool
	m.registry.Mutate(sessionID, func(e *sessionEntry) {
		if e.pluginTerminalReady {
			alreadyReady = true
			return
		}
		e.pluginTerminalReady = true
		outputCh = e.pluginOutput
	})
	if alreadyReady || outputCh == nil || m.onStreamReady == nil {
		return
	}
	m.onStreamReady(sessionID, outputCh)
}

func (m *SessionManager) runPluginSession(entry *sessionEntry, conn *domain.Connection) {
	if m.pluginBridge == nil {
		m.updateState(entry, domain.SessionError, fmt.Sprintf("protocol %s not yet implemented", conn.GetProtocol()))
		return
	}

	pluginID, err := m.pluginBridge.PluginIDForProtocol(conn.GetProtocol())
	if err != nil {
		m.updateState(entry, domain.SessionError, fmt.Sprintf("protocol %s not yet implemented", conn.GetProtocol()))
		return
	}

	sessionID := entry.info.SessionID
	m.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.pluginID = pluginID
	})

	isEmbed := false
	if plugin, err := m.pluginBridge.plugins.Registry().Get(pluginID); err == nil {
		isEmbed = plugin.Manifest.SessionSurface() == "embed"
	}
	if isEmbed {
		m.registry.Mutate(sessionID, func(e *sessionEntry) {
			e.sessionSurface = "embed"
			e.info.Surface = "embed"
		})
	} else {
		m.registry.Mutate(sessionID, func(e *sessionEntry) {
			e.pluginOutput = make(chan []byte, 128)
			e.ptyBridge = &pluginTerminalBridge{
				notify: func(ctx context.Context, method string, params json.RawMessage) error {
					return m.pluginBridge.NotifyForSession(ctx, pluginID, sessionID, method, params)
				},
			}
		})
	}

	if err := m.pluginBridge.Connect(entry.ctx, pluginID, sessionID, conn); err != nil {
		m.updateState(entry, domain.SessionError, err.Error())
		slog.Debug("plugin session connect failed", "sessionID", sessionID, "err", err)
	}
}

var _ PluginSessionSink = (*SessionManager)(nil)

// PluginOwnsConnection reports whether the plugin has an active session for the connection.
func (m *SessionManager) PluginOwnsConnection(pluginID, connectionID string) bool {
	for _, entry := range m.registry.All() {
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
func (m *SessionManager) PluginOwnsSession(pluginID, sessionID string) bool {
	if pluginID == "" || sessionID == "" {
		return false
	}
	entry, ok := m.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return false
	}
	return entry.info.State != domain.SessionClosed
}

// BindPluginSessionForTest assigns plugin ownership on a session (unit tests).
func (m *SessionManager) BindPluginSessionForTest(sessionID, pluginID string, outputBuffer ...int) error {
	buf := 8
	if len(outputBuffer) > 0 {
		buf = outputBuffer[0]
	}
	entry, ok := m.registry.Get(sessionID)
	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		entry = &sessionEntry{
			info: domain.ConnectionSession{
				SessionID: sessionID,
				State:     domain.SessionReady,
			},
			ctx:    ctx,
			cancel: cancel,
		}
		m.registry.Put(sessionID, entry)
	}
	m.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.pluginID = pluginID
		e.pluginOutput = make(chan []byte, buf)
	})
	return nil
}

// HandlePluginProcessCrashed marks plugin-owned sessions for recovery.
func (m *SessionManager) HandlePluginProcessCrashed(pluginID, sessionID string) {
	var targets []*sessionEntry
	for _, entry := range m.registry.All() {
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
		m.updateState(entry, domain.SessionConnecting, "Recovering from plugin crash")
	}
}

// RecoverPluginSession re-sends session.connect after a plugin process restart.
func (m *SessionManager) RecoverPluginSession(ctx context.Context, pluginID, sessionID string) error {
	entry, ok := m.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return domain.ErrSessionNotFound
	}
	if entry.info.State == domain.SessionClosed {
		return domain.ErrSessionNotFound
	}
	connectionID := entry.connectionID

	if m.pluginBridge == nil {
		return fmt.Errorf("plugin bridge unavailable")
	}

	conn, err := m.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return err
	}
	if conn == nil {
		return domain.ErrConnectionNotFound
	}

	return m.pluginBridge.Reconnect(ctx, pluginID, sessionID, conn)
}

// FailPluginSessionRecovery marks a session as failed after recovery attempts are exhausted.
func (m *SessionManager) FailPluginSessionRecovery(pluginID, sessionID string) {
	entry, ok := m.registry.Get(sessionID)
	if !ok || entry.pluginID != pluginID {
		return
	}
	if entry.info.State == domain.SessionClosed {
		return
	}
	m.updateState(entry, domain.SessionError, "Plugin process crashed (recovery failed)")
}

var _ PluginSessionRecoverer = (*SessionManager)(nil)

var _ PluginCrashHandler = (*SessionManager)(nil)
