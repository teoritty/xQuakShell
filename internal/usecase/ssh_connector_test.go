package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/domain"
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
func (sshTestVaultRepo) GetData() (*domain.VaultData, error) { return domain.NewVaultData(), nil }
func (sshTestVaultRepo) UpdateData(context.Context, func(*domain.VaultData) error) error {
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

func (neverJumpBuilder) BuildChain(context.Context, []domain.JumpHop, string, int, int, domain.SSHClientFactory, gossh.HostKeyCallback, domain.JumpHopAuthResolver) (net.Conn, func(), error) {
	panic("jump chain must not be used in this test")
}

type errJumpBuilder struct {
	err error
}

func (e errJumpBuilder) BuildChain(context.Context, []domain.JumpHop, string, int, int, domain.SSHClientFactory, gossh.HostKeyCallback, domain.JumpHopAuthResolver) (net.Conn, func(), error) {
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
	hkErr := &domain.HostKeyVerificationError{
		Err:  domain.ErrUnknownHost,
		Host: "example.com:22",
		Info: domain.HostKeyInfo{Host: "example.com:22", KeyType: pk.Type()},
	}

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

	_, err := sm.OpenSession(context.Background(), "c1")
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

	_, err := sm.OpenSession(context.Background(), "c1")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	waitForState(t, &mu, &last, domain.SessionError, 2*time.Second)
}

func TestSSHJumpChainHostKeyUnknown(t *testing.T) {
	pk := testPublicKey(t)
	hkErr := &domain.HostKeyVerificationError{
		Err:  domain.ErrUnknownHost,
		Host: "hop1:22",
		Info: domain.HostKeyInfo{Host: "hop1:22", KeyType: pk.Type()},
	}

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

	_, err := sm.OpenSession(context.Background(), "c1")
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

func TestOpenSession_RejectsInvalidDefaultUserAuth(t *testing.T) {
	conn := &domain.Connection{
		ID:   "c1",
		Host: "example.com",
		Port: 22,
		Users: []domain.ConnectionUser{{
			ID:       "u1",
			Username: "alice",
			Auth:     domain.AuthMethodKey,
		}},
		DefaultUserID: "u1",
	}
	sm := NewSessionManager(SessionManagerConfig{
		ConnRepo:                sshTestConnRepo{conn: conn},
		VaultRepo:               sshTestVaultRepo{},
		IdentRepo:               sshTestIdentRepo{},
		PasswordRepo:            sshTestPasswordRepo{},
		SSHFactory:              errSSHFactory{},
		PassphraseCache:         &mapPassphraseCache{},
		HostKeyCallbackBuilder:  acceptAllHostKeys{},
		JumpTransportBuilder:    neverJumpBuilder{},
		PrivateKeySignerFactory: stubKeySigner{},
	})

	_, err := sm.OpenSession(context.Background(), "c1")
	if err == nil {
		t.Fatal("expected validation error for key auth without identities")
	}
}

func TestResolveHopAuthWithCtx_RejectsUnknownAuthMethod(t *testing.T) {
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo:    sshTestVaultRepo{},
		IdentRepo:    sshTestIdentRepo{},
		PasswordRepo: sshTestPasswordRepo{},
	})
	_, _, _, _, err := c.resolveHopAuthWithCtx(context.Background(), "conn1", domain.JumpHop{
		Host:     "bastion",
		Port:     22,
		Username: "jump",
		Auth:     domain.AuthMethodType("token"),
	})
	if err == nil {
		t.Fatal("expected unknown auth method error")
	}
}

func TestResolveHopAuthWithCtx_RejectsKeyAuthWithoutIdentities(t *testing.T) {
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo:    sshTestVaultRepo{},
		IdentRepo:    sshTestIdentRepo{},
		PasswordRepo: sshTestPasswordRepo{},
	})
	_, _, _, _, err := c.resolveHopAuthWithCtx(context.Background(), "conn1", domain.JumpHop{
		Host:     "bastion",
		Port:     22,
		Username: "jump",
		Auth:     domain.AuthMethodKey,
	})
	if err == nil {
		t.Fatal("expected key auth without identities error")
	}
}

type stubAuthStarter struct {
	err error
}

func (s stubAuthStarter) Activate(context.Context, string, string) error { return s.err }

type stubAuthLookup struct {
	kind string
	err  error
}

func (s stubAuthLookup) AuthMethodKind(_, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.kind, nil
}

func (s stubAuthLookup) HasAuthProvider(_ string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return true, nil
}

type stubAuthGrant struct {
	granted bool
}

func (s stubAuthGrant) IsAuthProviderGranted(_ string) bool { return s.granted }

type noopAuthProvider struct{}

func (noopAuthProvider) Prepare(context.Context, string, domain.PluginAuthMethod) ([]byte, error) {
	return nil, nil
}
func (noopAuthProvider) AnswerChallenge(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthChallenge) ([]string, error) {
	return nil, nil
}
func (noopAuthProvider) Sign(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthSignRequest) (domain.PluginAuthSignResult, error) {
	return domain.PluginAuthSignResult{}, nil
}

type stubAuthBuilder struct{}

func (stubAuthBuilder) BuildKeyboardInteractive(context.Context, domain.PluginAuthProvider, string, domain.PluginAuthMethod) domain.AuthMethod {
	return gossh.Password("stub")
}
func (stubAuthBuilder) BuildPublicKey(context.Context, domain.PluginAuthProvider, string, domain.PluginAuthMethod) (domain.AuthMethod, error) {
	return gossh.Password("stub"), nil
}

