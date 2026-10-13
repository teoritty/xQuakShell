package usecase_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
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

// svcServer is the sync server's use cases behind the client port, so engine tests run against the
// real server logic without HTTP. The transport has its own tests; this is the engine's.
type svcServer struct {
	h     *servertest.Harness
	token string
	// tamper, when set, rewrites every page of changes - a hostile server.
	tamper    func([]synctrust.RemoteCommit) []synctrust.RemoteCommit
	tamperLog func([]vaultsync.DeviceEntry) []vaultsync.DeviceEntry
	tamperOps func([]vaultsync.MemberOp) []vaultsync.MemberOp
	// alwaysMore makes every page claim more commits follow - a server stalling a client in a loop.
	alwaysMore bool
	calls      int
}

func (s *svcServer) p() srvusecase.Principal { return s.h.Principal(s.token) }

func (s *svcServer) Vaults(ctx context.Context) ([]synctrust.RemoteVault, error) { return nil, nil }

func (s *svcServer) CreateVault(ctx context.Context, op vaultsync.MemberOp) error {
	return s.h.Svc.CreateVault(ctx, s.p(), op)
}

func (s *svcServer) Changes(ctx context.Context, vault vaultsync.ID, since uint64, limit int) ([]synctrust.RemoteCommit, bool, error) {
	if s.calls++; s.calls > 50 {
		return nil, false, errors.New("the client keeps asking: it is looping")
	}
	commits, more, err := s.h.Svc.Changes(ctx, s.p(), vault, since, limit)
	if err != nil {
		return nil, false, err
	}
	more = more || s.alwaysMore
	out := make([]synctrust.RemoteCommit, 0, len(commits))
	for _, c := range commits {
		out = append(out, synctrust.RemoteCommit{Seq: c.Seq, Commit: c.Commit})
	}
	if s.tamper != nil {
		out = s.tamper(out)
	}
	return out, more, nil
}

func (s *svcServer) Snapshot(ctx context.Context, vault vaultsync.ID) (synctrust.RemoteSnapshot, error) {
	return synctrust.RemoteSnapshot{}, errors.New("not used")
}

func (s *svcServer) Commit(ctx context.Context, c vaultsync.Commit) (uint64, error) {
	return s.h.Svc.Commit(ctx, s.p(), c.Head.Vault, c.Head.Seq-1, c)
}

func (s *svcServer) DeviceLog(ctx context.Context, account vaultsync.ID, since uint64) ([]vaultsync.DeviceEntry, error) {
	entries, err := s.h.Svc.DeviceLog(ctx, s.p(), account, since)
	if err == nil && s.tamperLog != nil {
		entries = s.tamperLog(entries)
	}
	return entries, err
}

func (s *svcServer) AppendDeviceLog(ctx context.Context, entries []vaultsync.DeviceEntry) error {
	return s.h.Svc.AppendDeviceLog(ctx, s.p(), entries)
}

func (s *svcServer) Membership(ctx context.Context, vault vaultsync.ID, since uint64) ([]vaultsync.MemberOp, error) {
	ops, err := s.h.Svc.Membership(ctx, s.p(), vault, since)
	if err == nil && s.tamperOps != nil {
		ops = s.tamperOps(ops)
	}
	return ops, err
}

func (s *svcServer) AppendMembership(ctx context.Context, vault vaultsync.ID, ops []vaultsync.MemberOp) error {
	return s.h.Svc.AppendMembership(ctx, s.p(), vault, ops)
}

// testKeys is a KeyRing with one fixed vault key: the server fixtures wrap no real keys, and the
// unwrapping has its own tests.
type testKeys struct {
	u  *servertest.User
	vk []byte
}

