package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

type mockKnownHostsRepo struct {
	addErr     error
	replaceErr error
	addCalled  bool
	lastHost   string
	lastKey    gossh.PublicKey
}

func (m *mockKnownHostsRepo) Check(string, gossh.PublicKey) error { return nil }
func (m *mockKnownHostsRepo) List() ([]domain.KnownHostEntry, error) {
	return nil, nil
}
func (m *mockKnownHostsRepo) Remove(context.Context, string) error { return nil }
func (m *mockKnownHostsRepo) Add(_ context.Context, host string, key gossh.PublicKey) error {
	m.addCalled = true
	m.lastHost = host
	m.lastKey = key
	return m.addErr
}
func (m *mockKnownHostsRepo) Replace(_ context.Context, host string, key gossh.PublicKey) error {
	m.lastHost = host
	m.lastKey = key
	return m.replaceErr
}

func testAuthorizedKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(gossh.MarshalAuthorizedKey(signer.PublicKey()))
}

// fakeHostKeySessions stands in for the session manager so a test can put a session into the
// "waiting on a host key" state, which is the only state ResolveHostKey acts on.
type fakeHostKeySessions struct {
	pending    *domain.HostKeyInfo
	lookupErr  error
	retried    string
	retryErr   error
	retryCalls int
}

func (f *fakeHostKeySessions) GetHostKeyInfo(string) (*domain.HostKeyInfo, error) {
	return f.pending, f.lookupErr
}

func (f *fakeHostKeySessions) RetrySession(_ context.Context, sessionID string) error {
	f.retryCalls++
	f.retried = sessionID
	return f.retryErr
}

func pendingHostKey(t *testing.T, host string) *domain.HostKeyInfo {
	t.Helper()
	return &domain.HostKeyInfo{Host: host, KeyBase64: testAuthorizedKey(t)}
}

func TestHostKeyService_ResolveHostKeyUnknownAction(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	svc := NewHostKeyService(repo, &fakeHostKeySessions{pending: pendingHostKey(t, "example.com")})

	if err := svc.ResolveHostKey(context.Background(), "s1", "delete"); err == nil {
		t.Fatal("expected an error for an action that is neither add nor replace")
	}
	if repo.addCalled {
		t.Error("an unknown action still wrote trust")
	}
}

// The key that gets trusted is the one the handshake produced, not one the caller names. Anything
// holding window.go used to be able to pass its own key here for any host.
func TestHostKeyService_ResolveHostKeyTrustsThePendingKey(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	sessions := &fakeHostKeySessions{pending: pendingHostKey(t, "example.com:2222")}
	svc := NewHostKeyService(repo, sessions)

	if err := svc.ResolveHostKey(context.Background(), "s1", "add"); err != nil {
		t.Fatalf("ResolveHostKey err = %v, want nil", err)
	}
	if !repo.addCalled {
		t.Fatal("the pending key was not added")
	}
	if repo.lastHost != "example.com:2222" {
		t.Errorf("added host %q, want the pending session's host", repo.lastHost)
	}
	if sessions.retried != "s1" {
		t.Errorf("retried session %q, want s1", sessions.retried)
	}
}

// A session with nothing pending must not be a way to write trust: without a key from a real
// handshake there is nothing for the user's decision to be about.
func TestHostKeyService_ResolveHostKeyRefusesASessionWithNothingPending(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	sessions := &fakeHostKeySessions{pending: nil}
	svc := NewHostKeyService(repo, sessions)

	err := svc.ResolveHostKey(context.Background(), "s1", "add")

	if !errors.Is(err, domain.ErrNoPendingHostKey) {
		t.Fatalf("ResolveHostKey err = %v, want ErrNoPendingHostKey", err)
	}
	if repo.addCalled {
		t.Error("trust was written for a session that was not waiting on a host key")
	}
	if sessions.retryCalls != 0 {
		t.Error("a refused decision still retried the session")
	}
}

// An unknown session is refused before anything is written. This used to write the key first and
// only then discover the session was gone.
func TestHostKeyService_ResolveHostKeyRefusesAnUnknownSession(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	sessions := &fakeHostKeySessions{lookupErr: domain.ErrSessionNotFound}
	svc := NewHostKeyService(repo, sessions)

	if err := svc.ResolveHostKey(context.Background(), "missing", "add"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("ResolveHostKey err = %v, want ErrSessionNotFound", err)
	}
	if repo.addCalled {
		t.Error("trust was written for a session that does not exist")
	}
}

func TestHostKeyService_ResolveHostKeyReplaceUsesThePendingKey(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	sessions := &fakeHostKeySessions{pending: pendingHostKey(t, "example.com")}
	svc := NewHostKeyService(repo, sessions)

	if err := svc.ResolveHostKey(context.Background(), "s1", "replace"); err != nil {
		t.Fatalf("ResolveHostKey err = %v, want nil", err)
	}
	if repo.lastHost != "example.com" || repo.lastKey == nil {
		t.Errorf("replace got host %q key %v, want the pending pair", repo.lastHost, repo.lastKey)
	}
}

func TestHostKeyService_VerifyDelegates(t *testing.T) {
	called := false
	repo := &mockKnownHostsRepo{}
	repoCheck := &verifyRepo{mockKnownHostsRepo: repo, onCheck: func() { called = true }}
	svc := NewHostKeyService(repoCheck, nil)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(priv)
	_ = svc.Verify("host", signer.PublicKey())
	if !called {
		t.Fatal("expected Check to be called")
	}
}

type verifyRepo struct {
	*mockKnownHostsRepo
	onCheck func()
}

func (v *verifyRepo) Check(host string, key gossh.PublicKey) error {
	if v.onCheck != nil {
		v.onCheck()
	}
	return errors.New("check")
}
