package usecase_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/infra/syncstate"
	"xquakshell/internal/syncserver/servertest"
	srvusecase "xquakshell/internal/syncserver/usecase"
	"xquakshell/internal/usecase"
)

// accountServer adds the account routes to svcServer.
type accountServer struct {
	*svcServer
	loginResult func(synctrust.LoginResult) synctrust.LoginResult
	// failRekey loses the next password re-registration, as a dropped connection would.
	failRekey *bool
}

var testServerIdentity = bytes.Repeat([]byte{0x51}, 32)

func (s *accountServer) Hello(context.Context) (synctrust.HelloInfo, error) {
	return synctrust.HelloInfo{IdentityKey: testServerIdentity, Context: []byte("hello-context")}, nil
}

func (s *accountServer) SetAccessToken(token string) { s.token = token }

func (s *accountServer) RegisterInit(ctx context.Context, invite, username string, account vaultsync.ID, request []byte) ([]byte, error) {
	resp, err := s.h.Svc.RegisterStart(ctx, invite, username, account, request)
	return resp, remote(err)
}

func (s *accountServer) RegisterFinish(ctx context.Context, r synctrust.Registration) error {
	return remote(s.h.Svc.RegisterFinish(ctx, srvusecase.Registration{Invite: r.Invite, Username: r.Username, Account: r.Account,
		Record: r.Record, IdentityKey: r.IdentityKey, EncryptionKey: r.EncryptionKey, Bundle: r.Bundle, Device: r.Device, Vault: r.Vault}))
}

func (s *accountServer) LoginInit(ctx context.Context, username string, ke1 []byte) (string, vaultsync.ID, []byte, error) {
	id, account, ke2, err := s.h.Svc.LoginStart(ctx, username, "ip", ke1)
	return id, account, ke2, remote(err)
}

func (s *accountServer) LoginFinish(ctx context.Context, loginID string, ke3 []byte) (synctrust.LoginResult, error) {
	res, err := s.h.Svc.LoginFinish(ctx, loginID, "ip", ke3)
	out := synctrust.LoginResult{AccountToken: res.AccountToken, Bundle: res.Bundle, IdentityKey: res.IdentityKey, EncryptionKey: res.EncryptionKey}
	if s.loginResult != nil {
		out = s.loginResult(out)
	}
	return out, remote(err)
}

func (s *accountServer) DeviceChallenge(ctx context.Context, account, device vaultsync.ID) ([]byte, error) {
	n, err := s.h.Svc.DeviceChallenge(ctx, account, device)
	return n, remote(err)
}

func (s *accountServer) DeviceVerify(ctx context.Context, account, device vaultsync.ID, nonce, sig []byte) (string, error) {
	tok, _, err := s.h.Svc.DeviceVerify(ctx, account, device, nonce, sig)
	return tok, remote(err)
}

func (s *accountServer) PasswordInit(ctx context.Context, request []byte) ([]byte, error) {
	if s.failRekey != nil && *s.failRekey {
		*s.failRekey = false
		return nil, errors.New("connection reset")
	}
	p, err := s.principal()
	if err != nil {
		return nil, err
	}
	resp, err := s.h.Svc.PasswordStart(ctx, p, request)
	return resp, remote(err)
}

func (s *accountServer) PasswordFinish(ctx context.Context, record, bundle, sig []byte) error {
	p, err := s.principal()
	if err != nil {
		return err
	}
	return remote(s.h.Svc.PasswordFinish(ctx, p, record, bundle, sig))
}

func (s *accountServer) Events(context.Context) (<-chan synctrust.RemoteEvent, error) {
	return nil, errors.New("not used")
}

// fakeAKE is the client half that agrees with servertest.FakeAKE: the record and the login proof are
// functions of the password, so the server accepts exactly the right password.
type fakeAKE struct{ pw, ke1 []byte }

func (a *fakeAKE) record() []byte { return append([]byte("rec:"), a.pw...) }

func (a *fakeAKE) export(account vaultsync.ID) []byte {
	sum := sha256.Sum256(append(append([]byte("export:"), a.pw...), account[:]...))
	return append(sum[:], sum[:]...)
}

func (a *fakeAKE) RegistrationStart(pw []byte) ([]byte, error) {
	a.pw = bytes.Clone(pw)
	return []byte("req"), nil
}