func pluginAuthConnection() *domain.Connection {
	return &domain.Connection{
		ID: "c1", Host: "example.com", Port: 22, Protocol: domain.ProtocolSSH,
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "root", Auth: domain.AuthMethodPlugin,
			PluginAuth: &domain.PluginAuthConfig{
				PluginID: "com.test.auth", AuthMethodID: "otp",
				Fields: map[string]string{"tenant": "acme"},
			},
		}},
		DefaultUserID: "u1",
	}
}

func TestResolvePluginAuth_NotWired(t *testing.T) {
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo: sshTestVaultRepo{}, IdentRepo: sshTestIdentRepo{}, PasswordRepo: sshTestPasswordRepo{},
	})
	_, _, err := c.resolvePluginAuth(context.Background(), "c1", pluginAuthConnection().DefaultUser().PluginAuth)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("resolvePluginAuth() = %v, want not configured error", err)
	}
}

func TestResolvePluginAuth_GrantDenied(t *testing.T) {
	attempts := NewPluginAuthAttemptRegistry()
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo: sshTestVaultRepo{}, IdentRepo: sshTestIdentRepo{}, PasswordRepo: sshTestPasswordRepo{},
		AuthProvider: noopAuthProvider{}, AuthMethodBuilder: stubAuthBuilder{},
		AuthAttempts: attempts, AuthLookup: stubAuthLookup{kind: domain.AuthProviderKindKeyboardInteractive},
		AuthStarter: stubAuthStarter{}, AuthGrantReader: stubAuthGrant{granted: false},
	})
	_, _, err := c.resolvePluginAuth(context.Background(), "c1", pluginAuthConnection().DefaultUser().PluginAuth)
	if err == nil || !strings.Contains(err.Error(), "not granted") {
		t.Fatalf("resolvePluginAuth() = %v, want grant denied", err)
	}
}

func TestResolvePluginAuth_StarterFails(t *testing.T) {
	attempts := NewPluginAuthAttemptRegistry()
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo: sshTestVaultRepo{}, IdentRepo: sshTestIdentRepo{}, PasswordRepo: sshTestPasswordRepo{},
		AuthProvider: noopAuthProvider{}, AuthMethodBuilder: stubAuthBuilder{},
		AuthAttempts: attempts, AuthLookup: stubAuthLookup{kind: domain.AuthProviderKindKeyboardInteractive},
		AuthStarter: stubAuthStarter{err: errors.New("plugin down")},
		AuthGrantReader: stubAuthGrant{granted: true},
	})
	_, _, err := c.resolvePluginAuth(context.Background(), "c1", pluginAuthConnection().DefaultUser().PluginAuth)
	if err == nil || !strings.Contains(err.Error(), "start auth plugin") {
		t.Fatalf("resolvePluginAuth() = %v, want starter error", err)
	}
}

func TestResolvePluginAuth_HappyPathKeepsAttemptAlive(t *testing.T) {
	attempts := NewPluginAuthAttemptRegistry()
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo: sshTestVaultRepo{}, IdentRepo: sshTestIdentRepo{}, PasswordRepo: sshTestPasswordRepo{},
		AuthProvider: noopAuthProvider{}, AuthMethodBuilder: stubAuthBuilder{},
		AuthAttempts: attempts, AuthLookup: stubAuthLookup{kind: domain.AuthProviderKindKeyboardInteractive},
		AuthStarter: stubAuthStarter{}, AuthGrantReader: stubAuthGrant{granted: true},
	})
	extra, attemptID, err := c.resolvePluginAuth(context.Background(), "c1", pluginAuthConnection().DefaultUser().PluginAuth)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra) != 1 || attemptID == "" {
		t.Fatalf("extra=%v attemptID=%q", extra, attemptID)
	}
	if _, ok := attempts.Lookup(attemptID); !ok {
		t.Fatal("attempt should remain registered until End")
	}
	attempts.End(attemptID)
}

func TestResolveHopAuthWithCtx_PluginAuthReleaseAfterHandshake(t *testing.T) {
	attempts := NewPluginAuthAttemptRegistry()
	c := NewSSHConnector(SSHConnectorConfig{
		VaultRepo: sshTestVaultRepo{}, IdentRepo: sshTestIdentRepo{}, PasswordRepo: sshTestPasswordRepo{},
		AuthProvider: noopAuthProvider{}, AuthMethodBuilder: stubAuthBuilder{},
		AuthAttempts: attempts, AuthLookup: stubAuthLookup{kind: domain.AuthProviderKindKeyboardInteractive},
		AuthStarter: stubAuthStarter{}, AuthGrantReader: stubAuthGrant{granted: true},
	})
	hop := domain.JumpHop{
		Host: "bastion", Port: 22, Username: "jump", Auth: domain.AuthMethodPlugin,
		PluginAuth: pluginAuthConnection().DefaultUser().PluginAuth,
	}
	_, _, _, release, err := c.resolveHopAuthWithCtx(context.Background(), "c1", hop)
	if err != nil {
		t.Fatal(err)
	}
	if release == nil {
		t.Fatal("expected release callback for plugin auth hop")
	}
	// Fill remaining slots so the hop attempt keeps the plugin at capacity.
	for i := 0; i < MaxConcurrentAuthAttempts-1; i++ {
		if _, err := attempts.Begin("com.test.auth", "prefill", "otp"); err != nil {
			t.Fatalf("prefill begin %d: %v", i, err)
		}
	}
	if _, err := attempts.Begin("com.test.auth", "c2", "otp"); err != domainplugin.ErrAuthProviderBusy {
		t.Fatalf("expected busy while hop attempt active, got %v", err)
	}
	release()
	if _, err := attempts.Begin("com.test.auth", "c2", "otp"); err != nil {
		t.Fatalf("expected slot after release, got %v", err)
	}
}
