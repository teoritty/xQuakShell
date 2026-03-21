package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

// --- Stubs for SessionManager SSH tests (no network) ---

type sshTestConnRepo struct {
	conn *domain.Connection
}

func (s sshTestConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return nil, nil
}
func (s sshTestConnRepo) SaveFolder(context.Context, *domain.ConnectionFolder) error { return nil }
func (s sshTestConnRepo) DeleteFolder(context.Context, string) error                 { return nil }
func (s sshTestConnRepo) GetAllConnections(context.Context) ([]domain.Connection, error) {
	return nil, nil
}
func (s sshTestConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}
func (s sshTestConnRepo) GetByID(_ context.Context, _ string) (*domain.Connection, error) {
	if s.conn == nil {
		return nil, domain.ErrConnectionNotFound
	}
	return s.conn, nil
}
func (s sshTestConnRepo) Save(context.Context, *domain.Connection) error { return nil }
func (s sshTestConnRepo) Delete(context.Context, string) error           { return nil }
func (s sshTestConnRepo) MoveToFolder(context.Context, []string, string) error {
	return nil
}
func (s sshTestConnRepo) MoveFolder(context.Context, string, string) error { return nil }
func (s sshTestConnRepo) ReorderConnections(context.Context, []string, string) error {
	return nil
}
func (s sshTestConnRepo) ReorderFolders(context.Context, []string, string) error { return nil }

type sshTestVaultRepo struct{}

func (sshTestVaultRepo) Unlock(context.Context, string) error { return nil }
func (sshTestVaultRepo) Lock()                                {}
func (sshTestVaultRepo) IsUnlocked() bool                     { return true }
func (sshTestVaultRepo) GetData() (*domain.VaultData, error)  { return domain.NewVaultData(), nil }
func (sshTestVaultRepo) SaveData(context.Context, *domain.VaultData) error {
	return nil
}

type sshTestIdentRepo struct{}

func (sshTestIdentRepo) GetAll(context.Context) ([]domain.SSHIdentity, error) { return nil, nil }
func (sshTestIdentRepo) GetKeyBlob(context.Context, string) ([]byte, error) {
	return nil, domain.ErrIdentityNotFound
}
func (sshTestIdentRepo) Import(context.Context, []byte, string) (*domain.SSHIdentity, error) {
	return nil, nil
}
func (sshTestIdentRepo) Delete(context.Context, string) error { return nil }

type sshTestPasswordRepo struct {
	password string
}

func (p sshTestPasswordRepo) Import(context.Context, []byte, string) (string, error) { return "", nil }
func (p sshTestPasswordRepo) Get(context.Context, string) ([]byte, error) {
	return []byte(p.password), nil
}
func (p sshTestPasswordRepo) Delete(context.Context, string) error { return nil }
func (p sshTestPasswordRepo) List(context.Context) ([]domain.PasswordBlob, error) {
	return nil, nil
}

type sshTestKnownHosts struct{}

func (sshTestKnownHosts) Check(string, gossh.PublicKey) error { return nil }
func (sshTestKnownHosts) Add(context.Context, string, gossh.PublicKey) error {
	return nil
}
func (sshTestKnownHosts) List() ([]domain.KnownHostEntry, error) { return nil, nil }
func (sshTestKnownHosts) Remove(context.Context, string) error   { return nil }
func (sshTestKnownHosts) Replace(context.Context, string, gossh.PublicKey) error {
	return nil
}

type acceptAllHostKeys struct{}

func (acceptAllHostKeys) Build(_ domain.KnownHostsRepository) gossh.HostKeyCallback {
	return func(string, net.Addr, gossh.PublicKey) error { return nil }
}

type mapPassphraseCache struct {
	mu sync.Mutex
	m  map[string]string
}

func (m *mapPassphraseCache) Get(id string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		return "", false
	}
	v, ok := m.m[id]
	return v, ok
}

func (m *mapPassphraseCache) Set(id, passphrase string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = make(map[string]string)
	}
	m.m[id] = passphrase
}

func (m *mapPassphraseCache) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m = make(map[string]string)
}

type neverJumpBuilder struct{}

func (neverJumpBuilder) BuildChain(context.Context, []domain.JumpHop, string, int, int, *domain.ProxyAuth, domain.SSHClientFactory, gossh.HostKeyCallback, domain.JumpHopAuthResolver) (net.Conn, func(), error) {
	panic("jump chain must not be used in this test")
}

type errJumpBuilder struct {
	err error
}

func (e errJumpBuilder) BuildChain(context.Context, []domain.JumpHop, string, int, int, *domain.ProxyAuth, domain.SSHClientFactory, gossh.HostKeyCallback, domain.JumpHopAuthResolver) (net.Conn, func(), error) {
	return nil, nil, e.err
}

type stubKeySigner struct{}

func (stubKeySigner) ParsePrivateKeyWithPassphrase([]byte, string) (gossh.Signer, error) {
	return nil, errors.New("no key in test")
}