func (k testKeys) Account() vaultsync.ID { return k.u.Account }
func (k testKeys) Device() vaultsync.ID  { return k.u.Device }
func (k testKeys) AccountKeys() (vaultsync.SuiteID, []byte, []byte) {
	return vaultsync.Suite1, k.u.AIK.Public().(ed25519.PublicKey), k.u.AEK
}
func (k testKeys) VaultKey(vaultsync.ID, uint32, []vaultsync.MemberOp) ([]byte, error) {
	return k.vk, nil
}
func (k testKeys) Sign(msg []byte) ([]byte, error) { return ed25519.Sign(k.u.DSK, msg), nil }

type syncTestVault struct {
	mu   sync.Mutex
	data *domain.VaultData
}

func (v *syncTestVault) Exists() bool                                       { return true }
func (v *syncTestVault) Create(context.Context, string) error               { return nil }
func (v *syncTestVault) Unlock(context.Context, string) error               { return nil }
func (v *syncTestVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (v *syncTestVault) Lock()                                              {}
func (v *syncTestVault) IsUnlocked() bool                                   { return true }
func (v *syncTestVault) GetData() (*domain.VaultData, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	raw, _ := json.Marshal(v.data)
	var out domain.VaultData
	_ = json.Unmarshal(raw, &out)
	return &out, nil
}
func (v *syncTestVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
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

var syncCtx = context.Background()

// syncWorld is one account with two devices: B writes through the reference client the server
// tests use, A runs the engine under test.
type syncWorld struct {
	t         *testing.T
	h         *servertest.Harness
	a, b      *servertest.User
	cv        *servertest.ClientVault
	vk        []byte
	namingKey []byte
	server    *svcServer
	state     *syncstate.DB
	vault     *syncTestVault
	engine    *usecase.SyncEngine
}

func newSyncWorld(t *testing.T) *syncWorld {
	t.Helper()
	h := servertest.NewHarness(t, 0)
	a := h.Register("alice")
	_, dskB, _ := ed25519.GenerateKey(rand.Reader)
	b := *a
	b.Device, b.DSK = servertest.RandomID(t), dskB
	if err := h.Svc.AppendDeviceLog(syncCtx, h.Principal(a.Access), []vaultsync.DeviceEntry{a.AddDeviceEntry(t, b.Device, dskB)}); err != nil {
		t.Fatal(err)
	}
	b.Access = h.DeviceLogin(a, b.Device, dskB)
	st, err := syncstate.Open(syncCtx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	w := &syncWorld{t: t, h: h, a: a, b: &b, vk: bytes.Repeat([]byte{0x5a}, 32), namingKey: bytes.Repeat([]byte{0x4e}, 32),
		state: st, vault: &syncTestVault{data: domain.NewVaultData()}}
	w.cv = h.PersonalVault(a)
	w.cv.Users[a.Account] = w.b
	w.server = &svcServer{h: h, token: a.Access}
	w.engine = usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: w.server, State: st, Vault: w.vault,
		Keys: testKeys{u: a, vk: w.vk}, Cipher: synccrypto.Cipher{}, NowUnixMilli: func() int64 { return 42 }})
	return w
}

// sealed is a live record version written by device B, filed under id.
func (w *syncWorld) sealed(id vaultsync.ID, body synctrust.Body) vaultsync.Envelope {
	w.t.Helper()
	raw, err := synctrust.EncodeBody(body)
	if err != nil {
		w.t.Fatal(err)
	}
	return w.sealedRaw(id, raw)
}

// sealedRaw seals arbitrary plaintext, including bytes that are not a record body at all.
func (w *syncWorld) sealedRaw(id vaultsync.ID, raw []byte) vaultsync.Envelope {
	w.t.Helper()
	v, _ := w.cv.State.Version(id)
	e := vaultsync.Envelope{Vault: w.cv.ID, Record: id, Version: v + 1, Epoch: w.cv.M.Epoch(), Suite: vaultsync.Suite1,
		Author: w.b.Device, SigAlg: vaultsync.SigEd25519}
	if err := (synccrypto.Cipher{}).Seal(w.vk, &e, raw); err != nil {
		w.t.Fatal(err)
	}
	return w.signed(e)
}

func (w *syncWorld) signed(e vaultsync.Envelope) vaultsync.Envelope {
	msg, _ := e.SigningBytes()
	e.Sig = ed25519.Sign(w.b.DSK, msg)
	return e
}

func (w *syncWorld) tombstone(id vaultsync.ID) vaultsync.Envelope {
	v, _ := w.cv.State.Version(id)
	return w.signed(vaultsync.Envelope{Vault: w.cv.ID, Record: id, Version: v + 1, Epoch: w.cv.M.Epoch(), Suite: vaultsync.Suite1,
		Deleted: true, Author: w.b.Device, SigAlg: vaultsync.SigEd25519})
}

func (w *syncWorld) item(ty synctrust.RecordType, key string, v any) vaultsync.Envelope {
	w.t.Helper()
	data, _ := json.Marshal(v)
	id, err := synctrust.RecordID(w.namingKey, ty, key)
	if err != nil {
		w.t.Fatal(err)
	}
	return w.sealed(id, synctrust.Body{Type: ty, Schema: 1, Data: data})
}

func (w *syncWorld) meta() vaultsync.Envelope {
	body, _ := synctrust.NewVaultMetaBody(w.namingKey)
	return w.sealed(synctrust.VaultMetaID(w.cv.ID), body)
}

func (w *syncWorld) push(records ...vaultsync.Envelope) {
	w.t.Helper()
	w.cv.Accept(w.b, w.cv.Commit(w.b, records...))
}

func (w *syncWorld) pull() usecase.PullReport {
	w.t.Helper()
	r, err := w.engine.Pull(syncCtx, w.cv.ID)
	if err != nil {
		w.t.Fatalf("pull: %v", err)
	}
	return r
}

func TestPullAppliesAnotherDevicesChanges(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}),
		w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", FolderID: "f1", Name: "web", Host: "web.example", Port: 22}))
	r := w.pull()
	if r.Commits != 1 || r.Applied != 2 {
		t.Fatalf("report = %+v", r)
	}
	d, _ := w.vault.GetData()
	if len(d.Folders) != 1 || len(d.Connections) != 1 || d.Connections[0].Host != "web.example" {
		t.Fatalf("vault = %+v", d)
	}
	sv, _ := w.state.LoadVault(syncCtx, w.cv.ID)
	if sv.Head == nil || sv.Head.Seq != 1 || len(sv.Records) != 3 {
		t.Fatalf("state = %+v; the cursor must advance with what was applied", sv)
	}
	if again := w.pull(); again.Commits != 0 {
		t.Fatalf("a second pull re-applied %d commits", again.Commits)
	}
}

