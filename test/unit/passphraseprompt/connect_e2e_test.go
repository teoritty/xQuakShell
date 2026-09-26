// Package passphraseprompt drives a real SSH connection through a passphrase-protected key, end
// to end: a real vault on disk, the real key codec and key manager, the real known_hosts check and
// dialer, and an in-process SSH server that accepts only that key.
//
// Only the dialog itself is played by the test. It stands where the frontend stands - it receives
// the prompt the backend raises and answers through the same Resolve/Cancel the RPCs call - so
// what is under test is everything between "the key needs a passphrase" and "the server let us
// in". That path had no test at all, which is how a prompt that could only ever fail went unseen.
package passphraseprompt

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/keys"
	"xquakshell/internal/infra/persistence"
	infrassh "xquakshell/internal/infra/ssh"
	"xquakshell/internal/pkg/conlimit"
	"xquakshell/internal/usecase"
)

const (
	keyPassphrase = "hunter2-correct-horse"
	keyLabel      = "prod deploy key"
	waitTimeout   = 15 * time.Second
)

// --- the SSH server ---

// keyOnlyServer accepts exactly one public key and counts how many handshakes it let through, so a
// test can tell "connected" apart from "the client gave up before authenticating".
//
// The count is taken on the server's goroutine after NewServerConn returns, which is after the
// client has already been told it is in. A test that reads it the moment the session reports ready
// races that goroutine, so a test expecting a handshake waits on authed first.
type keyOnlyServer struct {
	addr          *net.TCPAddr
	hostKey       gossh.PublicKey
	authenticated atomic.Int32
	authed        chan struct{}
}

func startKeyOnlyServer(t *testing.T, authorized gossh.PublicKey) *keyOnlyServer {
	t.Helper()
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := gossh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}

	srv := &keyOnlyServer{hostKey: hostSigner.PublicKey(), authed: make(chan struct{}, 8)}
	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), authorized.Marshal()) {
				return nil, errors.New("key not authorized")
			}
			return &gossh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv.addr = ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(c, cfg)
		}
	}()
	return srv
}

func (s *keyOnlyServer) serve(c net.Conn, cfg *gossh.ServerConfig) {
	sc, chans, reqs, err := gossh.NewServerConn(c, cfg)
	if err != nil {
		_ = c.Close()
		return
	}
	s.authenticated.Add(1)
	select {
	case s.authed <- struct{}{}:
	default:
	}
	go gossh.DiscardRequests(reqs)
	for ch := range chans {
		_ = ch.Reject(gossh.Prohibited, "no channels in this test")
	}
	_ = sc.Close()
}

// awaitAuthenticated waits until the server has counted a handshake it let through.
func (s *keyOnlyServer) awaitAuthenticated(t *testing.T) {
	t.Helper()
	select {
	case <-s.authed:
	case <-time.After(waitTimeout):
		t.Fatal("the session reported ready but the server never counted an authenticated handshake")
	}
}

// --- the stand-in for the frontend dialog ---

// dialog receives what the backend would emit as PassphraseRequired / PassphrasePromptClosed.
type dialog struct {
	shown     chan usecase.PassphrasePrompt
	dismissed chan string
}

func newDialog() *dialog {
	return &dialog{shown: make(chan usecase.PassphrasePrompt, 4), dismissed: make(chan string, 4)}
}

func (d *dialog) ShowPassphrasePrompt(p usecase.PassphrasePrompt) { d.shown <- p }
func (d *dialog) DismissPassphrasePrompt(id string)               { d.dismissed <- id }

func (d *dialog) awaitPrompt(t *testing.T) usecase.PassphrasePrompt {
	t.Helper()
	select {
	case p := <-d.shown:
		return p
	case <-time.After(waitTimeout):
		t.Fatal("no passphrase prompt reached the UI; the connection failed or hung instead of asking")
		return usecase.PassphrasePrompt{}
	}
}

func (d *dialog) awaitDismissed(t *testing.T, requestID string) {
	t.Helper()
	select {
	case got := <-d.dismissed:
		if got != requestID {
			t.Fatalf("dismissed %q, want %q", got, requestID)
		}
	case <-time.After(waitTimeout):
		t.Fatal("the prompt was never dismissed; the dialog would stay on screen after its connection ended")
	}
}

// --- the application, composed as main_compose.go composes it ---

