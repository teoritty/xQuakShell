package usecase

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

// probeSSHClient answers KeepAlive with whatever the test configured. A nil block channel answers
// at once; a non-nil one holds the probe until the channel closes, standing in for a link that has
// stopped answering without failing.
type probeSSHClient struct {
	keepAliveErr error
	block        chan struct{}
}

func (c *probeSSHClient) OpenDirectTCP(context.Context, string) (net.Conn, error) { return nil, nil }
func (c *probeSSHClient) ListenTCP(context.Context, string) (net.Listener, error) { return nil, nil }
func (c *probeSSHClient) NewSession() (*gossh.Session, error)                     { return nil, nil }
func (c *probeSSHClient) Client() *gossh.Client                                   { return nil }
func (c *probeSSHClient) Close() error                                            { return nil }
func (c *probeSSHClient) KeepAlive() error {
	if c.block != nil {
		<-c.block
	}
	return c.keepAliveErr
}

var _ domain.SSHClient = (*probeSSHClient)(nil)

type stateLog struct {
	mu     sync.Mutex
	states []domain.ConnectionSession
}

func (l *stateLog) record(info domain.ConnectionSession) {
	l.mu.Lock()
	l.states = append(l.states, info)
	l.mu.Unlock()
}

func (l *stateLog) all() []domain.ConnectionSession {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]domain.ConnectionSession(nil), l.states...)
}

func lossTestLifecycle(t *testing.T) (*SessionLifecycleService, *SessionRegistry, *stateLog) {
	t.Helper()
	registry := NewSessionRegistry()
	log := &stateLog{}
	lifecycle := NewSessionLifecycleService(SessionLifecycleConfig{
		Registry:      registry,
		OnStateChange: log.record,
	})
	lifecycle.transportProbeTimeout = 20 * time.Millisecond
	return lifecycle, registry, log
}

func putSessionInState(registry *SessionRegistry, sessionID string, state domain.SessionState, client domain.SSHClient) {
	putTestSessionEntry(registry, sessionID, client)
	registry.Mutate(sessionID, func(e *sessionEntry) { e.info.State = state })
}

func sessionInfo(t *testing.T, lifecycle *SessionLifecycleService, sessionID string) domain.ConnectionSession {
	t.Helper()
	info, err := lifecycle.GetState(sessionID)
	if err != nil {
		t.Fatalf("GetState(%s): %v", sessionID, err)
	}
	return info
}

func TestClosedTerminalOverLiveTransportIsAShellExitNotALoss(t *testing.T) {
	lifecycle, registry, _ := lossTestLifecycle(t)
	putSessionInState(registry, "s1", domain.SessionReady, &probeSSHClient{})

	lifecycle.NotifySessionDisconnected("s1")

	info := sessionInfo(t, lifecycle, "s1")
	if info.State != domain.SessionError {
		t.Fatalf("state = %q, want error; the terminal is gone either way", info.State)
	}
	if info.ConnectionLost {
		t.Fatal("ConnectionLost = true for a shell that exited over a live transport; " +
			"the frontend would reopen a session the user just logged out of")
	}
	if info.ErrorMessage != shellExitedMessage {
		t.Errorf("message = %q, want %q", info.ErrorMessage, shellExitedMessage)
	}
}

func TestClosedTerminalOverDeadTransportIsALoss(t *testing.T) {
	lifecycle, registry, _ := lossTestLifecycle(t)
	putSessionInState(registry, "s1", domain.SessionReady, &probeSSHClient{keepAliveErr: errors.New("EOF")})

	lifecycle.NotifySessionDisconnected("s1")

	info := sessionInfo(t, lifecycle, "s1")
	if info.State != domain.SessionError || !info.ConnectionLost {
		t.Fatalf("state = %q, lost = %v; a transport that errors is a lost connection", info.State, info.ConnectionLost)
	}
	if info.ErrorMessage != connectionLostMessage {
		t.Errorf("message = %q, want %q", info.ErrorMessage, connectionLostMessage)
	}
}

