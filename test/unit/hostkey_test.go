package unit

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/persistence"
)

// mockVaultForKH is a minimal in-memory VaultRepository for testing known_hosts.
type mockVaultForKH struct {
	data *domain.VaultData
}

func (m *mockVaultForKH) Unlock(_ context.Context, _ string) error { return nil }
func (m *mockVaultForKH) Lock()                                    {}
func (m *mockVaultForKH) IsUnlocked() bool                         { return true }
func (m *mockVaultForKH) GetData() (*domain.VaultData, error)      { return m.data, nil }
func (m *mockVaultForKH) SaveData(_ context.Context, data *domain.VaultData) error {
	m.data = data
	return nil
}

func newMockVaultKH(knownHostsLines []string) *mockVaultForKH {
	d := domain.NewVaultData()
	d.KnownHosts = knownHostsLines
	return &mockVaultForKH{data: d}
}

func generateTestKey(t *testing.T) (gossh.PublicKey, gossh.Signer) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer.PublicKey(), signer
}

func generateTestKeyECDSA(t *testing.T) (gossh.PublicKey, gossh.Signer) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer.PublicKey(), signer
}

func formatKnownHostLine(host string, key gossh.PublicKey) string {
	return fmt.Sprintf("%s %s", host, strings.TrimSpace(string(gossh.MarshalAuthorizedKey(key))))
}

func TestHostKeyCheckUnknownHost(t *testing.T) {
	mv := newMockVaultKH([]string{})
	repo := persistence.NewKnownHostsRepo(mv)

	pubKey, _ := generateTestKey(t)

	err := repo.Check("example.com", pubKey)
	if err == nil {
		t.Fatal("expected ErrUnknownHost, got nil")
	}
	if !errors.Is(err, domain.ErrUnknownHost) {
		t.Errorf("expected ErrUnknownHost, got: %v", err)
	}
}

func TestHostKeyCheckMatchingKey(t *testing.T) {
	pubKey, _ := generateTestKey(t)
	line := formatKnownHostLine("example.com", pubKey)

	mv := newMockVaultKH([]string{line})
	repo := persistence.NewKnownHostsRepo(mv)

	err := repo.Check("example.com", pubKey)
	if err != nil {
		t.Fatalf("expected nil for matching key, got: %v", err)
	}
}

func TestHostKeyCheckMismatch(t *testing.T) {
	pubKey1, _ := generateTestKey(t)
	pubKey2, _ := generateTestKeyECDSA(t)

	line := formatKnownHostLine("example.com", pubKey1)
	mv := newMockVaultKH([]string{line})
	repo := persistence.NewKnownHostsRepo(mv)

	err := repo.Check("example.com", pubKey2)
	if err == nil {
		t.Fatal("expected ErrHostKeyMismatch, got nil")
	}
	if !errors.Is(err, domain.ErrHostKeyMismatch) {
		t.Errorf("expected ErrHostKeyMismatch, got: %v", err)
	}
}

func TestHostKeyAddAndCheckSuccess(t *testing.T) {
	mv := newMockVaultKH([]string{})
	repo := persistence.NewKnownHostsRepo(mv)

	pubKey, _ := generateTestKey(t)

	err := repo.Check("newhost.example.com", pubKey)
	if !errors.Is(err, domain.ErrUnknownHost) {
		t.Fatalf("expected ErrUnknownHost before add, got: %v", err)
	}

	err = repo.Add(context.Background(), "newhost.example.com", pubKey)
	if err != nil {
		t.Fatalf("add known host: %v", err)
	}

	err = repo.Check("newhost.example.com", pubKey)
	if err != nil {
		t.Fatalf("expected nil after add, got: %v", err)
	}
}

func TestHostKeyRemove(t *testing.T) {
	pubKey, _ := generateTestKey(t)
	line := formatKnownHostLine("removeme.example.com", pubKey)

	mv := newMockVaultKH([]string{line})
	repo := persistence.NewKnownHostsRepo(mv)

	err := repo.Check("removeme.example.com", pubKey)
	if err != nil {
		t.Fatalf("expected match before remove, got: %v", err)
	}

	err = repo.Remove(context.Background(), "removeme.example.com")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	err = repo.Check("removeme.example.com", pubKey)
	if !errors.Is(err, domain.ErrUnknownHost) {
		t.Fatalf("expected ErrUnknownHost after remove, got: %v", err)
	}
}

func TestHostKeyList(t *testing.T) {
	pubKey, _ := generateTestKey(t)
	line := formatKnownHostLine("listtest.example.com", pubKey)

	mv := newMockVaultKH([]string{line})
	repo := persistence.NewKnownHostsRepo(mv)

	entries, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Host != "listtest.example.com" {
		t.Errorf("expected host 'listtest.example.com', got '%s'", entries[0].Host)
	}
	if entries[0].Fingerprint == "" {
		t.Error("fingerprint should not be empty")
	}
}
