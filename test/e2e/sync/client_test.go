//go:build e2e_sync

package synce2e

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/opaque"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/syncclient"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/infra/syncstate"
	"xquakshell/internal/infra/vault"
	"xquakshell/internal/usecase"
)

const masterPassword = "master password of this device"

// device is one client stack over its own data root, composed as main_sync.go composes it.
type device struct {
	t       *testing.T
	s       *server
	dir     string
	vault   *persistence.VaultRepo
	state   *syncstate.DB
	svc     *usecase.SyncAccountService
	session usecase.SyncSession
	engine  *usecase.SyncEngine
}

func now() int64 { return time.Now().UnixMilli() }

func (s *server) device(name string) *device {
	s.t.Helper()
	d := &device{t: s.t, s: s, dir: s.t.TempDir()}
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
		NewAKE: func(helloContext []byte) (vaultsync.PasswordAKEClient, error) { return opaque.NewClient(helloContext) },
		Crypto: synccrypto.Account{}, Random: rand.Reader, NowUnixMilli: now, DeviceName: name, Platform: "e2e"})
	return d
}

func (d *device) newEngine(s usecase.SyncSession) *usecase.SyncEngine {
	return usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: s.Server, State: d.state, Vault: d.vault, Keys: s.Keys,
		Cipher: synccrypto.Cipher{}, Crypto: synccrypto.Account{}, NowUnixMilli: now})
}

// creds probes the server and pins its identity only after the fingerprint matches the one the
// operator's `xqsyncd fingerprint` printed - the comparison a user makes on first connection.
func (d *device) creds(username, password string) usecase.SyncCredentials {
	d.t.Helper()
	probe, err := d.svc.Probe(ctx, d.s.url)
	if err != nil {
		d.t.Fatalf("probe: %v", err)
	}
	if probe.Fingerprint != d.s.fingerprint {
		d.t.Fatalf("the server's fingerprint %q is not the operator's %q", probe.Fingerprint, d.s.fingerprint)
	}
	return usecase.SyncCredentials{ServerURL: d.s.url, Pin: probe.IdentityKey, Username: username, Password: []byte(password)}
}

func (d *device) use(sess usecase.SyncSession) {
	d.t.Helper()
	d.session, d.engine = sess, d.newEngine(sess)
	if err := retry(func() error { return d.engine.EnsureVaultMeta(ctx, sess.Personal, rand.Reader) }); err != nil {
		d.t.Fatalf("vault meta: %v", err)
	}
}

// retry repeats op while the server answers that the client is going too fast. Every client of a
// test arrives from 127.0.0.1, so the per-address limit is shared; the real scheduler waits the
// same Retry-After.
func retry(op func() error) error {
	for range 20 {
		err := op()
		var limited *synctrust.RateLimitedError
		if !errors.As(err, &limited) {
			return err
		}
		time.Sleep(limited.RetryAfter)
	}
	return errors.New("still rate limited after 20 attempts")
}

func (d *device) syncVault(v vaultsync.ID) error {
	return retry(func() error {
		_, err := d.engine.Sync(ctx, v)
		return err
	})
}

func (d *device) sync() {
	d.t.Helper()
	if err := d.syncVault(d.session.Personal); err != nil {
		d.t.Fatalf("sync: %v", err)
	}
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

func (d *device) head() uint64 {
	sv, err := d.state.LoadVault(ctx, d.session.Personal)
	if err != nil || sv.Head == nil {
		return 0
	}
	return sv.Head.Seq
}

// unique keeps accounts of different runs apart on a long-lived server.
func unique(name string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return name + "-" + hex.EncodeToString(b)
}

// registered is a device with a fresh account.
func (s *server) registered(username, password string) *device {
	s.t.Helper()
	d := s.device("laptop")
	c := d.creds(username, password)
	c.Invite = s.invite()
	var sess usecase.SyncSession
	if err := retry(func() (err error) { sess, err = d.svc.Register(ctx, c); return err }); err != nil {
		s.t.Fatalf("register %s: %v", username, err)
	}
	d.use(sess)
	return d
}

// enrolled is another device of the account, logged in and synced once.
func (s *server) enrolled(name, username, password string) *device {
	s.t.Helper()
	d := s.device(name)
	c := d.creds(username, password)
	var sess usecase.SyncSession
	if err := retry(func() (err error) { sess, err = d.svc.Login(ctx, c); return err }); err != nil {
		s.t.Fatalf("login: %v", err)
	}
	d.use(sess)
	d.sync()
	return d
}