func (a *fakeAKE) RegistrationFinish(_ []byte, account vaultsync.ID) ([]byte, []byte, error) {
	return a.record(), a.export(account), nil
}

func (a *fakeAKE) LoginStart(pw []byte) ([]byte, error) {
	a.pw = bytes.Clone(pw)
	a.ke1 = []byte("ke1")
	return a.ke1, nil
}

func (a *fakeAKE) LoginFinish(_ []byte, account vaultsync.ID) ([]byte, []byte, []byte, error) {
	return servertest.CorrectKE3(a.ke1, a.record()), nil, a.export(account), nil
}

type device struct {
	t         *testing.T
	failRekey bool
	backup    *fakeBackup
	state     *syncstate.DB
	vault     *syncTestVault
	svc       *usecase.SyncAccountService
	server    *accountServer
	session   usecase.SyncSession
	engine    *usecase.SyncEngine
	// now is the device's clock; tests move it to cross retention windows.
	now atomic.Int64
}

func newDevice(t *testing.T, h *servertest.Harness, name string) *device {
	d := &device{t: t, vault: &syncTestVault{data: domain.NewVaultData()}}
	d.now.Store(7)
	st, err := syncstate.Open(syncCtx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	d.state = st
	d.backup = &fakeBackup{vault: d.vault}
	d.svc = usecase.NewSyncAccountService(usecase.SyncAccountDeps{Vault: d.vault, NewEngine: d.newEngine, Backup: d.backup,
		NewServer: func(string, []byte) (synctrust.AccountServer, error) {
			d.server = &accountServer{svcServer: &svcServer{h: h}, failRekey: &d.failRekey}
			return d.server, nil
		},
		NewAKE: func([]byte) (vaultsync.PasswordAKEClient, error) { return &fakeAKE{}, nil },
		Crypto: synccrypto.Account{}, Random: rand.Reader, NowUnixMilli: d.now.Load, DeviceName: name, Platform: "test"})
	return d
}

func creds(password string) usecase.SyncCredentials {
	return usecase.SyncCredentials{ServerURL: "https://sync.test", Pin: bytes.Clone(testServerIdentity), Username: "alice",
		Password: []byte(password)}
}

func (d *device) newEngine(session usecase.SyncSession) *usecase.SyncEngine {
	return usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: session.Server, State: d.state, Vault: d.vault, Keys: session.Keys,
		Cipher: synccrypto.Cipher{}, Crypto: synccrypto.Account{}, NowUnixMilli: d.now.Load})
}

func (d *device) start(session usecase.SyncSession) {
	d.t.Helper()
	d.session = session
	d.engine = d.newEngine(session)
	if err := d.engine.EnsureVaultMeta(syncCtx, session.Personal, rand.Reader); err != nil {
		d.t.Fatalf("vault meta: %v", err)
	}
}

func (d *device) sync() usecase.SyncReport {
	d.t.Helper()
	r, err := d.engine.Sync(syncCtx, d.session.Personal)
	if err != nil {
		d.t.Fatalf("sync: %v", err)
	}
	return r
}

func registered(t *testing.T) (*servertest.Harness, *device) {
	h := servertest.NewHarness(t, 0)
	a := newDevice(t, h, "laptop")
	c := creds("correct horse")
	c.Invite = h.Invite()
	s, err := a.svc.Register(syncCtx, c)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	a.start(s)
	return h, a
}

func TestRegisterSyncAndEnrolASecondDevice(t *testing.T) {
	h, a := registered(t)
	if d, _ := a.vault.GetData(); d.Sync == nil || d.Sync.Account == nil || d.Sync.Account.Username != "alice" {
		t.Fatal("the account was not stored in the vault")
	}
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f1", Name: "Prod"})
		return nil
	})
	a.sync()

	b := newDevice(t, h, "phone")
	s, err := b.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	b.start(s)
	b.sync()
	if d, _ := b.vault.GetData(); len(d.Folders) != 1 || d.Folders[0].Name != "Prod" {
		t.Fatalf("the second device has %+v", d.Folders)
	}
	if b.session.Keys.Device() == a.session.Keys.Device() || b.session.Personal != a.session.Personal {
		t.Fatal("the second device did not get its own identity in the same account")
	}

	_ = b.vault.UpdateData(syncCtx, func(d *domain.VaultData) error { d.Folders[0].Name = "Production"; return nil })
	b.sync()
	a.sync()
	if d, _ := a.vault.GetData(); d.Folders[0].Name != "Production" {
		t.Fatal("the first device did not verify and apply the new device's commit")
	}
}