func TestUnansweredProbeCountsAsALoss(t *testing.T) {
	lifecycle, registry, _ := lossTestLifecycle(t)
	client := &probeSSHClient{block: make(chan struct{})}
	defer close(client.block)
	putSessionInState(registry, "s1", domain.SessionReady, client)

	lifecycle.NotifySessionDisconnected("s1")

	if !sessionInfo(t, lifecycle, "s1").ConnectionLost {
		t.Fatal("a probe that never answers must count as lost; waiting on it would strand the tab")
	}
}

func TestKeepaliveFailureIsALossWithoutProbing(t *testing.T) {
	lifecycle, registry, _ := lossTestLifecycle(t)
	// A client that would answer the probe: NotifyConnectionLost must not ask it, because the
	// keepalive that just failed has already answered the question.
	putSessionInState(registry, "s1", domain.SessionReady, &probeSSHClient{})

	lifecycle.NotifyConnectionLost("s1")

	if !sessionInfo(t, lifecycle, "s1").ConnectionLost {
		t.Fatal("a failed keepalive must be reported as a lost connection")
	}
}

func TestLossIsReportedOnlyForAReadySession(t *testing.T) {
	for _, state := range []domain.SessionState{domain.SessionConnecting, domain.SessionError, domain.SessionHostKeyRequired} {
		lifecycle, registry, log := lossTestLifecycle(t)
		putSessionInState(registry, "s1", state, &probeSSHClient{keepAliveErr: errors.New("EOF")})

		lifecycle.NotifySessionDisconnected("s1")
		lifecycle.NotifyConnectionLost("s1")

		if got := sessionInfo(t, lifecycle, "s1"); got.State != state || got.ConnectionLost {
			t.Errorf("from %q: state = %q, lost = %v; only an established session can lose its connection",
				state, got.State, got.ConnectionLost)
		}
		if n := len(log.all()); n != 0 {
			t.Errorf("from %q: %d state notification(s), want none", state, n)
		}
	}
}

func TestLossOfAnUnknownSessionIsIgnored(t *testing.T) {
	lifecycle, _, log := lossTestLifecycle(t)
	lifecycle.NotifySessionDisconnected("gone")
	lifecycle.NotifyConnectionLost("gone")
	if n := len(log.all()); n != 0 {
		t.Fatalf("%d notification(s) for a closed session; a user-closed tab must stay closed", n)
	}
}

func TestLeavingTheErrorClearsConnectionLost(t *testing.T) {
	lifecycle, registry, log := lossTestLifecycle(t)
	putSessionInState(registry, "s1", domain.SessionReady, &probeSSHClient{})
	lifecycle.NotifyConnectionLost("s1")

	entry, _ := registry.Get("s1")
	lifecycle.updateState(entry, domain.SessionConnecting, "")

	if sessionInfo(t, lifecycle, "s1").ConnectionLost {
		t.Fatal("ConnectionLost outlived the error it described")
	}
	states := log.all()
	if len(states) != 2 || !states[0].ConnectionLost || states[1].ConnectionLost {
		t.Fatalf("notifications = %+v; want the loss flagged, then the next state unflagged", states)
	}
}

func TestPluginErrorOutOfReadyIsALossAndOutOfConnectingIsNot(t *testing.T) {
	cases := []struct {
		from domain.SessionState
		lost bool
	}{
		{domain.SessionReady, true},
		{domain.SessionConnecting, false},
	}
	for _, tc := range cases {
		registry := NewSessionRegistry()
		bridge := NewPluginSessionBridge(PluginSessionBridgeConfig{})
		bridge.WireSessionRuntime(PluginSessionRuntimeConfig{Registry: registry})
		if err := bridge.BindPluginSessionForTest("s1", "p1"); err != nil {
			t.Fatal(err)
		}
		registry.Mutate("s1", func(e *sessionEntry) { e.info.State = tc.from })

		if err := bridge.HandlePluginUpdateState("p1", "s1", string(domain.SessionError), "peer went away"); err != nil {
			t.Fatalf("from %q: %v", tc.from, err)
		}

		var got domain.ConnectionSession
		registry.View("s1", func(e *sessionEntry) { got = e.info })
		if got.State != domain.SessionError || got.ConnectionLost != tc.lost {
			t.Errorf("from %q: state = %q, lost = %v, want error with lost = %v",
				tc.from, got.State, got.ConnectionLost, tc.lost)
		}
	}
}