func TestPullIsIncremental(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	w.pull()
	w.push(w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Production"}))
	w.push(w.item(synctrust.TypeFolder, "f2", domain.ConnectionFolder{ID: "f2", Name: "Dev"}))
	if r := w.pull(); r.Commits != 2 || r.Applied != 2 {
		t.Fatalf("report = %+v", r)
	}
	d, _ := w.vault.GetData()
	if len(d.Folders) != 2 || d.Folders[0].Name != "Production" {
		t.Fatalf("folders = %+v", d.Folders)
	}
}

func TestPullAppliesDeletions(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	id, _ := synctrust.RecordID(w.namingKey, synctrust.TypeFolder, "f1")
	w.push(w.tombstone(id))
	w.pull()
	if d, _ := w.vault.GetData(); len(d.Folders) != 0 {
		t.Fatalf("a tombstone did not delete: %+v", d.Folders)
	}
}

func quarantined(t *testing.T, w *syncWorld) []synctrust.QuarantineItem {
	t.Helper()
	items, err := w.state.Quarantined(syncCtx, w.cv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestRecordUnderTheWrongIdIsQuarantined(t *testing.T) {
	w := newSyncWorld(t)
	data, _ := json.Marshal(domain.ConnectionFolder{ID: "f1", Name: "Prod"})
	other, _ := synctrust.RecordID(w.namingKey, synctrust.TypeFolder, "f2")
	w.push(w.meta(), w.sealed(other, synctrust.Body{Type: synctrust.TypeFolder, Schema: 1, Data: data}))
	r := w.pull()
	if r.Quarantined != 1 || r.Applied != 0 {
		t.Fatalf("report = %+v", r)
	}
	if d, _ := w.vault.GetData(); len(d.Folders) != 0 {
		t.Fatal("a record filed under another item's id was applied")
	}
}

func TestRecordsBeforeTheNamingKeyAreQuarantined(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	if r := w.pull(); r.Quarantined != 1 {
		t.Fatalf("report = %+v; without the naming key no id can be checked", r)
	}
}

func TestInvalidAndUnknownRecordsAreQuarantined(t *testing.T) {
	w := newSyncWorld(t)
	newer, _ := synctrust.RecordID(w.namingKey, synctrust.TypeFolder, "f9")
	unknown, _ := synctrust.RecordID(w.namingKey, "gizmo", "g")
	w.push(w.meta(),
		w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Port: 70000}),
		w.sealed(newer, synctrust.Body{Type: synctrust.TypeFolder, Schema: 2, Data: json.RawMessage(`{"id":"f9","name":"x","parentId":"","order":0}`)}),
		w.sealed(unknown, synctrust.Body{Type: "gizmo", Schema: 1, Data: json.RawMessage(`{}`)}),
		w.sealedRaw(servertest.RandomID(t), []byte(`not json`)))
	r := w.pull()
	if r.Quarantined != 4 || r.Applied != 0 {
		t.Fatalf("report = %+v", r)
	}
	if d, _ := w.vault.GetData(); len(d.Connections) != 0 {
		t.Fatal("a connection that fails the domain validator was applied")
	}
}

func TestWeakeningChangesAreHeld(t *testing.T) {
	w := newSyncWorld(t)
	s := *domain.NewVaultData().Settings
	s.Plugins.RequireSignedPlugins = false
	sections, _ := synctrust.SplitSettings(s)
	id, _ := synctrust.RecordID(w.namingKey, synctrust.SettingsType("plugins"), "plugins")
	w.push(w.meta(), w.sealed(id, synctrust.Body{Type: synctrust.SettingsType("plugins"), Schema: 1, Data: sections["plugins"]}))
	r := w.pull()
	if r.Held != 1 || r.Applied != 0 {
		t.Fatalf("report = %+v", r)
	}
	if d, _ := w.vault.GetData(); !d.Settings.Plugins.RequireSignedPlugins {
		t.Fatal("another device turned off plugin signing without the master password")
	}
	if items := quarantined(t, w); len(items) != 1 || !items[0].Pending || items[0].Reason == "" {
		t.Fatalf("held items = %+v", items)
	}
}

func TestClearingNonExportableIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	w.vault.data.Identities["k1"] = domain.SSHIdentity{ID: "k1", NonExportable: true}
	w.push(w.meta(), w.item(synctrust.TypeIdentity, "k1", map[string]any{"identity": domain.SSHIdentity{ID: "k1"}}))
	r := w.pull()
	if r.Quarantined != 1 || r.Held != 0 {
		t.Fatalf("report = %+v", r)
	}
	if d, _ := w.vault.GetData(); !d.Identities["k1"].NonExportable {
		t.Fatal("the non-exportable latch was cleared from another device")
	}
	if items := quarantined(t, w); len(items) != 1 || items[0].Pending {
		t.Fatalf("items = %+v; a refused change is not confirmable", items)
	}
}