type errSSHFactory struct {
	err error
}

func (e errSSHFactory) Create(context.Context, domain.SSHClientConfig) (domain.SSHClient, error) {
	return nil, e.err
}

func testPublicKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return pk
}

func passwordSSHConnection() *domain.Connection {
	return &domain.Connection{
		ID:       "c1",
		FolderID: "f1",
		Name:     "test",
		Host:     "example.com",
		Port:     22,
		Protocol: domain.ProtocolSSH,
		Users: []domain.ConnectionUser{{
			ID:       "u1",
			Username: "root",
			Auth:     domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{PasswordID: "p1"},
		}},
		DefaultUserID: "u1",
	}
}

func TestSSHConnectHostKeyUnknown(t *testing.T) {
	pk := testPublicKey(t)
	hkErr := &domain.HostKeyVerificationError{Err: domain.ErrUnknownHost, Host: "example.com:22", Key: pk}

	var last domain.ConnectionSession
	var mu sync.Mutex
	onChange := func(s domain.ConnectionSession) {
		mu.Lock()
		last = s
		mu.Unlock()
	}

	sm := NewSessionManager(SessionManagerConfig{
		ConnRepo:                sshTestConnRepo{conn: passwordSSHConnection()},
		VaultRepo:               sshTestVaultRepo{},
		IdentRepo:               sshTestIdentRepo{},
		PasswordRepo:            sshTestPasswordRepo{password: "secret"},
		KnownHosts:              sshTestKnownHosts{},
		SSHFactory:              errSSHFactory{err: hkErr},
		PassphraseCache:         &mapPassphraseCache{},
		HostKeyCallbackBuilder:  acceptAllHostKeys{},
		JumpTransportBuilder:    neverJumpBuilder{},
		PrivateKeySignerFactory: stubKeySigner{},
		OnStateChange:           onChange,
	})

	_, err := sm.OpenSession("c1")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	waitForState(t, &mu, &last, domain.SessionHostKeyRequired, 2*time.Second)
}

func TestSSHConnectGenericError(t *testing.T) {
	var last domain.ConnectionSession
	var mu sync.Mutex
	onChange := func(s domain.ConnectionSession) {
		mu.Lock()
		last = s
		mu.Unlock()
	}

	sm := NewSessionManager(SessionManagerConfig{
		ConnRepo:                sshTestConnRepo{conn: passwordSSHConnection()},
		VaultRepo:               sshTestVaultRepo{},
		IdentRepo:               sshTestIdentRepo{},
		PasswordRepo:            sshTestPasswordRepo{password: "secret"},
		KnownHosts:              sshTestKnownHosts{},
		SSHFactory:              errSSHFactory{err: errors.New("dial refused")},
		PassphraseCache:         &mapPassphraseCache{},
		HostKeyCallbackBuilder:  acceptAllHostKeys{},
		JumpTransportBuilder:    neverJumpBuilder{},
		PrivateKeySignerFactory: stubKeySigner{},
		OnStateChange:           onChange,
	})

	_, err := sm.OpenSession("c1")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	waitForState(t, &mu, &last, domain.SessionError, 2*time.Second)
}

func TestSSHJumpChainHostKeyUnknown(t *testing.T) {
	pk := testPublicKey(t)
	hkErr := &domain.HostKeyVerificationError{Err: domain.ErrUnknownHost, Host: "hop1:22", Key: pk}

	conn := passwordSSHConnection()
	conn.JumpChain = domain.JumpChainConfig{
		Hops: []domain.JumpHop{{
			Host: "hop1.example.com", Port: 22, Username: "jump",
			Auth:     domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{PasswordID: "p1"},
		}},
	}

	var last domain.ConnectionSession
	var mu sync.Mutex
	onChange := func(s domain.ConnectionSession) {
		mu.Lock()
		last = s
		mu.Unlock()
	}

	sm := NewSessionManager(SessionManagerConfig{
		ConnRepo:                sshTestConnRepo{conn: conn},
		VaultRepo:               sshTestVaultRepo{},
		IdentRepo:               sshTestIdentRepo{},
		PasswordRepo:            sshTestPasswordRepo{password: "secret"},
		KnownHosts:              sshTestKnownHosts{},
		SSHFactory:              errSSHFactory{}, // not used when jump fails first
		PassphraseCache:         &mapPassphraseCache{},
		HostKeyCallbackBuilder:  acceptAllHostKeys{},
		JumpTransportBuilder:    errJumpBuilder{err: hkErr},
		PrivateKeySignerFactory: stubKeySigner{},
		OnStateChange:           onChange,
	})

	_, err := sm.OpenSession("c1")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	waitForState(t, &mu, &last, domain.SessionHostKeyRequired, 2*time.Second)
}

func waitForState(t *testing.T, mu *sync.Mutex, last *domain.ConnectionSession, want domain.SessionState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		st := last.State
		mu.Unlock()
		if st == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	got := last.State
	mu.Unlock()
	t.Fatalf("state: got %q, want %q", got, want)
}