type fixture struct {
	sessions  *usecase.SessionManager
	prompts   *usecase.PassphrasePrompts
	dialog    *dialog
	server    *keyOnlyServer
	states    chan domain.ConnectionSession
	connID    string
	identity  *domain.SSHIdentity
	knownHost string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()

	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "e2e master password, long enough"); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	t.Cleanup(vaultRepo.Lock)
	codec := keys.NewCodec()
	identRepo := persistence.NewIdentityRepo(vaultRepo, codec, keys.NewDataKey)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	knownHosts := persistence.NewKnownHostsRepo(vaultRepo)
	cache := infrassh.NewPassphraseCache()
	keyManager := usecase.NewKeyManagerService(usecase.KeyManagerConfig{
		Identities: identRepo,
		Vault:      vaultRepo,
		Codec:      codec,
		Cache:      cache,
		NewDataKey: keys.NewDataKey,
	})

	// CacheNever, so every connection has to ask: a cached passphrase would let a later test in
	// this file connect without the prompt it is supposed to exercise.
	identity, err := keyManager.Generate(ctx,
		domain.GeneratedKeySpec{Algorithm: domain.AlgorithmEd25519, Comment: keyLabel},
		keyPassphrase, usecase.KeyOptions{Cache: domain.CacheNever})
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if !identity.Encrypted {
		t.Fatal("generated key is not passphrase-protected; the test would not reach the prompt")
	}
	authorized, _, _, _, err := gossh.ParseAuthorizedKey([]byte(identity.PublicKey))
	if err != nil {
		t.Fatalf("parse identity public key: %v", err)
	}

	f := &fixture{
		prompts:  &usecase.PassphrasePrompts{},
		dialog:   newDialog(),
		server:   startKeyOnlyServer(t, authorized),
		states:   make(chan domain.ConnectionSession, 32),
		connID:   "conn-e2e",
		identity: identity,
	}
	f.knownHost = fmt.Sprintf("[127.0.0.1]:%d", f.server.addr.Port)
	if err := knownHosts.Add(ctx, f.knownHost, f.server.hostKey); err != nil {
		t.Fatalf("trust server host key: %v", err)
	}
	if err := connRepo.Save(ctx, keyConnection(f.connID, f.server.addr.Port, identity.ID)); err != nil {
		t.Fatalf("save connection: %v", err)
	}

	f.sessions = usecase.NewSessionManager(usecase.SessionManagerConfig{
		ConnRepo:                  connRepo,
		VaultRepo:                 vaultRepo,
		IdentRepo:                 identRepo,
		PasswordRepo:              persistence.NewPasswordRepo(vaultRepo),
		KnownHosts:                knownHosts,
		SSHFactory:                infrassh.NewDialer(),
		PassphraseCache:           cache,
		HostKeyCallbackBuilder:    infrassh.NewHostKeyCallbackBuilder(),
		JumpTransportBuilder:      infrassh.NewJumpTransportBuilder(),
		Keys:                      keyManager,
		ForwardConnLimiterFactory: func() domain.ConcurrencyLimiter { return conlimit.New(4) },
		OnStateChange:             func(s domain.ConnectionSession) { f.states <- s },
		// The same shape AppAPI.onPassphraseRequest has, with the dialog in place of the Wails
		// event emitter.
		PassphraseReq: func(ctx context.Context, question usecase.PassphraseQuestion) (string, error) {
			return f.prompts.Ask(ctx, question, f.dialog)
		},
	})
	t.Cleanup(f.sessions.CloseAll)
	return f
}

func keyConnection(id string, port int, identityID string) *domain.Connection {
	return &domain.Connection{
		ID:       id,
		Name:     "e2e",
		Host:     "127.0.0.1",
		Port:     port,
		Protocol: domain.ProtocolSSH,
		Users: []domain.ConnectionUser{{
			ID:       "u1",
			Username: "deploy",
			Auth:     domain.AuthMethodKey,
			KeyAuth:  &domain.KeyAuthConfig{IdentityIDs: []string{identityID}},
		}},
		DefaultUserID: "u1",
	}
}

// awaitState reads state changes until the session settles in ready or error, and fails unless it
// is the one wanted.
func (f *fixture) awaitState(t *testing.T, sessionID string, want domain.SessionState) domain.ConnectionSession {
	t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case s := <-f.states:
			if s.SessionID != sessionID {
				continue
			}
			if s.State == want {
				return s
			}
			if s.State == domain.SessionReady || s.State == domain.SessionError {
				t.Fatalf("session settled in %q (%q), want %q", s.State, s.ErrorMessage, want)
			}
		case <-deadline:
			t.Fatalf("session never reached %q", want)
		}
	}
}

func (f *fixture) open(t *testing.T) string {
	t.Helper()
	sessionID, err := f.sessions.OpenSession(context.Background(), f.connID)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	return sessionID
}

// --- the scenarios ---

// The bug itself: a connection through an encrypted key must ask for the passphrase and, given the
// right one, get in. Before the fix the prompt was a message box saying it could not ask, and the
// connection failed with ErrPassphraseRequired every time.
func TestEncryptedKeyConnectsAfterTheUserTypesThePassphrase(t *testing.T) {
	f := newFixture(t)
	sessionID := f.open(t)

	prompt := f.dialog.awaitPrompt(t)
	if prompt.IdentityID != f.identity.ID || prompt.Label != keyLabel {
		t.Errorf("prompt = %+v, want identity %q labelled %q so the user knows which key is asked for", prompt, f.identity.ID, keyLabel)
	}
	if err := f.prompts.Resolve(prompt.RequestID, keyPassphrase); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	f.awaitState(t, sessionID, domain.SessionReady)
	f.dialog.awaitDismissed(t, prompt.RequestID)
	f.server.awaitAuthenticated(t)
	if n := f.server.authenticated.Load(); n != 1 {
		t.Errorf("server authenticated %d handshakes, want 1", n)
	}
}

