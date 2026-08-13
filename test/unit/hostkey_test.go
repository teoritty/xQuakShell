package unit

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	// #nosec G505 -- see hashedKnownHostLine: the known_hosts hashed-host format is HMAC-SHA1.
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
)

// mockVaultForKH is a minimal in-memory VaultRepository for testing known_hosts.
type mockVaultForKH struct {
	data *domain.VaultData
}

func (m *mockVaultForKH) Exists() bool                                           { return true }
func (m *mockVaultForKH) Create(_ context.Context, _ string) error               { return nil }
func (m *mockVaultForKH) Unlock(_ context.Context, _ string) error               { return nil }
func (m *mockVaultForKH) VerifyMasterPassword(_ context.Context, _ string) error { return nil }
func (m *mockVaultForKH) Lock()                                                  {}
func (m *mockVaultForKH) IsUnlocked() bool                                       { return true }
func (m *mockVaultForKH) GetData() (*domain.VaultData, error) {
	return domain.CloneVaultData(m.data), nil
}
func (m *mockVaultForKH) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	return mutate(m.data)
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

// hashedKnownHostLine writes the entry OpenSSH produces with HashKnownHosts on:
// |1|<base64 salt>|<base64 HMAC-SHA1(salt, host)> <key>
func hashedKnownHostLine(t *testing.T, host string, key gossh.PublicKey) string {
	t.Helper()
	salt := make([]byte, sha1.Size)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("salt: %v", err)
	}
	// #nosec G401 -- the known_hosts hashed-host format is defined as HMAC-SHA1; this test writes
	// the format OpenSSH writes so the repository can be shown to read it.
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(host))
	return fmt.Sprintf("|1|%s|%s %s",
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		strings.TrimSpace(string(gossh.MarshalAuthorizedKey(key))))
}

// OpenSSH keeps several keys for one host - an Ed25519 and an ECDSA one - and the server picks
// which to present. Check used to return on the FIRST line whose host matched, so if the
// negotiated key was recorded second the user got "host key mismatch, you may be under attack" on
// a perfectly good connection. The false alarm is not the damage: the damage is that it teaches
// the user to click through that banner, and clicking through runs Replace.
func TestHostKeyCheckAcceptsAnyRecordedKeyForTheHost(t *testing.T) {
	first, _ := generateTestKeyECDSA(t)
	second, _ := generateTestKey(t)
	mv := newMockVaultKH([]string{
		formatKnownHostLine("example.com", first),
		formatKnownHostLine("example.com", second),
	})
	repo := persistence.NewKnownHostsRepo(mv)

	if err := repo.Check("example.com", second); err != nil {
		t.Fatalf("Check with the second recorded key = %v, want nil", err)
	}
	if err := repo.Check("example.com", first); err != nil {
		t.Fatalf("Check with the first recorded key = %v, want nil", err)
	}
}

// The other half of the same change: a key that matches none of the host's recorded entries is
// still a mismatch. Scanning them all must not turn into accepting anything.
func TestHostKeyCheckReportsMismatchOnlyWhenNoRecordedKeyMatches(t *testing.T) {
	recorded, _ := generateTestKey(t)
	alsoRecorded, _ := generateTestKeyECDSA(t)
	attacker, _ := generateTestKey(t)
	mv := newMockVaultKH([]string{
		formatKnownHostLine("example.com", recorded),
		formatKnownHostLine("example.com", alsoRecorded),
	})
	repo := persistence.NewKnownHostsRepo(mv)

	err := repo.Check("example.com", attacker)

	if !errors.Is(err, domain.ErrHostKeyMismatch) {
		t.Fatalf("Check with an unrecorded key = %v, want ErrHostKeyMismatch", err)
	}
}

// A known_hosts file imported from OpenSSH with hashing on used to contribute nothing: every entry
// failed the string comparison, the host came back unknown, and the user was walked through
// trust-on-first-use again for a host they had already verified.
func TestHostKeyCheckMatchesHashedEntries(t *testing.T) {
	key, _ := generateTestKey(t)
	mv := newMockVaultKH([]string{hashedKnownHostLine(t, "example.com", key)})
	repo := persistence.NewKnownHostsRepo(mv)

	if err := repo.Check("example.com", key); err != nil {
		t.Fatalf("Check against a hashed entry = %v, want nil", err)
	}
}

func TestHostKeyCheckHashedEntryDoesNotMatchAnotherHost(t *testing.T) {
	key, _ := generateTestKey(t)
	mv := newMockVaultKH([]string{hashedKnownHostLine(t, "example.com", key)})
	repo := persistence.NewKnownHostsRepo(mv)

	if err := repo.Check("evil.example.net", key); !errors.Is(err, domain.ErrUnknownHost) {
		t.Fatalf("Check for a different host = %v, want ErrUnknownHost", err)
	}
}

func TestHostKeyCheckHashedEntryStillDetectsAChangedKey(t *testing.T) {
	recorded, _ := generateTestKey(t)
	attacker, _ := generateTestKey(t)
	mv := newMockVaultKH([]string{hashedKnownHostLine(t, "example.com", recorded)})
	repo := persistence.NewKnownHostsRepo(mv)

	if err := repo.Check("example.com", attacker); !errors.Is(err, domain.ErrHostKeyMismatch) {
		t.Fatalf("Check with a changed key against a hashed entry = %v, want ErrHostKeyMismatch", err)
	}
}

// Replace answers "this host's key of this type changed". Dropping the host's other keys as well
// destroys trust the user established and never revoked.
func TestHostKeyReplaceKeepsTheHostsOtherKeyTypes(t *testing.T) {
	ed25519Key, _ := generateTestKey(t)
	ecdsaKey, _ := generateTestKeyECDSA(t)
	newEd25519Key, _ := generateTestKey(t)
	mv := newMockVaultKH([]string{
		formatKnownHostLine("example.com", ed25519Key),
		formatKnownHostLine("example.com", ecdsaKey),
	})
	repo := persistence.NewKnownHostsRepo(mv)

	if err := repo.Replace(context.Background(), "example.com", newEd25519Key); err != nil {
		t.Fatalf("Replace err = %v", err)
	}

	if err := repo.Check("example.com", newEd25519Key); err != nil {
		t.Errorf("the replacement key was not trusted: %v", err)
	}
	if err := repo.Check("example.com", ecdsaKey); err != nil {
		t.Errorf("the host's ECDSA key was discarded by a replacement of its Ed25519 key: %v", err)
	}
	if err := repo.Check("example.com", ed25519Key); !errors.Is(err, domain.ErrHostKeyMismatch) {
		t.Errorf("the superseded key is still trusted: %v", err)
	}
}
