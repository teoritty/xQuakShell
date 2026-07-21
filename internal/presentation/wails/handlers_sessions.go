package wails

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

// --- Sessions ---

// OpenSession starts a new SSH session for the given connection.
// Returns the session ID; the connection process is async.
func (a *AppAPI) OpenSession(connectionID string) (string, error) {
	sessionID, err := a.sessions.OpenSession(context.Background(), connectionID)
	if err != nil {
		return "", err
	}
	slog.Debug("embed: OpenSession RPC returning sessionId to frontend", "pluginId", "com.xquakshell.vnc", "sessionId", sessionID, "connectionId", connectionID)

	if a.plugins != nil {
		a.plugins.PublishCoreEvent(context.Background(), "core.session.opened", map[string]string{
			"sessionId":    sessionID,
			"connectionId": connectionID,
		})
	}

	safego.GoNamed("session.initPTY", func() { a.initSessionPTYAndSFTP(sessionID) })

	return sessionID, nil
}

// CloseSession terminates a session by its ID.
func (a *AppAPI) CloseSession(sessionID string) error {
	err := a.sessions.CloseSession(sessionID)
	if err != nil {
		return err
	}
	if a.auditSvc != nil {
		a.auditSvc.RemoveSession(sessionID)
	}
	if a.remoteFS != nil {
		a.remoteFS.ClearSessionCache(sessionID)
	}
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSessionClosed, map[string]string{"sessionId": sessionID})
	}
	if a.plugins != nil {
		a.plugins.PublishCoreEvent(context.Background(), "core.session.closed", map[string]string{
			"sessionId": sessionID,
		})
	}
	safego.GoNamed("session.gc", func() {
		runtime.GC()
	})
	return nil
}

// GetSessionState returns the current state of a session.
func (a *AppAPI) GetSessionState(sessionID string) (SessionDTO, error) {
	info, err := a.sessions.GetState(sessionID)
	if err != nil {
		return SessionDTO{}, err
	}
	return SessionToDTO(info), nil
}

// GetPlatform returns the current OS: "windows", "linux", "darwin".
func (a *AppAPI) GetPlatform() string {
	return runtime.GOOS
}

// --- Terminal ---

// SendTerminalInput sends keyboard input to a session's PTY and logs to audit.
// commandLine, when non-empty, is the shell input line captured at Enter time by the frontend.
func (a *AppAPI) SendTerminalInput(sessionID, data, commandLine string) error {
	bridge, err := a.sessions.GetPTYBridge(sessionID)
	if err != nil {
		return err
	}
	if err := bridge.Write([]byte(data)); err != nil {
		return err
	}

	if a.auditSvc != nil {
		safego.GoNamed("audit.trackInput", func() { a.trackAuditInput(sessionID, data, commandLine) })
	}
	return nil
}

func (a *AppAPI) trackAuditInput(sessionID, data, commandLine string) {
	if !containsEnter(data) {
		return
	}

	line, ok := a.auditSvc.ResolveCommandLine(sessionID, data, commandLine)
	if !ok {
		return
	}

	if err := a.auditSvc.RecordCommand(context.Background(), sessionID, line); err != nil {
		slog.Warn("audit record command failed", "sessionId", sessionID, "err", err)
	}
}

func containsEnter(data string) bool {
	for _, r := range data {
		if r == '\r' || r == '\n' {
			return true
		}
	}
	return false
}

// TerminalResize changes the PTY window size for a session.
func (a *AppAPI) TerminalResize(sessionID string, cols, rows int) error {
	return a.sessions.ResizeTerminal(sessionID, uint32(cols), uint32(rows))
}

// --- Host Key ---

// ResolveHostKey handles the user's decision on a pending host key verification.
// action is "add" or "replace"; after resolving, retries the session connection.
func (a *AppAPI) ResolveHostKey(sessionID, action, host, authorizedKey string) error {
	if a.hostKeys == nil {
		return fmt.Errorf("host key service unavailable")
	}
	return a.hostKeys.ResolveHostKey(context.Background(), sessionID, action, host, authorizedKey)
}