func TestLoginWithAWrongPassword(t *testing.T) {
	h, _ := registered(t)
	b := newDevice(t, h, "phone")
	if _, err := b.svc.Login(syncCtx, creds("wrong")); !errors.Is(err, usecase.ErrSyncLogin) {
		t.Fatalf("err = %v, want ErrSyncLogin", err)
	}
	if d, _ := b.vault.GetData(); d.Sync != nil {
		t.Fatal("a failed login stored something")
	}
}

func TestEnablingTwiceIsRefused(t *testing.T) {
	h, a := registered(t)
	c := creds("correct horse")
	c.Invite = h.Invite()
	if _, err := a.svc.Register(syncCtx, c); !errors.Is(err, usecase.ErrSyncAlreadyEnabled) {
		t.Fatalf("err = %v", err)
	}
}

func TestSubstitutedKeysAtLoginAreRefused(t *testing.T) {
	h, _ := registered(t)
	b := newDevice(t, h, "phone")
	b.svc = usecase.NewSyncAccountService(usecase.SyncAccountDeps{Vault: b.vault,
		NewServer: func(string, []byte) (synctrust.AccountServer, error) {
			return &accountServer{svcServer: &svcServer{h: h}, loginResult: func(r synctrust.LoginResult) synctrust.LoginResult {
				r.IdentityKey = bytes.Repeat([]byte{1}, 32)
				return r
			}}, nil
		},
		NewAKE: func([]byte) (vaultsync.PasswordAKEClient, error) { return &fakeAKE{}, nil },
		Crypto: synccrypto.Account{}, Random: rand.Reader, NowUnixMilli: func() int64 { return 7 }})
	if _, err := b.svc.Login(syncCtx, creds("correct horse")); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; keys the bundle does not hold must not be trusted", err)
	}
	if d, _ := b.vault.GetData(); d.Sync != nil {
		t.Fatal("a refused login stored something")
	}
}

func TestEnablingNeedsAConfirmedPin(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	a := newDevice(t, h, "laptop")
	c := creds("correct horse")
	c.Pin = nil
	c.Invite = h.Invite()
	if _, err := a.svc.Register(syncCtx, c); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("err = %v; nothing may be sent before the user confirmed the server", err)
	}
}

func TestExpiredAccessTokenIsRenewed(t *testing.T) {
	h, a := registered(t)
	h.Clock.Advance(16 * 60_000)
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f2", Name: "Dev"})
		return nil
	})
	if r := a.sync(); r.Pushed == 0 {
		t.Fatal("nothing pushed after the token expired")
	}
}

func TestPasswordIsClearedAfterUse(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	a := newDevice(t, h, "laptop")
	c := creds("correct horse")
	c.Invite = h.Invite()
	pw := c.Password
	if _, err := a.svc.Register(syncCtx, c); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pw, make([]byte, len(pw))) {
		t.Fatal("the sync password stayed in memory after registration")
	}
}

func TestKeyRingNeedsAWrapForThisAccount(t *testing.T) {
	_, a := registered(t)
	_, err := a.session.Keys.VaultKey(a.session.Personal, 3, nil)
	if !errors.Is(err, usecase.ErrNoVaultKey) {
		t.Fatalf("err = %v", err)
	}
	if _, err := usecase.NewSyncKeyRing(&domain.SyncAccount{Account: make([]byte, 16)}, synccrypto.Account{}); !errors.Is(err, usecase.ErrSyncAccount) {
		t.Fatalf("an incomplete account built a key ring: %v", err)
	}
}

