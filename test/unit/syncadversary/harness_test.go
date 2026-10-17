// Package syncadversary runs the real sync client, engine and state store against the real server
// behind a hostile proxy that rewrites any reply (spec §12.5). Each case asserts the specific
// failure, that nothing reached the local vault, and that the stored sync state did not advance.
package syncadversary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/syncclient"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/infra/syncstate"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/transport"
	"xquakshell/internal/usecase"
)

var ctx = context.Background()

// rewrite edits one reply: the request it answers, its status and its body.
type rewrite func(r *http.Request, status int, body []byte) (int, []byte)

// world is one server behind a hostile proxy.
type world struct {
	t        *testing.T
	h        *servertest.Harness
	inner    http.Handler
	url      string
	identity ed25519.PublicKey
	mu       sync.Mutex
	rewrites []rewrite
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{t: t, h: servertest.NewHarness(t, 0)}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	seed := bytes.Repeat([]byte{0x33}, ed25519.SeedSize)
	w.identity = ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	w.inner, err = transport.New(transport.Config{IdentitySeed: seed, TLSPublicKey: leaf.RawSubjectPublicKeyInfo,
		RatePerSecond: 100_000, Burst: 100_000}, w.h.Svc)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(w.serve))
	srv.Listener = tls.NewListener(srv.Listener, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf}}})
	srv.Start()
	t.Cleanup(srv.Close)
	w.url = "https://" + srv.Listener.Addr().String()
	return w
}

// serve passes a request to the real server and lets the active rewrites edit the reply.
func (w *world) serve(rw http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	w.inner.ServeHTTP(rec, r)
	status, body := rec.Code, rec.Body.Bytes()
	w.mu.Lock()
	rewrites := append([]rewrite(nil), w.rewrites...)
	w.mu.Unlock()
	for _, f := range rewrites {
		status, body = f(r, status, body)
	}
	for k, v := range rec.Header() {
		if k != "Content-Length" {
			rw.Header()[k] = v
		}
	}
	rw.WriteHeader(status)
	_, _ = rw.Write(body)
}

func (w *world) attack(f rewrite) {
	w.mu.Lock()
	w.rewrites = append(w.rewrites, f)
	w.mu.Unlock()
}

// on rewrites the JSON reply of one route kind, decoded into T.
func on[T any](path func(string) bool, edit func(*T)) rewrite {
	return func(r *http.Request, status int, body []byte) (int, []byte) {
		if status != http.StatusOK || !path(r.URL.Path) {
			return status, body
		}
		var v T
		if err := json.Unmarshal(body, &v); err != nil {
			return status, body
		}
		edit(&v)
		out, _ := json.Marshal(v)
		return status, out
	}
}

func suffix(s string) func(string) bool {
	return func(p string) bool { return strings.HasSuffix(p, s) }
}

// fakeAKE agrees with servertest.FakeAKE: record and proof are functions of the password.
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
	a.pw, a.ke1 = bytes.Clone(pw), []byte("ke1")
	return a.ke1, nil
}
func (a *fakeAKE) LoginFinish(_ []byte, account vaultsync.ID) ([]byte, []byte, []byte, error) {
	return servertest.CorrectKE3(a.ke1, a.record()), nil, a.export(account), nil
}

// memVault is an in-memory VaultRepository that deep-copies on every read and write.
type memVault struct {
	mu   sync.Mutex
	data *domain.VaultData
}

func (v *memVault) Exists() bool                                       { return true }
func (v *memVault) Create(context.Context, string) error               { return nil }
func (v *memVault) Unlock(context.Context, string) error               { return nil }
func (v *memVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (v *memVault) Lock()                                              {}
func (v *memVault) IsUnlocked() bool                                   { return true }

func (v *memVault) GetData() (*domain.VaultData, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	raw, _ := json.Marshal(v.data)
	var out domain.VaultData
	_ = json.Unmarshal(raw, &out)
	return &out, nil
}

func (v *memVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	raw, _ := json.Marshal(v.data)
	var work domain.VaultData
	_ = json.Unmarshal(raw, &work)
	if err := mutate(&work); err != nil {
		return err
	}
	v.data = &work
	return nil
}

func (v *memVault) snapshot() []byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	raw, _ := json.Marshal(v.data)
	return raw
}

