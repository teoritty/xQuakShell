package usecase_test

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/usecase"
)

// teamItem seals a record into a team vault as device B of the engine's account.
func (w *syncWorld) teamItem(cv *servertest.ClientVault, ty synctrust.RecordType, key string, v any) vaultsync.Envelope {
	w.t.Helper()
	data, _ := json.Marshal(v)
	id, err := synctrust.RecordID(w.namingKey, ty, key)
	if err != nil {
		w.t.Fatal(err)
	}
	return w.sealIn(cv, id, synctrust.Body{Type: ty, Schema: 1, Data: data})
}

func (w *syncWorld) sealIn(cv *servertest.ClientVault, id vaultsync.ID, body synctrust.Body) vaultsync.Envelope {
	w.t.Helper()
	raw, err := synctrust.EncodeBody(body)
	if err != nil {
		w.t.Fatal(err)
	}
	ver, _ := cv.State.Version(id)
	e := vaultsync.Envelope{Vault: cv.ID, Record: id, Version: ver + 1, Epoch: cv.M.Epoch(), Suite: vaultsync.Suite1,
		Author: w.b.Device, SigAlg: vaultsync.SigEd25519}
	if err := (synccrypto.Cipher{}).Seal(w.vk, &e, raw); err != nil {
		w.t.Fatal(err)
	}
	msg, _ := e.SigningBytes()
	e.Sig = ed25519.Sign(w.b.DSK, msg)
	return e
}

func (w *syncWorld) teamMeta(cv *servertest.ClientVault) vaultsync.Envelope {
	body, _ := synctrust.NewVaultMetaBody(w.namingKey)
	return w.sealIn(cv, synctrust.VaultMetaID(cv.ID), body)
}

// teamOf creates a team vault owned by owner with the engine's account as an editor, and records
// device B as a writer in it.
func (w *syncWorld) teamOf(owner *servertest.User) *servertest.ClientVault {
	cv := w.h.NewTeamVault(owner, servertest.Member{User: w.a, Role: vaultsync.RoleEditor})
	cv.Users[w.a.Account] = w.b
	return cv
}

func TestEngineListsTheTeamVaultsItBelongsTo(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.teamOf(bob)
	vaults, err := w.engine.Vaults(syncCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 2 || vaults[0] != w.cv.ID || !slices.Contains(vaults, cv.ID) {
		t.Fatalf("vaults = %v; want the personal vault first and the team vault", vaults)
	}
}

func TestTeamVaultSyncsIntoItsOwnScope(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.teamOf(bob)
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv), w.teamItem(cv, synctrust.TypeFolder, "tf", domain.ConnectionFolder{ID: "tf", Name: "Team"})))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatalf("sync team vault: %v", err)
	}
	data, _ := w.vault.GetData()
	if len(data.Folders) != 1 || data.Folders[0].VaultID != cv.ID.String() {
		t.Fatalf("folders = %+v; a team item must land in the vault stamped with its team", data.Folders)
	}
	snap, _ := synctrust.Snapshot(w.namingKey, data)
	for _, e := range snap {
		if e.Type == synctrust.TypeFolder {
			t.Fatal("the personal snapshot holds the team folder")
		}
	}
	if data.Sync == nil || len(data.Sync.Contacts) != 1 || data.Sync.Contacts[0].Account != bob.Account.String() || data.Sync.Contacts[0].Verified {
		t.Fatalf("contacts = %+v; the creator is recorded on first use, unverified", data.Sync)
	}
}

func TestSettingsInATeamVaultAreQuarantined(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.teamOf(bob)
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv),
		w.teamItem(cv, synctrust.SettingsType("terminal"), "terminal", map[string]any{"fontSize": 40})))
	r, err := w.engine.Sync(syncCtx, cv.ID)
	if err != nil {
		t.Fatalf("sync: %v; a wrong type is quarantined, not a reason to stop the vault", err)
	}
	if r.Pull.Quarantined != 1 {
		t.Fatalf("report = %+v; the settings record must be quarantined", r)
	}
}

