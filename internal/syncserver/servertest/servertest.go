// Package servertest builds sync-server fixtures for tests of the use cases and the transport: a
// real store in a temporary directory, a deterministic stand-in for OPAQUE, a driven clock, and
// users and vaults as their devices see them, producing the signed logs and commits the server must
// accept or refuse. It is imported only by tests.
package servertest

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"path/filepath"
	"sync"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/usecase"
)

// Ctx is the context fixtures run under.
var Ctx = context.Background()

// FakeClock is a clock tests advance by hand.
type FakeClock struct {
	mu  sync.Mutex
	now uint64
}

// NowUnixMilli implements usecase.Clock.
func (c *FakeClock) NowUnixMilli() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward.
func (c *FakeClock) Advance(ms uint64) {
	c.mu.Lock()
	c.now += ms
	c.mu.Unlock()
}

// FakeAKE stands in for OPAQUE, whose adapter tests cover the protocol. The use cases only route its
// messages, so a deterministic stand-in lets tests assert the routing: a correct KE3 is the SHA-256
// of KE1 and the stored record, anything else fails.
type FakeAKE struct{}

// KE2Size is the length of every KE2 FakeAKE produces, known or unknown account alike.
const KE2Size = 64

func (FakeAKE) RegistrationResponse(request []byte, _ vaultsync.ID) ([]byte, error) {
	return append([]byte("resp:"), request...), nil
}

func (FakeAKE) ValidateRecord(record []byte) error {
	if len(record) == 0 {
		return vaultsync.ErrMalformedMessage
	}
	return nil
}

func (FakeAKE) LoginStart(ke1, record []byte, _ vaultsync.ID) ([]byte, []byte, []byte, error) {
	return bytes.Repeat([]byte{1}, KE2Size), CorrectKE3(ke1, record), []byte("session"), nil
}

func (FakeAKE) LoginStartUnknown(_ []byte, _ vaultsync.ID) ([]byte, error) {
	return bytes.Repeat([]byte{2}, KE2Size), nil
}

func (FakeAKE) LoginFinish(ke3, mac []byte) error {
	if !bytes.Equal(ke3, mac) {
		return vaultsync.ErrAuthFailed
	}
	return nil
}

// CorrectKE3 is the KE3 FakeAKE accepts for a password that matches the record.
func CorrectKE3(ke1, record []byte) []byte {
	mac := sha256.Sum256(append(bytes.Clone(ke1), record...))
	return mac[:]
}

// Harness is a service over a real store with a fake clock and FakeAKE.
type Harness struct {
	T     *testing.T
	Svc   *usecase.Service
	DB    *store.DB
	Clock *FakeClock
}