func loginWith(t *testing.T, h *servertest.Harness, tamperLog func([]vaultsync.DeviceEntry) []vaultsync.DeviceEntry,
	tamperResult func(synctrust.LoginResult) synctrust.LoginResult) error {
	t.Helper()
	b := newDevice(t, h, "phone")
	b.svc = usecase.NewSyncAccountService(usecase.SyncAccountDeps{Vault: b.vault,
		NewServer: func(string, []byte) (synctrust.AccountServer, error) {
			return &accountServer{svcServer: &svcServer{h: h, tamperLog: tamperLog}, loginResult: tamperResult}, nil
		},
		NewAKE: func([]byte) (vaultsync.PasswordAKEClient, error) { return &fakeAKE{}, nil },
		Crypto: synccrypto.Account{}, Random: rand.Reader, NowUnixMilli: func() int64 { return 7 }})
	_, err := b.svc.Login(syncCtx, creds("correct horse"))
	if d, _ := b.vault.GetData(); err != nil && d.Sync != nil {
		t.Fatal("a refused login stored something")
	}
	return err
}

func TestSubstitutedEncryptionKeyAtLoginIsRefused(t *testing.T) {
	h, _ := registered(t)
	err := loginWith(t, h, nil, func(r synctrust.LoginResult) synctrust.LoginResult {
		_, other, _ := synccrypto.GenerateEncryptionKey()
		r.EncryptionKey = other
		return r
	})
	if !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; vault keys would be wrapped to a key the server chose", err)
	}
}

func TestForgedDeviceLogAtLoginIsRefused(t *testing.T) {
	h, _ := registered(t)
	err := loginWith(t, h, func(es []vaultsync.DeviceEntry) []vaultsync.DeviceEntry {
		es[0].Cert.Name = "evil"
		return es
	}, nil)
	if !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

// A server that hides a succession can serve the old bundle and the old keys together; the device
// log still carries the succession, and its current key must be the bundle's.
func TestHiddenSuccessionAtLoginIsRefused(t *testing.T) {
	h, a := registered(t)
	acct := a.vault.data.Sync.Account
	account := vaultsync.ID(acct.Account)
	oldEnc := bytes.Clone(acct.EncryptionKey)
	aik := ed25519.NewKeyFromSeed(acct.IdentitySeed)
	entries, _ := a.session.Server.DeviceLog(syncCtx, account, 0)
	log, _ := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, aik.Public().(ed25519.PublicKey), oldEnc)
	for i := range entries {
		_ = log.Apply(&entries[i])
	}
	sign := func(e vaultsync.DeviceEntry) vaultsync.DeviceEntry {
		msg, _ := e.SigningBytes()
		e.Sig = ed25519.Sign(aik, msg)
		return e
	}
	_, spare, _ := ed25519.GenerateKey(rand.Reader)
	spareID := servertest.RandomID(t)
	add := sign(vaultsync.DeviceEntry{Account: account, Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpAddDevice,
		Cert: &vaultsync.DeviceCert{Device: spareID, SigningKey: spare.Public().(ed25519.PublicKey), Name: "spare", Platform: "x"}, SigAlg: vaultsync.SigEd25519})
	_ = log.Apply(&add)
	revoke := sign(vaultsync.DeviceEntry{Account: account, Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpRevokeDevice, Revoked: spareID, SigAlg: vaultsync.SigEd25519})
	_ = log.Apply(&revoke)
	_, newAIK, _ := ed25519.GenerateKey(rand.Reader)
	_, newEnc, _ := synccrypto.GenerateEncryptionKey()
	succ := vaultsync.DeviceEntry{Account: account, Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpSuccession, NewSuite: vaultsync.Suite1,
		NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: newEnc, SigAlg: vaultsync.SigEd25519}
	msg, _ := succ.SigningBytes()
	succ.Sig, succ.SuccessorSig = ed25519.Sign(aik, msg), ed25519.Sign(newAIK, msg)
	if err := a.session.Server.AppendDeviceLog(syncCtx, []vaultsync.DeviceEntry{add}); err != nil {
		t.Fatal(err)
	}
	if err := a.session.Server.AppendDeviceLog(syncCtx, []vaultsync.DeviceEntry{revoke, succ}); err != nil {
		t.Fatal(err)
	}
	err := loginWith(t, h, nil, func(r synctrust.LoginResult) synctrust.LoginResult {
		r.EncryptionKey = oldEnc
		return r
	})
	if !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; the device log moved on to another key than the bundle holds", err)
	}
}

