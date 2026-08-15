package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"

	"xquakshell/internal/domain"
)

// stubConnRepo is declared in ping_manager_test.go - same package, same stub.
func trustLifecycle(t *testing.T, conns ...domain.Connection) (*SessionLifecycleService, *SessionRegistry) {
	t.Helper()
	registry := NewSessionRegistry()
	svc := NewSessionLifecycleService(SessionLifecycleConfig{
		Registry: registry,
		ConnRepo: &stubConnRepo{conns: conns},
	})
	return svc, registry
}

func putTrustSession(registry *SessionRegistry, sessionID, connectionID, pluginID string, state domain.SessionState) {
	ctx, cancel := context.WithCancel(context.Background())
	entry := newSessionEntry(domain.ConnectionSession{
		SessionID: sessionID,
		State:     state,
	}, ctx, cancel, connectionID)
	entry.pluginID = pluginID
	registry.Put(sessionID, entry)
}

func trustPrompt() *PeerTrustPrompt {
	return &PeerTrustPrompt{Subject: "10.0.0.5:3389", Fingerprint: "SHA256:x", Material: []byte{1, 2}}
}

// The pending decision and the session state must change together. Apart, they produce a session
// that waits for an answer while looking like it is connecting - the user watches an endless
// "connecting" instead of the question.
func TestBeginPeerTrustPromptMovesSessionIntoTrustRequired(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}
	entry, ok := registry.Get("s1")
	if !ok {
		t.Fatal("the session disappeared")
	}
	if entry.info.State != domain.SessionTrustRequired {
		t.Fatalf("state = %q, want trust-required", entry.info.State)
	}
	got, err := svc.GetPeerTrustPrompt("s1")
	if err != nil {
		t.Fatalf("GetPeerTrustPrompt: %v", err)
	}
	if got == nil || got.Subject != "10.0.0.5:3389" {
		t.Fatalf("the pending decision was not stored: %+v", got)
	}
}

// A changed material and a first meeting must produce different words: identical wording teaches
// the user to walk past the second the same way they walk past the first.
func TestBeginPeerTrustPromptDistinguishesMismatch(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}
	entry, _ := registry.Get("s1")
	firstMsg := entry.info.ErrorMessage

	// A second question needs the first one out of the way: replacing a live one is exactly what
	// TestBeginPeerTrustPromptRefusesToReplaceALiveQuestion pins down.
	if err := svc.RejectPeerTrust("s1"); err != nil {
		t.Fatalf("RejectPeerTrust: %v", err)
	}
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	changed := trustPrompt()
	changed.Mismatch = true
	if err := svc.BeginPeerTrustPrompt("s1", changed); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}
	entry, _ = registry.Get("s1")
	if entry.info.ErrorMessage == firstMsg {
		t.Fatalf("a changed material is described in the same words as a first meeting: %q", firstMsg)
	}
}

// THE consent-integrity property. A prompt that could be replaced would let a plugin show a
// harmless fingerprint, wait for the dialog to appear, and swap the material underneath it: the
// user clicks "trust" on one value and stores another.
func TestBeginPeerTrustPromptRefusesToReplaceALiveQuestion(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	first := trustPrompt()
	if err := svc.BeginPeerTrustPrompt("s1", first); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}
	second := trustPrompt()
	second.Material = []byte{9, 9}
	second.Fingerprint = "SHA256:other"

	if err := svc.BeginPeerTrustPrompt("s1", second); !errors.Is(err, domain.ErrPeerTrustPromptBusy) {
		t.Fatalf("err = %v, want ErrPeerTrustPromptBusy", err)
	}
	got, err := svc.GetPeerTrustPrompt("s1")
	if err != nil {
		t.Fatalf("GetPeerTrustPrompt: %v", err)
	}
	if got == nil || got.Fingerprint != first.Fingerprint {
		t.Fatalf("the pending question was replaced: %+v", got)
	}
}

// The same material asked again is a retried handshake, not an attack. Refusing it would break
// plugins that reconnect, and the question on screen is unchanged either way.
func TestBeginPeerTrustPromptIsIdempotentForTheSameMaterial(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}
	if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); err != nil {
		t.Fatalf("the same question asked twice was refused: %v", err)
	}
	entry, _ := registry.Get("s1")
	if entry.info.State != domain.SessionTrustRequired {
		t.Fatalf("state = %q after a repeat, want trust-required", entry.info.State)
	}
}

// Only a connecting session can be stopped by a trust question. Without this a plugin could drag
// its own established session back into trust-required whenever it liked.
func TestBeginPeerTrustPromptRefusesEstablishedSessions(t *testing.T) {
	for _, state := range []domain.SessionState{domain.SessionReady, domain.SessionError, domain.SessionClosed} {
		t.Run(string(state), func(t *testing.T) {
			svc, registry := trustLifecycle(t)
			putTrustSession(registry, "s1", "c1", "plug", state)

			if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); !errors.Is(err, domain.ErrPeerTrustNotWaiting) {
				t.Fatalf("err = %v, want ErrPeerTrustNotWaiting", err)
			}
			entry, _ := registry.Get("s1")
			if entry.info.State != state {
				t.Fatalf("state = %q, want it untouched at %q", entry.info.State, state)
			}
			if entry.peerTrustPrompt != nil {
				t.Fatal("a question was attached to a session that cannot answer one")
			}
		})
	}
}

