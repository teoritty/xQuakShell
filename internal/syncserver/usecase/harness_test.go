package usecase_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/usecase"
)

var ctx = context.Background()

type fakeClock struct {
	mu  sync.Mutex
	now uint64
}

func (c *fakeClock) NowUnixMilli() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(ms uint64) {
	c.mu.Lock()
	c.now += ms
	c.mu.Unlock()
}

// fakeAKE stands in for OPAQUE, whose own adapter tests cover the protocol. The use cases only pass
// its messages through, so a deterministic stand-in lets these tests assert the routing: a correct
// KE3 is the SHA-256 of KE1 and the stored record, anything else fails.
type fakeAKE struct{}

const ke2Size = 64

func (fakeAKE) RegistrationResponse(request []byte, _ vaultsync.ID) ([]byte, error) {
	return append([]byte("resp:"), request...), nil
}

func (fakeAKE) ValidateRecord(record []byte) error {
	if len(record) == 0 {
		return vaultsync.ErrMalformedMessage
	}
	return nil
}

func (fakeAKE) LoginStart(ke1, record []byte, _ vaultsync.ID) ([]byte, []byte, []byte, error) {
	mac := sha256.Sum256(append(bytes.Clone(ke1), record...))
	return bytes.Repeat([]byte{1}, ke2Size), mac[:], []byte("session"), nil
}

func (fakeAKE) LoginStartUnknown(_ []byte, _ vaultsync.ID) ([]byte, error) {
	return bytes.Repeat([]byte{2}, ke2Size), nil
}

func (fakeAKE) LoginFinish(ke3, mac []byte) error {
	if !bytes.Equal(ke3, mac) {
		return vaultsync.ErrAuthFailed
	}
	return nil
}

func correctKE3(ke1, record []byte) []byte {
	mac := sha256.Sum256(append(bytes.Clone(ke1), record...))
	return mac[:]
}

type harness struct {
	t     *testing.T
	svc   *usecase.Service
	db    *store.DB
	clock *fakeClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "xqsyncd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &fakeClock{now: 1_000_000}
	svc := usecase.New(usecase.Config{Store: db, Clock: clock, AKE: fakeAKE{}, Random: rand.Reader, FakeAccountSecret: []byte("fake-secret")})
	return &harness{t: t, svc: svc, db: db, clock: clock}
}

// user is an account as its first device sees it.
type user struct {
	name    string
	account vaultsync.ID
	device  vaultsync.ID
	aik     ed25519.PrivateKey
	dsk     ed25519.PrivateKey
	aek     []byte
	vault   vaultsync.ID
	record  []byte
	log     *vaultsync.DeviceLogState
	access  string
}

func randomID(t *testing.T) vaultsync.ID {
	id, err := vaultsync.NewID(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newUser(t *testing.T, name string) *user {
	_, aik, _ := ed25519.GenerateKey(rand.Reader)
	_, dsk, _ := ed25519.GenerateKey(rand.Reader)
	u := &user{name: name, account: randomID(t), device: randomID(t), aik: aik, dsk: dsk,
		aek: bytes.Repeat([]byte{9}, 1216), vault: randomID(t), record: []byte("opaque-record-" + name)}
	log, err := vaultsync.NewDeviceLogState(u.account, vaultsync.Suite1, aik.Public().(ed25519.PublicKey), u.aek)
	if err != nil {
		t.Fatal(err)
	}
	u.log = log
	return u
}

// deviceEntry signs an entry at the next position of the user's local copy of their log.
func (u *user) deviceEntry(t *testing.T, e vaultsync.DeviceEntry) vaultsync.DeviceEntry {
	t.Helper()
	e.Account, e.Seq, e.Prev, e.SigAlg = u.account, u.log.Seq()+1, u.log.Hash(), vaultsync.SigEd25519
	msg, err := e.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	e.Sig = ed25519.Sign(u.aik, msg)
	if err := u.log.Apply(&e); err != nil {
		t.Fatalf("local log refused its own entry: %v", err)
	}
	return e
}

func (u *user) addDeviceEntry(t *testing.T, device vaultsync.ID, dsk ed25519.PrivateKey) vaultsync.DeviceEntry {
	return u.deviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{
		Device: device, SigningKey: dsk.Public().(ed25519.PublicKey), Name: "test", Platform: "linux"}})
}

func (u *user) createOp(t *testing.T, vault vaultsync.ID) vaultsync.MemberOp {
	t.Helper()
	op := vaultsync.MemberOp{Vault: vault, Seq: 1, Kind: vaultsync.MemberCreate, Author: u.account, AuthorDevice: u.device,
		Suite: vaultsync.Suite1, RetentionDays: 30, SigAlg: vaultsync.SigEd25519,
		Wraps: []vaultsync.KeyWrap{{Member: u.account, Ciphertext: bytes.Repeat([]byte{3}, 1168)}}}
	genesis, _ := vaultsync.NewMembershipState(vault)
	op.Prev = genesis.Hash()
	msg, err := op.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	op.Sig = ed25519.Sign(u.dsk, msg)
	return op
}

func (h *harness) invite() string {
	h.t.Helper()
	code, err := h.svc.CreateInvite(ctx, 3_600_000)
	if err != nil {
		h.t.Fatal(err)
	}
	return code
}

func (h *harness) registration(u *user, invite string) usecase.Registration {
	return usecase.Registration{Invite: invite, Username: u.name, Account: u.account, Record: u.record,
		IdentityKey: u.aik.Public().(ed25519.PublicKey), EncryptionKey: u.aek, Bundle: []byte("sealed-bundle"),
		Device: u.addDeviceEntry(h.t, u.device, u.dsk), Vault: u.createOp(h.t, u.vault)}
}

// register runs both registration steps and a device login, leaving u with an access token.
func (h *harness) register(name string) *user {
	h.t.Helper()
	u := newUser(h.t, name)
	inv := h.invite()
	if _, err := h.svc.RegisterStart(ctx, inv, u.name, u.account, []byte("blinded")); err != nil {
		h.t.Fatalf("register start: %v", err)
	}
	if err := h.svc.RegisterFinish(ctx, h.registration(u, inv)); err != nil {
		h.t.Fatalf("register finish: %v", err)
	}
	u.access = h.deviceLogin(u, u.device, u.dsk)
	return u
}

func (h *harness) deviceLogin(u *user, device vaultsync.ID, dsk ed25519.PrivateKey) string {
	h.t.Helper()
	nonce, err := h.svc.DeviceChallenge(ctx, u.account, device)
	if err != nil {
		h.t.Fatalf("challenge: %v", err)
	}
	msg, _ := vaultsync.DeviceAuthMessage(u.account, device, nonce)
	token, _, err := h.svc.DeviceVerify(ctx, u.account, device, nonce, ed25519.Sign(dsk, msg))
	if err != nil {
		h.t.Fatalf("verify: %v", err)
	}
	return token
}

func (h *harness) principal(token string) usecase.Principal {
	h.t.Helper()
	p, err := h.svc.Authenticate(ctx, token, usecase.TokenAccess)
	if err != nil {
		h.t.Fatalf("authenticate: %v", err)
	}
	return p
}

func isErr(err, target error) bool { return errors.Is(err, target) }
