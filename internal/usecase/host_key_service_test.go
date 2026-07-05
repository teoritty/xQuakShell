package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
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

func TestHostKeyService_AddInvalidKey(t *testing.T) {
	svc := NewHostKeyService(&mockKnownHostsRepo{}, nil)
	err := svc.Add(context.Background(), "host", "not-a-key")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestHostKeyService_Add(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	svc := NewHostKeyService(repo, nil)
	keyLine := testAuthorizedKey(t)
	if err := svc.Add(context.Background(), "example.com", keyLine); err != nil {
		t.Fatal(err)
	}
	if !repo.addCalled || repo.lastHost != "example.com" {
		t.Fatalf("add not called correctly: %+v", repo)
	}
}

func TestHostKeyService_ResolveHostKeyUnknownAction(t *testing.T) {
	svc := NewHostKeyService(&mockKnownHostsRepo{}, nil)
	err := svc.ResolveHostKey(context.Background(), "s1", "delete", "host", testAuthorizedKey(t))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHostKeyService_ResolveHostKeyAdd(t *testing.T) {
	repo := &mockKnownHostsRepo{}
	mgr := &SessionManager{}
	svc := NewHostKeyService(repo, mgr)
	keyLine := testAuthorizedKey(t)
	err := svc.ResolveHostKey(context.Background(), "missing-session", "add", "host", keyLine)
	if err == nil {
		t.Fatal("expected retry error for missing session")
	}
	if !repo.addCalled {
		t.Fatal("expected add to be called")
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