func TestANewerVersionReleasesTheQuarantinedOne(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta(), w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Port: 70000}))
	w.pull()
	w.push(w.item(synctrust.TypeConnection, "c1", domain.Connection{ID: "c1", Port: 22}))
	w.pull()
	if items := quarantined(t, w); len(items) != 0 {
		t.Fatalf("quarantine = %+v; an applied newer version supersedes it", items)
	}
}

func assertUntouched(t *testing.T, w *syncWorld, before *domain.VaultData, seq uint64) {
	t.Helper()
	after, _ := w.vault.GetData()
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("the vault changed during a pull that failed verification")
	}
	sv, _ := w.state.LoadVault(syncCtx, w.cv.ID)
	if got := uint64(0); sv.Head != nil {
		got = sv.Head.Seq
		if got != seq {
			t.Fatalf("state advanced to %d, want %d", got, seq)
		}
	} else if seq != 0 {
		t.Fatalf("state lost its head, want %d", seq)
	}
}

func TestHostileServerIsStopped(t *testing.T) {
	for name, tamper := range map[string]func([]synctrust.RemoteCommit) []synctrust.RemoteCommit{
		"drop a record": func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
			cs[0].Commit.Records = cs[0].Commit.Records[:1]
			return cs
		},
		"flip a body bit": func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
			cs[0].Commit.Records[1].Body[10] ^= 1
			return cs
		},
		"rewrite the head": func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
			cs[0].Commit.Head.MerkleRoot[0] ^= 1
			return cs
		},
		"skip a commit": func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit { return cs[1:] },
	} {
		t.Run(name, func(t *testing.T) {
			w := newSyncWorld(t)
			w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
			w.push(w.item(synctrust.TypeFolder, "f2", domain.ConnectionFolder{ID: "f2", Name: "Dev"}))
			before, _ := w.vault.GetData()
			w.server.tamper = tamper
			if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
				t.Fatalf("err = %v, want ErrIntegrity", err)
			}
			assertUntouched(t, w, before, 0)
		})
	}
}