// A mistyped passphrase asks again, says so, and the right one still gets in - without anything
// reaching the server with a key that did not open.
func TestWrongPassphraseAsksAgainAndTheRightOneConnects(t *testing.T) {
	f := newFixture(t)
	sessionID := f.open(t)

	first := f.dialog.awaitPrompt(t)
	if first.Retry {
		t.Error("the first prompt is marked as a retry; the user has not typed anything yet")
	}
	if err := f.prompts.Resolve(first.RequestID, "not-the-passphrase"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.dialog.awaitDismissed(t, first.RequestID)

	second := f.dialog.awaitPrompt(t)
	if !second.Retry {
		t.Error("the prompt after a wrong passphrase is not marked as a retry; the dialog cannot tell the user it was wrong")
	}
	if second.RequestID == first.RequestID {
		t.Error("the retry reuses the answered request id; an answer could then be replayed into it")
	}
	if n := f.server.authenticated.Load(); n != 0 {
		t.Errorf("server authenticated %d handshakes before the key was opened, want 0", n)
	}
	if err := f.prompts.Resolve(second.RequestID, keyPassphrase); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	f.awaitState(t, sessionID, domain.SessionReady)
	f.dialog.awaitDismissed(t, second.RequestID)
	f.server.awaitAuthenticated(t)
	if n := f.server.authenticated.Load(); n != 1 {
		t.Errorf("server authenticated %d handshakes, want 1", n)
	}
}

// Attempts are bounded: after the last wrong passphrase the connection fails, names the reason,
// and does not put up another dialog.
func TestRepeatedWrongPassphrasesFailTheConnection(t *testing.T) {
	f := newFixture(t)
	sessionID := f.open(t)

	for attempt := 1; attempt <= 3; attempt++ {
		prompt := f.dialog.awaitPrompt(t)
		if err := f.prompts.Resolve(prompt.RequestID, fmt.Sprintf("guess-%d", attempt)); err != nil {
			t.Fatalf("Resolve attempt %d: %v", attempt, err)
		}
		f.dialog.awaitDismissed(t, prompt.RequestID)
	}

	s := f.awaitState(t, sessionID, domain.SessionError)
	if s.ErrorMessage != "Wrong key passphrase" {
		t.Errorf("error message = %q, want %q; a generic authentication failure points at the server", s.ErrorMessage, "Wrong key passphrase")
	}
	select {
	case p := <-f.dialog.shown:
		t.Errorf("a fourth prompt %q was shown; the attempts must be bounded", p.RequestID)
	default:
	}
	if n := f.server.authenticated.Load(); n != 0 {
		t.Errorf("server authenticated %d handshakes after wrong passphrases, want 0", n)
	}
}

// Cancelling the dialog ends the connection instead of leaving it in "connecting", and says the
// passphrase was not entered rather than that the server refused.
func TestCancelledPromptFailsTheConnection(t *testing.T) {
	f := newFixture(t)
	sessionID := f.open(t)

	prompt := f.dialog.awaitPrompt(t)
	if err := f.prompts.Cancel(prompt.RequestID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	s := f.awaitState(t, sessionID, domain.SessionError)
	if s.ErrorMessage != "Key passphrase was not entered" {
		t.Errorf("error message = %q, want %q", s.ErrorMessage, "Key passphrase was not entered")
	}
	f.dialog.awaitDismissed(t, prompt.RequestID)
	if n := f.server.authenticated.Load(); n != 0 {
		t.Errorf("server authenticated %d handshakes after a cancel, want 0", n)
	}
}

// Closing the tab while the dialog is open must take the dialog down and make a late answer
// harmless: the passphrase must not be delivered to a connection nobody is waiting for.
func TestClosingTheSessionWithdrawsThePrompt(t *testing.T) {
	f := newFixture(t)
	sessionID := f.open(t)

	prompt := f.dialog.awaitPrompt(t)
	if err := f.sessions.CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	f.dialog.awaitDismissed(t, prompt.RequestID)
	if err := f.prompts.Resolve(prompt.RequestID, keyPassphrase); !errors.Is(err, domain.ErrNoPendingPassphrasePrompt) {
		t.Errorf("Resolve after close = %v, want ErrNoPendingPassphrasePrompt", err)
	}
	if n := f.server.authenticated.Load(); n != 0 {
		t.Errorf("server authenticated %d handshakes for a closed session, want 0", n)
	}
}
