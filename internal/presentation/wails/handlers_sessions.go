package wails

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
)

// --- Sessions ---

// OpenSession starts a new SSH session for the given connection.
// Returns the session ID; the connection process is async.
func (a *AppAPI) OpenSession(connectionID string) (string, error) {
	sessionID, err := a.sessions.OpenSession(context.Background(), connectionID)
	if err != nil {
		return "", err
	}

	go a.initSessionPTYAndSFTP(sessionID)

	return sessionID, nil
}

// CloseSession terminates a session by its ID.
func (a *AppAPI) CloseSession(sessionID string) error {
	err := a.sessions.CloseSession(sessionID)
	if err != nil {
		return err
	}
	a.auditInputBuffersMu.Lock()
	delete(a.auditInputBuffers, sessionID)
	a.auditInputBuffersMu.Unlock()
	a.ownerCacheMu.Lock()
	delete(a.ownerCache, sessionID)
	delete(a.groupCache, sessionID)
	a.ownerCacheMu.Unlock()
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSessionClosed, map[string]string{"sessionId": sessionID})
	}
	go func() {
		runtime.GC()
	}()
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
func (a *AppAPI) SendTerminalInput(sessionID, data string) error {
	bridge, err := a.sessions.GetPTYBridge(sessionID)
	if err != nil {
		return err
	}
	if err := bridge.Write([]byte(data)); err != nil {
		return err
	}

	if a.auditLog != nil {
		go a.bufferAndAppendAuditInput(sessionID, data)
	}
	return nil
}

// bufferAndAppendAuditInput buffers terminal input and appends to audit only on newline (\n or \r).
// Escape sequences (e.g. arrow keys) are not buffered.
func (a *AppAPI) bufferAndAppendAuditInput(sessionID, data string) {
	if len(data) > 0 && data[0] == '\x1b' && (len(data) == 1 || (len(data) > 1 && data[1] != '\n' && data[1] != '\r')) {
		return
	}

	a.auditInputBuffersMu.Lock()
	buf := a.auditInputBuffers[sessionID] + data
	a.auditInputBuffers[sessionID] = ""
	a.auditInputBuffersMu.Unlock()

	lines := splitLines(buf)
	if len(lines) == 0 {
		return
	}

	for i := 0; i < len(lines)-1; i++ {
		trimmed := trimLineEnding(lines[i])
		if trimmed != "" {
			a.appendAuditEntry(sessionID, trimmed)
		}
	}

	last := lines[len(lines)-1]
	if len(last) > 0 && (last[len(last)-1] == '\n' || last[len(last)-1] == '\r') {
		trimmed := trimLineEnding(last)
		if trimmed != "" {
			a.appendAuditEntry(sessionID, trimmed)
		}
	} else {
		a.auditInputBuffersMu.Lock()
		a.auditInputBuffers[sessionID] = last
		a.auditInputBuffersMu.Unlock()
	}
}

func splitLines(s string) []string {
	var lines []string
	var cur []rune
	for _, r := range s {
		if r == '\n' || r == '\r' {
			cur = append(cur, r)
			lines = append(lines, string(cur))
			cur = cur[:0]
		} else {
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		lines = append(lines, string(cur))
	}
	return lines
}

func trimLineEnding(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func (a *AppAPI) appendAuditEntry(sessionID, input string) {
	sanitizer := a.getSanitizer(sessionID)
	sanitized, redacted := sanitizer.SanitizeInput(input)

	info, err := a.sessions.GetState(sessionID)
	if err != nil {
		return
	}

	conn, _ := a.connRepo.GetByID(context.Background(), info.ConnectionID)
	username := ""
	if conn != nil {
		username = conn.EffectiveUsername()
	}

	entry := domain.AuditEntry{
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		ConnectionID: info.ConnectionID,
		Username:     username,
		Input:        sanitized,
		Redacted:     redacted,
	}
	_ = a.auditLog.Append(context.Background(), entry)
}

func (a *AppAPI) getSanitizer(sessionID string) *auditlog.Sanitizer {
	a.sanitizersMu.Lock()
	defer a.sanitizersMu.Unlock()
	s, ok := a.sanitizers[sessionID]
	if !ok {
		s = auditlog.NewSanitizer()
		a.sanitizers[sessionID] = s
	}
	return s
}

// TerminalResize changes the PTY window size for a session.
func (a *AppAPI) TerminalResize(sessionID string, cols, rows int) error {
	return a.sessions.ResizeTerminal(sessionID, uint32(cols), uint32(rows))
}

// --- Host Key ---

// ResolveHostKey handles the user's decision on a pending host key verification.
// action is "add" or "replace"; after resolving, retries the session connection.
func (a *AppAPI) ResolveHostKey(sessionID, action, host, authorizedKey string) error {
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return fmt.Errorf("parse host key: %w", err)
	}

	switch action {
	case "add":
		if err := a.knownHosts.Add(context.Background(), host, key); err != nil {
			return fmt.Errorf("add host key: %w", err)
		}
	case "replace":
		if err := a.knownHosts.Replace(context.Background(), host, key); err != nil {
			return fmt.Errorf("replace host key: %w", err)
		}
	default:
		return fmt.Errorf("unknown host key action %q", action)
	}

	return a.sessions.RetrySession(context.Background(), sessionID)
}

// --- Internal helpers ---

func (a *AppAPI) onSessionStateChange(session domain.ConnectionSession) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventSessionStateChanged, SessionToDTO(session))
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
	go a.streamTerminalOutput(sessionID, outputCh)
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

	go a.streamTerminalOutput(sessionID, outputCh)

	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventSFTPReady, map[string]interface{}{
			"sessionId":   sessionID,
			"initialPath": initialPath,
		})
	}
}

func (a *AppAPI) streamTerminalOutput(sessionID string, outputCh <-chan []byte) {
	sanitizer := a.getSanitizer(sessionID)
	batchTicker := time.NewTicker(50 * time.Millisecond)
	defer batchTicker.Stop()

	var batch []byte

	flush := func() {
		if len(batch) == 0 {
			return
		}
		sanitizer.FeedOutput(string(batch))
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
				a.sanitizersMu.Lock()
				delete(a.sanitizers, sessionID)
				a.sanitizersMu.Unlock()
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