// device is a full client stack: account service, engine and state store over the real client.
type device struct {
	t       *testing.T
	w       *world
	vault   *memVault
	state   *syncstate.DB
	svc     *usecase.SyncAccountService
	session usecase.SyncSession
	engine  *usecase.SyncEngine
}

func (w *world) device(name string) *device {
	w.t.Helper()
	d := &device{t: w.t, w: w, vault: &memVault{data: domain.NewVaultData()}}
	st, err := syncstate.Open(ctx, filepath.Join(w.t.TempDir(), "state.db"))
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _ = st.Close() })
	d.state = st
	d.svc = usecase.NewSyncAccountService(usecase.SyncAccountDeps{Vault: d.vault, NewEngine: d.newEngine,
		NewServer: func(url string, pin []byte) (synctrust.AccountServer, error) {
			return syncclient.New(syncclient.Config{BaseURL: url, PinnedIdentity: pin})
		},
		NewAKE: func([]byte) (vaultsync.PasswordAKEClient, error) { return &fakeAKE{}, nil },
		Crypto: synccrypto.Account{}, Random: rand.Reader, NowUnixMilli: func() int64 { return 1 }, DeviceName: name, Platform: "test"})
	return d
}

func (d *device) newEngine(s usecase.SyncSession) *usecase.SyncEngine {
	return usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: s.Server, State: d.state, Vault: d.vault, Keys: s.Keys,
		Cipher: synccrypto.Cipher{}, Crypto: synccrypto.Account{}, NowUnixMilli: func() int64 { return 1 }})
}

func (d *device) creds() usecase.SyncCredentials {
	return usecase.SyncCredentials{ServerURL: d.w.url, Pin: bytes.Clone(d.w.identity), Username: "alice", Password: []byte("correct horse")}
}

func (d *device) use(s usecase.SyncSession) {
	d.t.Helper()
	d.session, d.engine = s, d.newEngine(s)
	if err := d.engine.EnsureVaultMeta(ctx, s.Personal, rand.Reader); err != nil {
		d.t.Fatalf("vault meta: %v", err)
	}
}

func (d *device) sync() {
	d.t.Helper()
	if _, err := d.engine.Sync(ctx, d.session.Personal); err != nil {
		d.t.Fatalf("sync: %v", err)
	}
}

func (d *device) addFolder(id, name string) {
	_ = d.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		v.Folders = append(v.Folders, domain.ConnectionFolder{ID: id, Name: name})
		return nil
	})
}

func (d *device) head() uint64 {
	sv, _ := d.state.LoadVault(ctx, d.session.Personal)
	if sv.Head == nil {
		return 0
	}
	return sv.Head.Seq
}

// pair is an account with two devices in step: A writes, B reads.
func pair(t *testing.T) (*world, *device, *device) {
	t.Helper()
	w := newWorld(t)
	a := w.device("laptop")
	c := a.creds()
	c.Invite = w.h.Invite()
	s, err := a.svc.Register(ctx, c)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	a.use(s)
	a.addFolder("f1", "Prod")
	a.sync()
	b := w.device("phone")
	sb, err := b.svc.Login(ctx, b.creds())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	b.use(sb)
	b.sync()
	return w, a, b
}

// expectRefused pulls on d under attack and asserts the failure, an untouched vault and an
// unmoved state.
func expectRefused(t *testing.T, d *device, want error) {
	t.Helper()
	before, seq := d.vault.snapshot(), d.head()
	_, err := d.engine.Sync(ctx, d.session.Personal)
	if !errorsIs(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if !bytes.Equal(before, d.vault.snapshot()) {
		t.Fatal("the vault changed during a refused sync")
	}
	if d.head() != seq {
		t.Fatalf("state moved from %d to %d during a refused sync", seq, d.head())
	}
}