func TestKeyRingUsesOnlyThisAccountsWrapAtThatEpoch(t *testing.T) {
	_, a := registered(t)
	ops, err := a.session.Server.Membership(syncCtx, a.session.Personal, 0)
	if err != nil || len(ops) == 0 {
		t.Fatalf("membership: %v", err)
	}
	if _, err := a.session.Keys.VaultKey(a.session.Personal, 0, ops); err != nil {
		t.Fatalf("the genesis wrap does not open: %v", err)
	}
	if _, err := a.session.Keys.VaultKey(a.session.Personal, 3, ops); !errors.Is(err, usecase.ErrNoVaultKey) {
		t.Fatalf("epoch 3: %v; a wrap for epoch 0 must not be tried for another epoch", err)
	}
	foreign := ops[0]
	foreign.Wraps = []vaultsync.KeyWrap{{Member: vaultsync.ID{9}, Digest: foreign.Wraps[0].Digest, Ciphertext: foreign.Wraps[0].Ciphertext}}
	if _, err := a.session.Keys.VaultKey(a.session.Personal, 0, []vaultsync.MemberOp{foreign}); !errors.Is(err, usecase.ErrNoVaultKey) {
		t.Fatalf("another member's wrap: %v", err)
	}
}

func TestOtherDevicesAreToldWhenADeviceJoins(t *testing.T) {
	h, a := registered(t)
	a.sync()
	b := newDevice(t, h, "phone")
	s, err := b.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	b.start(s)
	r := a.sync()
	if len(r.Pull.NewDevices) != 1 || r.Pull.NewDevices[0].Name != "phone" || r.Pull.NewDevices[0].Device != b.session.Keys.Device() {
		t.Fatalf("new devices = %+v; the user must be told a device joined", r.Pull.NewDevices)
	}
	if again := a.sync(); len(again.Pull.NewDevices) != 0 {
		t.Fatal("the same device was announced twice")
	}
	if own := b.sync(); len(own.Pull.NewDevices) != 0 {
		t.Fatalf("a device was announced to itself: %+v", own.Pull.NewDevices)
	}
}

func TestAFirstPullNeverAnnouncesTheDeviceItself(t *testing.T) {
	h, a := registered(t)
	a.sync()
	c := newDevice(t, h, "tablet")
	s, err := c.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	st, _ := syncstate.Open(syncCtx, filepath.Join(t.TempDir(), "state.db"))
	t.Cleanup(func() { _ = st.Close() })
	engine := usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: s.Server, State: st, Vault: c.vault, Keys: s.Keys,
		Cipher: synccrypto.Cipher{}, NowUnixMilli: func() int64 { return 7 }})
	r, err := engine.Pull(syncCtx, s.Personal)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range r.NewDevices {
		if d.Device == s.Keys.Device() {
			t.Fatal("the device was announced to itself")
		}
	}
	if len(r.NewDevices) != 1 || r.NewDevices[0].Name != "laptop" {
		t.Fatalf("new devices = %+v", r.NewDevices)
	}
}

// countingHello counts hellos and can fail the first request after one, as a renewed server
// certificate makes every pooled connection fail its pin check.
type countingHello struct {
	*accountServer
	hellos       int
	identityOnce bool
}

func (c *countingHello) Hello(ctx context.Context) (synctrust.HelloInfo, error) {
	c.hellos++
	return c.accountServer.Hello(ctx)
}

func (c *countingHello) Vaults(ctx context.Context) ([]synctrust.RemoteVault, error) {
	if c.identityOnce {
		c.identityOnce = false
		return nil, synctrust.ErrServerIdentity
	}
	return c.accountServer.Vaults(ctx)
}

func TestAuthenticatedServerConnectsOnFirstUseAndAfterARenewal(t *testing.T) {
	h, a := registered(t)
	srv := &countingHello{accountServer: &accountServer{svcServer: &svcServer{h: h}}}
	auth := usecase.NewAuthenticatedServer(srv, a.session.Keys)
	if srv.hellos != 0 {
		t.Fatal("constructing the session already talked to the server; it must not before the vault is used")
	}
	if _, err := auth.Vaults(syncCtx); err != nil {
		t.Fatal(err)
	}
	if srv.hellos != 1 {
		t.Fatalf("hellos = %d; the first request must verify the server first", srv.hellos)
	}
	srv.identityOnce = true
	if _, err := auth.Vaults(syncCtx); err != nil {
		t.Fatalf("a renewed certificate was not re-verified: %v", err)
	}
	if srv.hellos != 2 {
		t.Fatalf("hellos = %d, want a second hello after the identity failure", srv.hellos)
	}
}
