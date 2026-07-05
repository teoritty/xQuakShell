package usecase

import (
	"errors"
	"log/slog"
	"strings"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/safego"
)

// connectSession performs the SSH handshake in a goroutine.
// Order: SSHConnector.Connect → store client → server-alive loop.
// Host key / passphrase UI is driven by HostKeyRequestFunc and PassphraseRequestFunc.
// On host key errors it transitions to SessionHostKeyRequired and waits for RetrySession.
func (m *SessionManager) connectSession(entry *sessionEntry, conn *domain.Connection) {
	result := m.sshConnector.Connect(entry.ctx, conn)
	if result.HostKeyInfo != nil {
		m.applyHostKeyRequired(entry, *result.HostKeyInfo, result.Err)
		return
	}
	if result.Err != nil {
		slog.Error("session connect failed", "sessionID", entry.info.SessionID, "err", result.Err)
		m.updateState(entry, domain.SessionError, sshConnectErrorMessage(result.Err))
		return
	}

	m.registry.Mutate(entry.info.SessionID, func(e *sessionEntry) {
		e.sshClient = result.Client
	})

	if result.JumpCleanup != nil {
		safego.GoNamed("session.jumpCleanup", func() {
			<-entry.ctx.Done()
			result.JumpCleanup()
		})
	}

	safego.GoNamed("session.serverAlive", func() { m.runServerAlive(entry) })

	m.updateState(entry, domain.SessionReady, "")
}

func (m *SessionManager) applyHostKeyRequired(entry *sessionEntry, hkInfo domain.HostKeyInfo, err error) {
	m.registry.Mutate(entry.info.SessionID, func(e *sessionEntry) {
		e.hostKeyInfo = &hkInfo
	})
	msg := "Host key verification required"
	if hkInfo.Mismatch || errors.Is(err, domain.ErrHostKeyMismatch) {
		msg = "Host key mismatch"
	}
	m.updateState(entry, domain.SessionHostKeyRequired, msg)
	if m.hostKeyRequest != nil {
		m.hostKeyRequest(entry.info.SessionID, hkInfo)
	}
}

func sshConnectErrorMessage(err error) string {
	if err == nil {
		return "Connection failed"
	}
	s := err.Error()
	switch {
	case strings.HasPrefix(s, "authentication failed:"):
		return "Authentication failed"
	case strings.HasPrefix(s, "jump chain connection failed:"):
		return "Jump chain connection failed"
	default:
		return "Connection failed"
	}
}
