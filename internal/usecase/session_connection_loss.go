package usecase

import (
	"context"
	"log/slog"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

// defaultTransportProbeTimeout bounds how long a closed terminal waits to learn whether the SSH
// transport under it is still alive.
//
// The probe answers fast in both real cases: a shell that exited leaves a healthy transport that
// replies within a round trip, and a dropped link has usually already failed the transport, so the
// request errors immediately. The bound exists for the case in between - a link that died a moment
// after the server sent EOF - where the request would otherwise wait out the TCP timeout.
const defaultTransportProbeTimeout = 5 * time.Second

const (
	connectionLostMessage = "Connection lost"
	shellExitedMessage    = "Remote shell exited"
)

// NotifySessionDisconnected handles the terminal output of an SSH session ending.
//
// That happens for two reasons that look identical from here: the remote shell exited, or the
// link underneath it dropped. They must not be reported the same way, because the frontend
// reconnects automatically on a lost connection and a user who typed `exit` has not lost anything.
// Asking the transport tells them apart - an exited shell leaves the SSH connection itself alive.
func (s *SessionLifecycleService) NotifySessionDisconnected(sessionID string) {
	entry, client, ok := s.readySession(sessionID)
	if !ok {
		return
	}
	if client != nil && s.transportAlive(entry.ctx, client) {
		slog.Info("session shell exited", "component", "session", "sessionID", sessionID)
		s.updateState(entry, domain.SessionError, shellExitedMessage)
		return
	}
	s.markConnectionLost(entry)
}

// NotifyConnectionLost handles a failure already known to be the transport's - a keepalive that
// went unanswered - so there is nothing left to probe.
func (s *SessionLifecycleService) NotifyConnectionLost(sessionID string) {
	entry, _, ok := s.readySession(sessionID)
	if !ok {
		return
	}
	s.markConnectionLost(entry)
}

// readySession returns the entry and its SSH client when the session is still ready. Anything else
// means the loss is old news - the session already failed, or was closed on purpose, which also
// ends its terminal output - and must not be reported a second time.
func (s *SessionLifecycleService) readySession(sessionID string) (*sessionEntry, domain.SSHClient, bool) {
	var (
		entry  *sessionEntry
		client domain.SSHClient
	)
	s.registry.View(sessionID, func(e *sessionEntry) {
		if e.info.State == domain.SessionReady {
			entry = e
			client = e.sshClient
		}
	})
	return entry, client, entry != nil
}

func (s *SessionLifecycleService) markConnectionLost(entry *sessionEntry) {
	slog.Warn("session connection lost", "component", "session", "sessionID", entry.info.SessionID)
	s.applyState(entry, domain.SessionError, connectionLostMessage, true)
}

// transportAlive reports whether the SSH connection answers a keepalive within the probe timeout.
// Every way of not answering - an error, the timeout, the session being closed meanwhile - counts
// as dead: calling a live link lost costs one reconnect, calling a dead one alive strands the user.
func (s *SessionLifecycleService) transportAlive(ctx context.Context, client domain.SSHClient) bool {
	timeout := s.transportProbeTimeout
	if timeout <= 0 {
		timeout = defaultTransportProbeTimeout
	}
	// Buffered so the probe goroutine can always deliver and exit, even after this function has
	// stopped listening. KeepAlive itself returns at the latest when CloseSession closes the client.
	result := make(chan error, 1)
	safego.GoNamed("session.transportProbe", func() { result <- client.KeepAlive() })

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err == nil
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}