// ReportEmbedViewport forwards pixel dimensions to the plugin process.
func (a *AppAPI) ReportEmbedViewport(sessionID string, widthPx, heightPx int, devicePixelRatio float64) error {
	slog.Debug("embed: frontend reported viewport (embed panel mounted)", "pluginId", "com.xquakshell.vnc", "sessionId", sessionID, "w", widthPx, "h", heightPx)
	if a.embedBridge == nil {
		return nil
	}
	return a.embedBridge.ReportViewport(context.Background(), sessionID, widthPx, heightPx, devicePixelRatio, true)
}

// ReportEmbedActivity updates broker backpressure when the session tab focus changes.
func (a *AppAPI) ReportEmbedActivity(sessionID string, active bool) error {
	slog.Debug("embed: frontend reported activity (embed panel mounted)", "pluginId", "com.xquakshell.vnc", "sessionId", sessionID, "active", active)
	if a.embedBridge == nil {
		return nil
	}
	return a.embedBridge.ReportActivity(context.Background(), sessionID, active)
}

// --- Internal helpers ---

func (a *AppAPI) onSessionStateChange(session domain.ConnectionSession) {
	slog.Debug("embed: emitting SessionStateChanged to frontend", "pluginId", "com.xquakshell.vnc", "sessionId", session.SessionID, "state", string(session.State), "surface", session.Surface)
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSessionStateChanged, SessionToDTO(session))
	}
	if a.plugins != nil {
		a.plugins.PublishCoreEvent(context.Background(), "core.session.stateChanged", map[string]string{
			"sessionId": session.SessionID,
			"state":     string(session.State),
		})
	}
}

func (a *AppAPI) onHostKeyRequest(sessionID string, info domain.HostKeyInfo) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventHostKeyRequired, map[string]interface{}{
		"sessionId":   sessionID,
		"host":        info.Host,
		"keyType":     info.KeyType,
		"fingerprint": info.Fingerprint,
		"keyBase64":   info.KeyBase64,
		"mismatch":    info.Mismatch,
	})
}

func (a *AppAPI) onPassphraseRequest(identityID, comment string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context for passphrase request")
	}
	_, _ = wailsrt.MessageDialog(a.ctx, wailsrt.MessageDialogOptions{
		Type:    wailsrt.InfoDialog,
		Title:   "Passphrase Required",
		Message: fmt.Sprintf("Key '%s' requires a passphrase. This feature requires a custom dialog.", comment),
	})
	return "", domain.ErrPassphraseRequired
}

// onStreamReady is called when a plugin stream connector has started the terminal bridge.
func (a *AppAPI) onStreamReady(sessionID string, outputCh <-chan []byte) {
	safego.GoNamed("session.streamOutput", func() { a.streamTerminalOutput(sessionID, outputCh) })
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTerminalReady, map[string]interface{}{"sessionId": sessionID})
	}
}

// initSessionPTYAndSFTP delegates PTY and SFTP setup to the SessionManager,
// then emits Wails events when the subsystems are ready.
func (a *AppAPI) initSessionPTYAndSFTP(sessionID string) {
	outputCh, initialPath, err := a.sessions.InitSessionIO(context.Background(), sessionID)
	if err != nil || outputCh == nil {
		return
	}

	safego.GoNamed("session.streamOutput", func() { a.streamTerminalOutput(sessionID, outputCh) })

	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSFTPReady, map[string]interface{}{
			"sessionId":   sessionID,
			"initialPath": initialPath,
		})
	}
}

func (a *AppAPI) streamTerminalOutput(sessionID string, outputCh <-chan []byte) {
	batchTicker := time.NewTicker(50 * time.Millisecond)
	defer batchTicker.Stop()

	var batch []byte

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if a.auditSvc != nil {
			a.auditSvc.FeedOutput(sessionID, string(batch))
		}
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTerminalOutput, TerminalOutputPayload{
				SessionID: sessionID,
				Output:    base64.StdEncoding.EncodeToString(batch),
			})
		}
		batch = batch[:0]
	}

	hasNewline := func(b []byte) bool {
		for _, c := range b {
			if c == '\n' || c == '\r' {
				return true
			}
		}
		return false
	}

	for {
		select {
		case data, ok := <-outputCh:
			if !ok {
				flush()
				if a.auditSvc != nil {
					a.auditSvc.RemoveSession(sessionID)
				}
				a.sessions.NotifySessionDisconnected(sessionID)
				return
			}
			batch = append(batch, data...)
			if hasNewline(batch) {
				flush()
			}
		case <-batchTicker.C:
			flush()
		}
	}
}
