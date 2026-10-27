package usecase_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

// serverCommitRecords is what one commit may carry on the server: records and purges together.
const serverCommitRecords = 1000

// serverCommitBody is the server's default commit body limit (transport.Config.MaxCommitBody).
const serverCommitBody = 20 << 20

func addConnections(w *syncWorld, n int) {
	w.edit(func(d *domain.VaultData) {
		for i := range n {
			id := fmt.Sprintf("bulk-%04d", i)
			d.Connections = append(d.Connections, domain.Connection{ID: id, Name: id, Host: "h.example", Port: 22})
		}
	})
}

func folderName(w *syncWorld, id string) string {
	d, _ := w.vault.GetData()
	for _, f := range d.Folders {
		if f.ID == id {
			return f.Name
		}
	}
	return ""
}

// One change set larger than a commit may carry - an import, a bulk delete - used to go up as one
// commit the server refuses, every round, for good.
func TestALargeChangeIsCommittedInPieces(t *testing.T) {
	w := readyWorld(t)
	addConnections(w, 2*serverCommitRecords+1)
	if r := w.sync(); r.Pushed != 2*serverCommitRecords+1 {
		t.Fatalf("pushed %d, want %d", r.Pushed, 2*serverCommitRecords+1)
	}
	commits := w.catchUp()
	if len(commits) < 3 {
		t.Fatalf("%d commits for %d records; one commit cannot carry them", len(commits), 2*serverCommitRecords+1)
	}
	for i, c := range commits {
		if n := len(c.Records) + len(c.Purged); n > serverCommitRecords {
			t.Fatalf("commit %d carries %d records, over the server's %d", i, n, serverCommitRecords)
		}
	}
}

// A rotation re-encrypts every live record. The cadence is automatic, so a vault past one commit's
// worth of records stopped syncing on its owner's device within half a retention window, with nobody
// having done anything.
func TestAVaultLargerThanOneCommitRotatesOnSchedule(t *testing.T) {
	_, a := registered(t)
	ids := make([]string, 0, serverCommitRecords+200)
	for i := range serverCommitRecords + 200 {
		ids = append(ids, fmt.Sprintf("f-%04d", i))
	}
	addFolders(a, ids...)
	a.sync()
	setClocks(7+15*retentionDay, a)
	a.sync()
	if epoch := currentEpoch(t, a); epoch != 1 {
		t.Fatalf("epoch %d after half a window; the vault must rotate", epoch)
	}
	sv, err := a.state.LoadVault(syncCtx, a.session.Personal)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range sv.Records {
		if !r.Deleted && r.Epoch != 1 {
			t.Fatalf("record %s still under epoch %d; the re-encryption did not finish", r.Record, r.Epoch)
		}
	}
	addFolders(a, "after")
	a.sync()
}

func TestMoreTombstonesThanOneCommitArePurged(t *testing.T) {
	h, a := registered(t)
	ids := make([]string, 0, serverCommitRecords+200)
	for i := range serverCommitRecords + 200 {
		ids = append(ids, fmt.Sprintf("f-%04d", i))
	}
	addFolders(a, ids...)
	a.sync()
	for _, id := range ids {
		removeFolder(a, id)
	}
	a.sync()
	h.Clock.Advance(uint64(31 * retentionDay))
	setClocks(7+30*retentionDay, a)
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 0 {
		t.Fatalf("%d tombstones outlived the window", tombs)
	}
}

// A refusal that is final for a commit - a full quota, a role that no longer writes - used to keep
// the commit in the outbox, resent first on every round, so the vault stopped pulling too.
func TestARefusedCommitDoesNotStopTheVaultPulling(t *testing.T) {
	w := readyWorld(t)
	w.server.refuseCommit = func(vaultsync.Commit) error { return synctrust.ErrRemoteQuota }
	w.edit(func(d *domain.VaultData) {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f9", Name: "New"})
	})
	if _, err := w.engine.Sync(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrRemoteQuota) {
		t.Fatalf("err = %v, want the quota refusal reported", err)
	}
	w.push(w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Renamed"}))
	if _, err := w.engine.Sync(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrRemoteQuota) {
		t.Fatalf("err = %v, want the quota refusal reported again", err)
	}
	if got := folderName(w, "f1"); got != "Renamed" {
		t.Fatalf("folder f1 = %q; another device's change must arrive while this one's are refused", got)
	}
	w.server.refuseCommit = nil
	w.sync()
	pushed := false
	for _, c := range w.catchUp() {
		for _, r := range c.Records {
			pushed = pushed || (!r.Deleted && w.openAs(r).Type == synctrust.TypeFolder && bytes.Contains(w.openAs(r).Data, []byte(`"f9"`)))
		}
	}
	if !pushed {
		t.Fatal("the edit refused earlier never reached the server once it would accept it")
	}
}

// The server charges a commit for the ciphertext it adds, so deleting is how a person gets back under
// a full quota - which only works if the deletions do not travel with the additions it refuses.
func TestDeletionsLandWhileAdditionsAreRefused(t *testing.T) {
	w := readyWorld(t)
	w.server.refuseCommit = func(c vaultsync.Commit) error {
		for _, r := range c.Records {
			if !r.Deleted {
				return synctrust.ErrRemoteQuota
			}
		}
		return nil
	}
	w.edit(func(d *domain.VaultData) {
		d.Connections = nil
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f9", Name: "New"})
	})
	if _, err := w.engine.Sync(syncCtx, w.cv.ID); !errors.Is(err, synctrust.ErrRemoteQuota) {
		t.Fatalf("err = %v, want the refused addition reported", err)
	}
	deleted := false
	for _, c := range w.catchUp() {
		for _, r := range c.Records {
			deleted = deleted || r.Deleted
		}
	}
	if !deleted {
		t.Fatal("the deletion was held back with the addition the server refused")
	}
}

// The largest file a device syncs for a plugin must fit one commit under the server's default body
// limit. A 16 MiB file is base64-encoded twice on its way - inside the record and again on the wire -
// and came to about 28 MiB, so it was refused on every round whatever else the commit held.
func TestTheLargestSyncedPluginFileFitsOneCommit(t *testing.T) {
	content := bytes.Repeat([]byte{0xa7}, domainplugin.MaxSyncFileBytes)
	sum := sha256.Sum256(content)
	data, err := json.Marshal(domain.SyncPluginFile{Plugin: "com.example.echo", Path: "state/big.bin", Mode: 0o600,
		SHA256: hex.EncodeToString(sum[:]), Content: content})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := synctrust.EncodeBody(synctrust.Body{Type: synctrust.TypePluginData, Schema: 1, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	id := func(b byte) vaultsync.ID { return vaultsync.ID(bytes.Repeat([]byte{b}, 16)) }
	env := vaultsync.Envelope{Vault: id(1), Record: id(2), Version: 1, Suite: vaultsync.Suite1, Author: id(3),
		SigAlg: vaultsync.SigEd25519, Sig: make([]byte, 64)}
	if err := (synccrypto.Cipher{}).Seal(bytes.Repeat([]byte{0x5a}, 32), &env, raw); err != nil {
		t.Fatal(err)
	}
	c := vaultsync.Commit{Head: vaultsync.Head{Vault: id(1), Seq: 1, Commit: id(4), Author: id(3), SigAlg: vaultsync.SigEd25519,
		Sig: make([]byte, 64)}, Records: []vaultsync.Envelope{env}}
	wire, err := json.Marshal(syncwire.CommitFrom(&c))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) > serverCommitBody {
		t.Fatalf("a %d-byte plugin file is a %d-byte commit, over the server's %d", len(content), len(wire), serverCommitBody)
	}
}