func TestVaultMetaCannotChange(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	w.pull()
	before, _ := w.vault.GetData()
	w.namingKey = bytes.Repeat([]byte{1}, 32)
	w.push(w.meta())
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; a new naming key would orphan every record id", err)
	}
	assertUntouched(t, w, before, 1)
}

func TestCommitByAnUncertifiedDeviceIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	w.server.tamper = func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
		for i := range cs {
			h := &cs[i].Commit.Head
			msg, _ := h.SigningBytes()
			h.Sig = ed25519.Sign(stranger, msg)
		}
		return cs
	}
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestAPageThatAlwaysClaimsMoreDoesNotLoop(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	w.server.alwaysMore = true
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); err != nil {
		t.Fatalf("pull: %v; an empty page must end the pull even when it claims more", err)
	}
}

func TestForgedDeviceLogIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	w.server.tamperLog = func(es []vaultsync.DeviceEntry) []vaultsync.DeviceEntry {
		forged := es[len(es)-1]
		forged.Seq++
		forged.Cert = &vaultsync.DeviceCert{Device: servertest.RandomID(t), SigningKey: stranger.Public().(ed25519.PublicKey), Name: "evil", Platform: "x"}
		return append(es, forged)
	}
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; a device the account key never certified must not join", err)
	}
	if stored, _ := w.state.DeviceLog(syncCtx, w.a.Account); len(stored) != 0 {
		t.Fatal("a device log that failed verification was stored")
	}
}

// A revocation without its succession leaves the old account keys in force; the server refuses
// such a batch, so a log that ends in one was cut short on purpose.
func TestDeviceLogEndingInARevocationIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	revoke := w.a.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: w.b.Device})
	w.server.tamperLog = func(es []vaultsync.DeviceEntry) []vaultsync.DeviceEntry { return append(es, revoke) }
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestForgedMembershipIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	w.server.tamperOps = func(ops []vaultsync.MemberOp) []vaultsync.MemberOp {
		ops[0].RetentionDays++
		return ops
	}
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	if stored, _ := w.state.Membership(syncCtx, w.cv.ID); len(stored) != 0 {
		t.Fatal("a membership log that failed verification was stored")
	}
}

func TestHeadBeyondTheMembershipLogIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	w.server.tamper = func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
		cs[0].Commit.Head.MembershipSeq = 9
		return cs
	}
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}
