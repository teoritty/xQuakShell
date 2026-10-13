// Package syncflow runs the sync scenarios of spec §12.6 end to end: the real server over HTTPS on
// loopback, and client stacks composed as the application composes them - real VaultRepo, real
// state.db, real client, real crypto - each in its own temporary data root. Waits are on channels;
// nothing sleeps.
package syncflow

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
	"sync/atomic"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/syncclient"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/infra/syncstate"
	"xquakshell/internal/infra/vault"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/transport"
	"xquakshell/internal/usecase"
)

var ctx = context.Background()

const masterPassword = "master password of this device"

// server is the real transport and server use cases over TLS, with counters and one fault switch.
type server struct {
	t        *testing.T
	h        *servertest.Harness
	inner    http.Handler
	url      string
	identity ed25519.PublicKey
	requests atomic.Int64
	// records counts envelopes served in changes replies, per bearer token.
	mu      sync.Mutex
	records map[string]int
	// dropNextCommitReply stores the next commit and then fails the reply, as a server killed between
	// the two would.
	dropNextCommitReply atomic.Bool
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{t: t, h: servertest.NewHarness(t, 0), records: map[string]int{}}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	seed := bytes.Repeat([]byte{0x44}, ed25519.SeedSize)
	s.identity = ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	s.inner, err = transport.New(transport.Config{IdentitySeed: seed, TLSPublicKey: leaf.RawSubjectPublicKeyInfo,
		RatePerSecond: 100_000, Burst: 100_000}, s.h.Svc)
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	hs.Listener = tls.NewListener(hs.Listener, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf}}})
	hs.Start()
	t.Cleanup(hs.Close)
	s.url = "https://" + hs.Listener.Addr().String()
	return s
}

func (s *server) serve(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	rec := httptest.NewRecorder()
	s.inner.ServeHTTP(rec, r)
	if strings.HasSuffix(r.URL.Path, "/commit") && rec.Code == http.StatusOK && s.dropNextCommitReply.CompareAndSwap(true, false) {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/changes") && rec.Code == http.StatusOK {
		var c syncwire.Changes
		if json.Unmarshal(rec.Body.Bytes(), &c) == nil {
			s.mu.Lock()
			for _, e := range c.Commits {
				s.records[r.Header.Get("Authorization")] += len(e.Records)
			}
			s.mu.Unlock()
		}
	}
	for k, v := range rec.Header() {
		if k != "Content-Length" {
			w.Header()[k] = v
		}
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

// servedRecords is how many envelopes changes replies carried in total.
func (s *server) servedRecords() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, v := range s.records {
		n += v
	}
	return n
}

// commits is how many commits the server stored for a vault.
func (s *server) commits(d *device) int {
	p := s.h.Principal(s.tokenOf(d))
	all, _, err := s.h.Svc.Changes(ctx, p, d.session.Personal, 0, 1000)
	if err != nil {
		s.t.Fatal(err)
	}
	return len(all)
}

// tokenOf signs d in through the harness: the client keeps its own token private, and a fresh device
// challenge yields an equivalent one.
func (s *server) tokenOf(d *device) string {
	acct := d.account()
	return s.h.DeviceLogin(&servertest.User{Account: vaultsync.ID(acct.Account)}, vaultsync.ID(acct.Device), ed25519.NewKeyFromSeed(acct.DeviceSeed))
}

// fakeAKE agrees with servertest.FakeAKE; real OPAQUE runs in cmd/xqsyncd's end-to-end test, at
// its full cost, and every other piece here is real.
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

// device is one client stack over its own data root.
type device struct {
	t       *testing.T
	s       *server
	dir     string
	vault   *persistence.VaultRepo
	state   *syncstate.DB
	svc     *usecase.SyncAccountService
	session usecase.SyncSession
	engine  *usecase.SyncEngine
	now     atomic.Int64
}

func (s *server) device(name string) *device {
	s.t.Helper()
	d := &device{t: s.t, s: s, dir: s.t.TempDir()}
	d.now.Store(1)
	d.vault = persistence.NewVaultRepo(filepath.Join(d.dir, "vault"))
	if err := d.vault.Create(ctx, masterPassword); err != nil {
		s.t.Fatal(err)
	}
	st, err := syncstate.Open(ctx, filepath.Join(d.dir, "state.db"))
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = st.Close() })
	d.state = st
	d.svc = usecase.NewSyncAccountService(usecase.SyncAccountDeps{Vault: d.vault, NewEngine: d.newEngine,
		Backup: vault.Backups{Dir: filepath.Join(d.dir, "vault"), Now: time.Now},
		NewServer: func(url string, pin []byte) (synctrust.AccountServer, error) {
			return syncclient.New(syncclient.Config{BaseURL: url, PinnedIdentity: pin})
		},
		NewAKE: func([]byte) (vaultsync.PasswordAKEClient, error) { return &fakeAKE{}, nil },
		Crypto: synccrypto.Account{}, Random: rand.Reader, NowUnixMilli: d.now.Load, DeviceName: name, Platform: "test"})
	return d
}

func (d *device) newEngine(s usecase.SyncSession) *usecase.SyncEngine {
	return usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: s.Server, State: d.state, Vault: d.vault, Keys: s.Keys,
		Cipher: synccrypto.Cipher{}, Crypto: synccrypto.Account{}, NowUnixMilli: d.now.Load})
}

func (d *device) creds(password string) usecase.SyncCredentials {
	return usecase.SyncCredentials{ServerURL: d.s.url, Pin: bytes.Clone(d.s.identity), Username: "alice", Password: []byte(password)}
}

func (d *device) use(s usecase.SyncSession) {
	d.t.Helper()
	d.session, d.engine = s, d.newEngine(s)
	if err := d.engine.EnsureVaultMeta(ctx, s.Personal, rand.Reader); err != nil {
		d.t.Fatalf("vault meta: %v", err)
	}
}

func (d *device) sync() usecase.SyncReport {
	d.t.Helper()
	r, err := d.engine.Sync(ctx, d.session.Personal)
	if err != nil {
		d.t.Fatalf("sync: %v", err)
	}
	return r
}

func (d *device) edit(f func(*domain.VaultData)) {
	d.t.Helper()
	if err := d.vault.UpdateData(ctx, func(v *domain.VaultData) error { f(v); return nil }); err != nil {
		d.t.Fatal(err)
	}
}

func (d *device) data() *domain.VaultData {
	d.t.Helper()
	v, err := d.vault.GetData()
	if err != nil {
		d.t.Fatal(err)
	}
	return v
}

func (d *device) account() *domain.SyncAccount { return d.data().Sync.Account }

// registered is device A with a fresh account.
func (s *server) registered() *device {
	s.t.Helper()
	a := s.device("laptop")
	c := a.creds("correct horse")
	c.Invite = s.h.Invite()
	sess, err := a.svc.Register(ctx, c)
	if err != nil {
		s.t.Fatalf("register: %v", err)
	}
	a.use(sess)
	return a
}

// enrolled is another device logged in to the account and synced once.
func (s *server) enrolled(name, password string) *device {
	s.t.Helper()
	b := s.device(name)
	sess, err := b.svc.Login(ctx, b.creds(password))
	if err != nil {
		s.t.Fatalf("login: %v", err)
	}
	b.use(sess)
	b.sync()
	return b
}
