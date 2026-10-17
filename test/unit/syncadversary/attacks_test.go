package syncadversary

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	srvusecase "xquakshell/internal/syncserver/usecase"
	"xquakshell/internal/usecase"
)

var errorsIs = errors.Is

func changes(edit func(*syncwire.Changes)) rewrite { return on(suffix("/changes"), edit) }

// twoCommits has A push a commit of two records and then a commit of one.
func twoCommits(a *device) {
	a.addFolder("f2", "Dev")
	a.addFolder("f3", "QA")
	a.sync()
	a.addFolder("f4", "Ops")
	a.sync()
}

func TestDropRecord(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(changes(func(c *syncwire.Changes) {
		if len(c.Commits) > 0 && len(c.Commits[0].Records) > 1 {
			c.Commits[0].Records = c.Commits[0].Records[:1]
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}

func TestReorderCommits(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(changes(func(c *syncwire.Changes) {
		if len(c.Commits) == 2 {
			c.Commits[0], c.Commits[1] = c.Commits[1], c.Commits[0]
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}

// A replay serves a commit the device already verified, in place of what follows its head.
func TestReplayCommit(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	seen := b.head()
	w.attack(func(r *http.Request, status int, body []byte) (int, []byte) {
		if !strings.HasSuffix(r.URL.Path, "/changes") || status != http.StatusOK {
			return status, body
		}
		p := w.h.Principal(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		old, _, _ := w.h.Svc.Changes(ctx, p, b.session.Personal, seen-1, 1)
		return status, encodeChanges(old)
	})
	expectRefused(t, b, synctrust.ErrIntegrity)
}

func encodeChanges(commits []srvusecase.StoredCommit) []byte {
	var out syncwire.Changes
	for _, c := range commits {
		e := syncwire.CommitEntry{Seq: c.Seq, Head: syncwire.HeadFrom(&c.Commit.Head)}
		for i := range c.Commit.Records {
			e.Records = append(e.Records, syncwire.EnvelopeFrom(&c.Commit.Records[i]))
		}
		out.Commits = append(out.Commits, e)
	}
	raw, _ := json.Marshal(out)
	return raw
}

// A server that answers "changes since my head" with the vault's history from the start is serving
// an older head; the chain does not link to what the device verified.
func TestRollbackHead(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(func(r *http.Request, status int, body []byte) (int, []byte) {
		if !strings.HasSuffix(r.URL.Path, "/changes") || status != http.StatusOK {
			return status, body
		}
		p := w.h.Principal(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		commits, _, _ := w.h.Svc.Changes(ctx, p, b.session.Personal, 0, 100)
		return status, encodeChanges(commits)
	})
	expectRefused(t, b, synctrust.ErrIntegrity)
}

// Moving a body to another record is refused before decryption: the author's signature covers the
// body. A body that kept its signature would still fail the AEAD bound to the record's coordinates.
func TestSwapCiphertext(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(changes(func(c *syncwire.Changes) {
		if len(c.Commits) > 0 && len(c.Commits[0].Records) > 1 {
			r := c.Commits[0].Records
			r[0].Body, r[1].Body = r[1].Body, r[0].Body
			r[0].WrappedDEK, r[1].WrappedDEK = r[1].WrappedDEK, r[0].WrappedDEK
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}

func TestSwapEpoch(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(changes(func(c *syncwire.Changes) {
		if len(c.Commits) > 0 {
			c.Commits[0].Records[0].Epoch++
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}

// Cutting a page short mid-commit breaks the commit's root. A page cut between commits and claimed
// complete only leaves the device behind, consistently: it is caught by the next pull, or by the
// fork check of a later phase, never by applying anything wrong.
func TestTruncatePage(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(changes(func(c *syncwire.Changes) {
		if len(c.Commits) > 0 {
			last := &c.Commits[len(c.Commits)-1]
			if len(last.Records) > 1 {
				last.Records = last.Records[:len(last.Records)-1]
			} else {
				c.Commits[0].Records = c.Commits[0].Records[:1]
			}
			c.More = false
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}

func TestBadSnapshot(t *testing.T) {
	w, a, _ := pair(t)
	twoCommits(a)
	w.attack(on(suffix("/snapshot"), func(s *syncwire.Snapshot) {
		if len(s.Records) > 1 {
			s.Records = s.Records[1:]
		}
	}))
	c := w.device("tablet")
	s, err := c.svc.Login(ctx, c.creds())
	if err != nil {
		t.Fatal(err)
	}
	c.session, c.engine = s, c.newEngine(s)
	before := c.vault.snapshot()
	if err := c.engine.EnsureVaultMeta(ctx, s.Personal, rand.Reader); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	if !bytes.Equal(before, c.vault.snapshot()) || c.head() != 0 {
		t.Fatal("a refused snapshot changed the new device")
	}
}

func TestOtherBundle(t *testing.T) {
	w, _, _ := pair(t)
	stranger := w.h.Register("mallory")
	strangerBundle := []byte("sealed-bundle")
	_ = stranger
	w.attack(on(suffix("/login/finish"), func(r *syncwire.LoginFinishReply) { r.Bundle = strangerBundle }))
	c := w.device("tablet")
	if _, err := c.svc.Login(ctx, c.creds()); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; another account's bundle must not open", err)
	}
	if c.vault.data.Sync != nil {
		t.Fatal("a refused login stored something")
	}
}

func TestOversizeAndDeepReplies(t *testing.T) {
	for name, body := range map[string][]byte{
		"oversize": []byte(`{"entries":[],"pad":"` + strings.Repeat("x", 2<<20) + `"}`),
		"deep":     []byte(`{"entries":` + strings.Repeat("[", 50_000) + strings.Repeat("]", 50_000) + `}`),
	} {
		t.Run(name, func(t *testing.T) {
			w, _, b := pair(t)
			w.attack(func(r *http.Request, status int, _ []byte) (int, []byte) {
				if strings.HasSuffix(r.URL.Path, "/devicelog") && r.Method == http.MethodGet {
					return http.StatusOK, body
				}
				return status, nil
			})
			_, err := b.engine.Sync(ctx, b.session.Personal)
			if !errors.Is(err, synctrust.ErrRemoteProtocol) {
				t.Fatalf("err = %v, want ErrRemoteProtocol", err)
			}
		})
	}
}

func TestSuiteDowngradeInHello(t *testing.T) {
	w, _, _ := pair(t)
	w.attack(on(suffix("/hello"), func(r *syncwire.HelloReply) { r.Suites = []uint16{0} }))
	c := w.device("tablet")
	if _, err := c.svc.Probe(ctx, w.url); !errors.Is(err, synctrust.ErrRemoteUnsupported) {
		t.Fatalf("err = %v; a weaker suite must be refused before any work", err)
	}
}

func TestSuiteDowngradeInARecord(t *testing.T) {
	w, a, b := pair(t)
	twoCommits(a)
	w.attack(changes(func(c *syncwire.Changes) {
		if len(c.Commits) > 0 {
			c.Commits[0].Records[0].Suite = 0
		}
	}))
	before := b.vault.snapshot()
	if _, err := b.engine.Sync(ctx, b.session.Personal); err == nil {
		t.Fatal("a record of a lesser suite was accepted")
	}
	if !bytes.Equal(before, b.vault.snapshot()) {
		t.Fatal("the vault changed")
	}
}

// forged builds a commit on top of what d verified, signed by signer as device author, with the
// record and head at the given epochs - what a revoked or malicious device could produce.
func forged(t *testing.T, d *device, signer ed25519.PrivateKey, author vaultsync.ID, recordEpoch uint32, headEpoch func(uint32) uint32) syncwire.CommitEntry {
	t.Helper()
	sv, _ := d.state.LoadVault(ctx, d.session.Personal)
	st, err := vaultsync.RestoreVaultState(d.session.Personal, sv.Head, sv.Records)
	if err != nil {
		t.Fatal(err)
	}
	ops, _ := d.session.Server.Membership(ctx, d.session.Personal, 0)
	ms, _ := vaultsync.NewMembershipState(d.session.Personal)
	for i := range ops {
		_ = ms.Apply(&ops[i], func(vaultsync.ID, vaultsync.ID, vaultsync.SigAlg, []byte, []byte) error { return nil })
	}
	rid, _ := vaultsync.NewID(rand.Reader)
	env := vaultsync.Envelope{Vault: d.session.Personal, Record: rid, Version: 1, Epoch: recordEpoch, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{1}, 72), Body: bytes.Repeat([]byte{2}, 296), Author: author, SigAlg: vaultsync.SigEd25519}
	msg, _ := env.SigningBytes()
	env.Sig = ed25519.Sign(signer, msg)
	leaves := st.Leaves()
	l, _ := vaultsync.LeafOf(&env)
	root, _ := vaultsync.MerkleRoot(append(leaves, l))
	cid, _ := vaultsync.NewID(rand.Reader)
	cur := st.Cursor()
	h := vaultsync.Head{Vault: d.session.Personal, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: root, CurrentEpoch: headEpoch(ms.Epoch()),
		MembershipSeq: ms.Seq(), MembershipHead: ms.Hash(), Commit: cid, Author: author, ClientTimeUnixMilli: 1, SigAlg: vaultsync.SigEd25519}
	msg, _ = h.SigningBytes()
	h.Sig = ed25519.Sign(signer, msg)
	return syncwire.CommitEntry{Seq: h.Seq, Head: syncwire.HeadFrom(&h), Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&env)}}
}

func (d *device) keys() (ed25519.PrivateKey, vaultsync.ID, *domain.SyncAccount) {
	acct := d.vault.data.Sync.Account
	return ed25519.NewKeyFromSeed(acct.DeviceSeed), vaultsync.ID(acct.Device), acct
}

func revoked(t *testing.T) (*world, *device, *device) {
	t.Helper()
	w, a, b := pair(t)
	s, err := a.svc.RevokeDevices(ctx, []vaultsync.ID{b.session.Keys.Device()}, []byte("correct horse"))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	a.session, a.engine = s, a.newEngine(s)
	a.sync()
	return w, a, b
}

func inject(w *world, entry syncwire.CommitEntry) {
	w.attack(changes(func(c *syncwire.Changes) { c.Commits = append(c.Commits, entry) }))
}

func TestRevokedSigner(t *testing.T) {
	w, a, b := revoked(t)
	bKey, bDevice, _ := b.keys()
	inject(w, forged(t, a, bKey, bDevice, 1, func(e uint32) uint32 { return e }))
	expectRefused(t, a, synctrust.ErrIntegrity)
}

func TestStaleEpochWrite(t *testing.T) {
	w, a, _ := revoked(t)
	aKey, aDevice, _ := a.keys()
	inject(w, forged(t, a, aKey, aDevice, 0, func(e uint32) uint32 { return e }))
	expectRefused(t, a, synctrust.ErrIntegrity)
}

// The revoked device still holds the old identity key; a succession it signs with it is refused
// wherever it appears, here slipped into the device log by the server.
func TestSupersededSuccession(t *testing.T) {
	w, a, b := revoked(t)
	_, _, oldAcct := b.keys()
	oldAIK := ed25519.NewKeyFromSeed(oldAcct.IdentitySeed)
	// The forged entry extends the log exactly where A's verified copy ends, so the only thing
	// wrong with it is the key that signed it.
	_, _, aAcct := a.keys()
	account := vaultsync.ID(aAcct.Account)
	stored, _ := a.state.DeviceLog(ctx, account)
	log, _ := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, aAcct.GenesisIdentityKey, aAcct.GenesisEncryptionKey)
	for i := range stored {
		if err := log.Apply(&stored[i]); err != nil {
			t.Fatal(err)
		}
	}
	_, newAIK, _ := ed25519.GenerateKey(rand.Reader)
	e := vaultsync.DeviceEntry{Account: account, Seq: log.Seq() + 1, Prev: log.Hash(), Op: vaultsync.OpSuccession, NewSuite: vaultsync.Suite1,
		NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: oldAcct.EncryptionKey, SigAlg: vaultsync.SigEd25519}
	msg, _ := e.SigningBytes()
	e.Sig, e.SuccessorSig = ed25519.Sign(oldAIK, msg), ed25519.Sign(newAIK, msg)
	if err := log.Apply(&e); !errors.Is(err, vaultsync.ErrSupersededKey) {
		t.Fatalf("the forged entry fails with %v, want ErrSupersededKey: the test must exercise the key, not the chain", err)
	}
	w.attack(on(suffix("/devicelog"), func(l *syncwire.DeviceLog) {
		l.Entries = append(l.Entries, syncwire.DeviceEntryFrom(&e))
	}))
	expectRefused(t, a, synctrust.ErrIntegrity)
}

// Malicious device: B's own engine pushes whatever B's vault holds. What weakens A waits for A's
// master password or is refused outright.
func TestMaliciousDeviceChanges(t *testing.T) {
	_, a, b := pair(t)
	_ = a.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		v.Identities["k1"] = domain.SSHIdentity{ID: "k1", KeyType: "ed25519", NonExportable: true}
		v.KnownHosts = append(v.KnownHosts, "web.example ssh-ed25519 AAAAgood")
		return nil
	})
	a.sync()
	b.sync()
	_ = b.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		id := v.Identities["k1"]
		id.NonExportable = false
		v.Identities["k1"] = id
		v.KnownHosts = []string{"web.example ssh-ed25519 AAAAevil"}
		v.Settings.Plugins.RequireSignedPlugins = false
		return nil
	})
	b.sync()
	r, err := a.engine.Sync(ctx, a.session.Personal)
	if err != nil {
		t.Fatal(err)
	}
	if r.Pull.Held != 2 || r.Pull.Quarantined != 1 {
		t.Fatalf("report = %+v; want the host key and plugin trust held, the exportability refused", r.Pull)
	}
	got, _ := a.vault.GetData()
	if !got.Identities["k1"].NonExportable || got.KnownHosts[0] != "web.example ssh-ed25519 AAAAgood" || !got.Settings.Plugins.RequireSignedPlugins {
		t.Fatal("a weakening change from another device reached the vault")
	}
}

var _ = usecase.ErrSyncLogin