func TestRemovalFromATeamDropsItsLocalItems(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.teamOf(bob)
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv), w.teamItem(cv, synctrust.TypeFolder, "tf", domain.ConnectionFolder{ID: "tf", Name: "Team"})))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if err := cv.AppendOps(bob, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: w.a.Account},
		vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: servertest.WrapFor(bob.Account)}); err != nil {
		t.Fatal(err)
	}
	vaults, err := w.engine.Vaults(syncCtx)
	if err != nil || slices.Contains(vaults, cv.ID) {
		t.Fatalf("vaults = %v, %v; a vault the account left is not synced", vaults, err)
	}
	data, _ := w.vault.GetData()
	if len(data.Folders) != 0 {
		t.Fatalf("folders = %+v; a removed member's device drops the team's items", data.Folders)
	}
}

// impostor serves another account's keys and log in place of one member's.
func impostor(w *syncWorld, victim vaultsync.ID, as *servertest.User) {
	keys, _ := w.h.Svc.AccountKeys(syncCtx, w.h.Principal(as.Access), as.Account)
	entries, _ := w.h.Svc.DeviceLog(syncCtx, w.h.Principal(as.Access), as.Account, 0)
	w.server.tamperMemberLog = func(account vaultsync.ID, ml synctrust.MemberLog) synctrust.MemberLog {
		if account != victim {
			return ml
		}
		return synctrust.MemberLog{Suite: keys.Suite, IdentityKey: keys.IdentityKey, EncryptionKey: keys.EncryptionKey, Entries: entries}
	}
}

func TestASubstitutedMembersKeyIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	bob, carol, mallory := w.h.Register("bob"), w.h.Register("carol"), w.h.Register("mallory")
	cv := w.h.NewTeamVault(bob, servertest.Member{User: w.a, Role: vaultsync.RoleEditor}, servertest.Member{User: carol, Role: vaultsync.RoleEditor})
	cv.Users[w.a.Account] = w.b
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv)))
	// A commit by carol makes the engine resolve her device through her own log.
	cv.Accept(carol, cv.Commit(carol, cv.Record(carol, 9)))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatalf("an honest member's commit was refused: %v", err)
	}
	cv.Accept(carol, cv.Commit(carol, cv.Record(carol, 9)))
	impostor(w, carol.Account, mallory)
	if _, err := w.engine.Sync(syncCtx, cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a member's keys must match the fingerprint their add recorded", err)
	}
}

func TestTheCreatorsKeyCannotChangeAfterFirstUse(t *testing.T) {
	w := newSyncWorld(t)
	bob, mallory := w.h.Register("bob"), w.h.Register("mallory")
	cv := w.teamOf(bob)
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv)))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	impostor(w, bob.Account, mallory)
	if _, err := w.engine.Sync(syncCtx, cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: the creator's key is held to the contact made on first use", err)
	}
}

// forgedLog is what a hostile server would serve for victim: a log under a key of its own that
// certifies the victim's real device, so every signature still verifies - only the anchor is wrong.
func forgedLog(t *testing.T, victim *servertest.User) synctrust.MemberLog {
	f := servertest.NewUser(t, "forger")
	f.Account = victim.Account
	log, err := vaultsync.NewDeviceLogState(f.Account, vaultsync.Suite1, f.AIK.Public().(ed25519.PublicKey), f.AEK)
	if err != nil {
		t.Fatal(err)
	}
	f.Log = log
	entry := f.AddDeviceEntry(t, victim.Device, victim.DSK)
	return synctrust.MemberLog{Suite: vaultsync.Suite1, IdentityKey: f.AIK.Public().(ed25519.PublicKey), EncryptionKey: f.AEK,
		Entries: []vaultsync.DeviceEntry{entry}}
}