// A refusal must fail the session, not merely close the dialog. Left in trust-required it would
// show a question nobody can answer and still accept a retry.
func TestRejectPeerTrustFailsTheSession(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)
	if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}

	if err := svc.RejectPeerTrust("s1"); err != nil {
		t.Fatalf("RejectPeerTrust: %v", err)
	}
	entry, _ := registry.Get("s1")
	if entry.peerTrustPrompt != nil {
		t.Fatal("the pending question survived the refusal")
	}
	if entry.info.State != domain.SessionError {
		t.Fatalf("state = %q, want error", entry.info.State)
	}
	if entry.info.ErrorMessage == "" {
		t.Fatal("the session failed with no reason a user could read")
	}
	if err := svc.RetrySession(context.Background(), "s1"); err == nil {
		t.Fatal("a refused session still accepts a retry")
	}
}

func TestPeerTrustPromptOnUnknownSessionIsRefused(t *testing.T) {
	svc, _ := trustLifecycle(t)
	if err := svc.BeginPeerTrustPrompt("no such", trustPrompt()); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
	if _, err := svc.GetPeerTrustPrompt("no such"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
	if err := svc.RejectPeerTrust("no such"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

// The reader must not hand out the live field. A caller holding that pointer would read it long
// after the lock was released, and every such read races the plugin-driven writer.
func TestGetPeerTrustPromptHandsOutACopy(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)
	if err := svc.BeginPeerTrustPrompt("s1", trustPrompt()); err != nil {
		t.Fatalf("BeginPeerTrustPrompt: %v", err)
	}

	got, err := svc.GetPeerTrustPrompt("s1")
	if err != nil {
		t.Fatalf("GetPeerTrustPrompt: %v", err)
	}
	got.Subject = "tampered"
	got.Material[0] = 0xFF

	entry, _ := registry.Get("s1")
	if entry.peerTrustPrompt.Subject != "10.0.0.5:3389" {
		t.Fatal("editing the returned value changed what the session is asking about")
	}
	if entry.peerTrustPrompt.Material[0] != 1 {
		t.Fatal("editing the returned material changed what would be stored")
	}
}

// Under -race this is the test that fails if the reader ever goes back to reading the field
// through Get. Without -race it still exercises both paths concurrently.
func TestPeerTrustPromptSurvivesConcurrentReadsAndWrites(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = svc.BeginPeerTrustPrompt("s1", trustPrompt())
			_ = svc.RejectPeerTrust("s1")
			registry.Mutate("s1", func(e *sessionEntry) { e.info.State = domain.SessionConnecting })
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if _, err := svc.GetPeerTrustPrompt("s1"); err != nil {
				t.Errorf("GetPeerTrustPrompt: %v", err)
				return
			}
		}
	}()
	wg.Wait()
}

// A trust subject is derived from here, and only from here.
func TestConnectionRecordForSessionReturnsTheSessionsConnection(t *testing.T) {
	conn := domain.Connection{ID: "c1", Name: "prod", Host: "10.0.0.5", Port: 3389, Protocol: "rdp"}
	svc, registry := trustLifecycle(t, conn)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionConnecting)

	got, err := svc.ConnectionRecordForSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ConnectionRecordForSession: %v", err)
	}
	if got.Host != "10.0.0.5" || got.Port != 3389 {
		t.Fatalf("connection record = %+v", got)
	}
	if _, err := svc.ConnectionRecordForSession(context.Background(), "no such"); err == nil {
		t.Fatal("a record was handed out for a session that does not exist")
	}
}

// A retry out of a trust decision must work and must clear BOTH forms of waiting: reconnecting, a
// session must not drag someone else's unanswered decision along.
func TestRetrySessionFromTrustRequiredClearsBothPendings(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c-missing", "plug", domain.SessionTrustRequired)
	registry.Mutate("s1", func(e *sessionEntry) {
		e.peerTrustPrompt = trustPrompt()
		e.hostKeyInfo = &domain.HostKeyInfo{Host: "h"}
	})

	// The stub has no such connection: the transition happens, and the connect that follows
	// runs into a missing record. The transition is what this test is about - it is what changed.
	_ = svc.RetrySession(context.Background(), "s1")

	entry, ok := registry.Get("s1")
	if !ok {
		t.Fatal("the session disappeared")
	}
	if entry.peerTrustPrompt != nil {
		t.Fatal("the trust decision was not cleared by the retry")
	}
	if entry.hostKeyInfo != nil {
		t.Fatal("the host key decision was not cleared by the retry")
	}
	if entry.info.State == domain.SessionTrustRequired {
		t.Fatal("the session stayed in trust-required after a retry")
	}
}

// REGRESSION: the SSH path must keep working exactly as before. That is what the second
// transition was added for, rather than replacing the first.
func TestRetrySessionStillWorksFromHostKeyRequired(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c-missing", "", domain.SessionHostKeyRequired)
	registry.Mutate("s1", func(e *sessionEntry) {
		e.hostKeyInfo = &domain.HostKeyInfo{Host: "h"}
	})

	_ = svc.RetrySession(context.Background(), "s1")

	entry, _ := registry.Get("s1")
	if entry.hostKeyInfo != nil {
		t.Fatal("the host key decision was not cleared - the SSH path is broken")
	}
	if entry.info.State == domain.SessionHostKeyRequired {
		t.Fatal("the session stayed in hostkey-required - the transition did not happen")
	}
}

// A retry from an arbitrary state is refused: the meaning of waiting is that the connection does
// not proceed until the decision is made.
func TestRetrySessionRefusesFromOtherStates(t *testing.T) {
	svc, registry := trustLifecycle(t)
	putTrustSession(registry, "s1", "c1", "plug", domain.SessionReady)

	if err := svc.RetrySession(context.Background(), "s1"); err == nil {
		t.Fatal("a retry from ready was accepted")
	}
	if err := svc.RetrySession(context.Background(), "no such"); err == nil {
		t.Fatal("a retry of a session that does not exist was accepted")
	}
}