// NewHarness opens a store in a temp dir; teamQuota 0 keeps the default.
func NewHarness(t *testing.T, teamQuota uint64) *Harness {
	t.Helper()
	db, err := store.Open(Ctx, filepath.Join(t.TempDir(), "xqsyncd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &FakeClock{now: 1_000_000}
	svc := usecase.New(usecase.Config{Store: db, Clock: clock, AKE: FakeAKE{}, Random: rand.Reader,
		FakeAccountSecret: []byte("fake-secret"), TeamQuota: teamQuota})
	return &Harness{T: t, Svc: svc, DB: db, Clock: clock}
}

// RandomID draws a fresh id.
func RandomID(t *testing.T) vaultsync.ID {
	id, err := vaultsync.NewID(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// User is an account as its first device sees it.
type User struct {
	Name    string
	Account vaultsync.ID
	Device  vaultsync.ID
	AIK     ed25519.PrivateKey
	DSK     ed25519.PrivateKey
	AEK     []byte
	Vault   vaultsync.ID
	Record  []byte
	Log     *vaultsync.DeviceLogState
	Access  string
}

// NewUser creates keys and a local device log; nothing is sent.
func NewUser(t *testing.T, name string) *User {
	t.Helper()
	_, aik, _ := ed25519.GenerateKey(rand.Reader)
	_, dsk, _ := ed25519.GenerateKey(rand.Reader)
	u := &User{Name: name, Account: RandomID(t), Device: RandomID(t), AIK: aik, DSK: dsk,
		AEK: bytes.Repeat([]byte{9}, 1216), Vault: RandomID(t), Record: []byte("opaque-record-" + name)}
	log, err := vaultsync.NewDeviceLogState(u.Account, vaultsync.Suite1, aik.Public().(ed25519.PublicKey), u.AEK)
	if err != nil {
		t.Fatal(err)
	}
	u.Log = log
	return u
}

// DeviceEntry signs e at the next position of the user's local log and applies it there.
func (u *User) DeviceEntry(t *testing.T, e vaultsync.DeviceEntry) vaultsync.DeviceEntry {
	t.Helper()
	e.Account, e.Seq, e.Prev, e.SigAlg = u.Account, u.Log.Seq()+1, u.Log.Hash(), vaultsync.SigEd25519
	msg, err := e.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	e.Sig = ed25519.Sign(u.AIK, msg)
	if err := u.Log.Apply(&e); err != nil {
		t.Fatalf("local log refused its own entry: %v", err)
	}
	return e
}

// AddDeviceEntry certifies a device.
func (u *User) AddDeviceEntry(t *testing.T, device vaultsync.ID, dsk ed25519.PrivateKey) vaultsync.DeviceEntry {
	return u.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{
		Device: device, SigningKey: dsk.Public().(ed25519.PublicKey), Name: "test", Platform: "linux"}})
}

// CreateOp is the genesis op of a vault owned by u, signed by u's device.
func (u *User) CreateOp(t *testing.T, vault vaultsync.ID) vaultsync.MemberOp {
	t.Helper()
	genesis, _ := vaultsync.NewMembershipState(vault)
	op := vaultsync.MemberOp{Vault: vault, Seq: 1, Prev: genesis.Hash(), Kind: vaultsync.MemberCreate, Author: u.Account,
		AuthorDevice: u.Device, Suite: vaultsync.Suite1, RetentionDays: 30, SigAlg: vaultsync.SigEd25519,
		Wraps: WrapFor(u.Account)}
	msg, err := op.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	op.Sig = ed25519.Sign(u.DSK, msg)
	return op
}

// WrapFor builds placeholder vault-key wraps of the suite's size; the server never opens them.
func WrapFor(ids ...vaultsync.ID) []vaultsync.KeyWrap {
	out := make([]vaultsync.KeyWrap, len(ids))
	for i, id := range ids {
		out[i] = vaultsync.KeyWrap{Member: id, Ciphertext: bytes.Repeat([]byte{id[0]}, 1168)}
	}
	return out
}

// Invite issues a one-hour invite.
func (h *Harness) Invite() string {
	h.T.Helper()
	code, err := h.Svc.CreateInvite(Ctx, 3_600_000)
	if err != nil {
		h.T.Fatal(err)
	}
	return code
}

// Registration builds a complete registration for u.
func (h *Harness) Registration(u *User, invite string) usecase.Registration {
	return usecase.Registration{Invite: invite, Username: u.Name, Account: u.Account, Record: u.Record,
		IdentityKey: u.AIK.Public().(ed25519.PublicKey), EncryptionKey: u.AEK, Bundle: []byte("sealed-bundle"),
		Device: u.AddDeviceEntry(h.T, u.Device, u.DSK), Vault: u.CreateOp(h.T, u.Vault)}
}

// Register runs both registration steps and a device login, leaving the user with an access token.
func (h *Harness) Register(name string) *User {
	h.T.Helper()
	u := NewUser(h.T, name)
	inv := h.Invite()
	if _, err := h.Svc.RegisterStart(Ctx, inv, u.Name, u.Account, []byte("blinded")); err != nil {
		h.T.Fatalf("register start: %v", err)
	}
	if err := h.Svc.RegisterFinish(Ctx, h.Registration(u, inv)); err != nil {
		h.T.Fatalf("register finish: %v", err)
	}
	u.Access = h.DeviceLogin(u, u.Device, u.DSK)
	return u
}

// DeviceLogin runs the challenge and returns an access token.
func (h *Harness) DeviceLogin(u *User, device vaultsync.ID, dsk ed25519.PrivateKey) string {
	h.T.Helper()
	nonce, err := h.Svc.DeviceChallenge(Ctx, u.Account, device)
	if err != nil {
		h.T.Fatalf("challenge: %v", err)
	}
	msg, _ := vaultsync.DeviceAuthMessage(u.Account, device, nonce)
	token, _, err := h.Svc.DeviceVerify(Ctx, u.Account, device, nonce, ed25519.Sign(dsk, msg))
	if err != nil {
		h.T.Fatalf("verify: %v", err)
	}
	return token
}

// Principal authenticates an access token.
func (h *Harness) Principal(token string) usecase.Principal {
	h.T.Helper()
	p, err := h.Svc.Authenticate(Ctx, token, usecase.TokenAccess)
	if err != nil {
		h.T.Fatalf("authenticate: %v", err)
	}
	return p
}