func TestAForgedCreatorLogIsHeldToTheContact(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.teamOf(bob)
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv)))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	forged := forgedLog(t, bob)
	w.server.tamperMemberLog = func(account vaultsync.ID, ml synctrust.MemberLog) synctrust.MemberLog {
		if account == bob.Account {
			return forged
		}
		return ml
	}
	if _, err := w.engine.Sync(syncCtx, cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a log under another key, however self-consistent, contradicts the contact", err)
	}
}

func TestAMembersLogEndingInAnUnfinishedRevocationIsRefused(t *testing.T) {
	w := newSyncWorld(t)
	bob, carol := w.h.Register("bob"), w.h.Register("carol")
	cv := w.h.NewTeamVault(bob, servertest.Member{User: w.a, Role: vaultsync.RoleEditor}, servertest.Member{User: carol, Role: vaultsync.RoleEditor})
	cv.Users[w.a.Account] = w.b
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv)))
	cv.Accept(carol, cv.Commit(carol, cv.Record(carol, 9)))
	revoke := carol.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: carol.Device})
	w.server.tamperMemberLog = func(account vaultsync.ID, ml synctrust.MemberLog) synctrust.MemberLog {
		if account == carol.Account {
			ml.Entries = append(ml.Entries, revoke)
		}
		return ml
	}
	if _, err := w.engine.Sync(syncCtx, cv.ID); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a revocation must be followed by its succession", err)
	}
}

func TestLeavingATeamDropsItAndLeavesTheRotationOwed(t *testing.T) {
	w := newSyncWorld(t)
	bob := w.h.Register("bob")
	cv := w.teamOf(bob)
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv), w.teamItem(cv, synctrust.TypeFolder, "tf", domain.ConnectionFolder{ID: "tf", Name: "Team"})))
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.engine.LeaveTeam(syncCtx, cv.ID); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if data, _ := w.vault.GetData(); len(data.Folders) != 0 {
		t.Fatal("the team's items stayed on a device that left")
	}
	ops, err := w.h.Svc.Membership(syncCtx, w.h.Principal(bob.Access), cv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The leave is signed by the engine's own device, A.
	cv.Users[w.a.Account] = w.a
	cv.OpsSoFar = nil
	cv.Learn(ops...)
	if cv.M.Role(w.a.Account) != vaultsync.RoleNone || !cv.M.PendingRotation() {
		t.Fatal("the leave did not remove the account with the rotation left owed")
	}
}

// A member who is not an admin leaves the team's upkeep to the admins: it neither rotates after
// another member's device is revoked nor rewraps, which the server would refuse anyway.
func TestAnEditorLeavesTeamUpkeepToTheAdmins(t *testing.T) {
	w := newSyncWorld(t)
	bob, carol := w.h.Register("bob"), w.h.Register("carol")
	cv := w.h.NewTeamVault(bob, servertest.Member{User: w.a, Role: vaultsync.RoleEditor}, servertest.Member{User: carol, Role: vaultsync.RoleEditor})
	cv.Users[w.a.Account] = w.b
	cv.Accept(w.b, cv.Commit(w.b, w.teamMeta(cv)))
	succeed(t, w.h, carol, secondDevice(t, w.h, carol))
	before, _ := w.h.Svc.Membership(syncCtx, w.h.Principal(bob.Access), cv.ID, 0)
	if _, err := w.engine.Sync(syncCtx, cv.ID); err != nil {
		t.Fatalf("an editor's sync failed: %v", err)
	}
	engine := usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: w.server, State: w.state, Vault: w.vault,
		Keys: testKeys{u: w.a, vk: w.vk}, Cipher: synccrypto.Cipher{}, Crypto: synccrypto.Account{}, NowUnixMilli: func() int64 { return 42 }})
	if err := engine.RewrapCurrent(syncCtx, cv.ID, w.a.AEK); err != nil {
		t.Fatalf("an editor's rewrap: %v; it must step aside, not fail", err)
	}
	after, _ := w.h.Svc.Membership(syncCtx, w.h.Principal(bob.Access), cv.ID, 0)
	if len(after) != len(before) {
		t.Fatal("an editor appended a membership op")
	}
}
