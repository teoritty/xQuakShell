package usecase_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
)

const retentionDay = int64(24 * 60 * 60 * 1000)

// pair is the account of registered(t) with a second device signed in and synced.
func pair(t *testing.T) (*servertest.Harness, *device, *device) {
	t.Helper()
	h, a := registered(t)
	b := newDevice(t, h, "phone")
	s, err := b.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	b.start(s)
	return h, a, b
}

func addFolders(d *device, ids ...string) {
	_ = d.vault.UpdateData(syncCtx, func(v *domain.VaultData) error {
		for _, id := range ids {
			v.Folders = append(v.Folders, domain.ConnectionFolder{ID: id, Name: "folder " + id})
		}
		return nil
	})
}

func removeFolder(d *device, id string) {
	_ = d.vault.UpdateData(syncCtx, func(v *domain.VaultData) error {
		for i, f := range v.Folders {
			if f.ID == id {
				v.Folders = append(v.Folders[:i], v.Folders[i+1:]...)
				return nil
			}
		}
		return nil
	})
}

func folderIDs(d *device) map[string]bool {
	data, _ := d.vault.GetData()
	out := map[string]bool{}
	for _, f := range data.Folders {
		out[f.ID] = true
	}
	return out
}

// storedRecords counts the records the device's verified base holds, tombstones included.
func storedRecords(t *testing.T, d *device) (live, tombstones int) {
	t.Helper()
	sv, err := d.state.LoadVault(syncCtx, d.session.Personal)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range sv.Records {
		if r.Deleted {
			tombstones++
		} else {
			live++
		}
	}
	return live, tombstones
}

func setClocks(at int64, ds ...*device) {
	for _, d := range ds {
		d.now.Store(at)
	}
}

func TestTombstoneIsPurgedOnceTheWindowHasPassed(t *testing.T) {
	h, a := registered(t)
	addFolders(a, "f1", "f2")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 1 {
		t.Fatalf("%d tombstones after the delete, want 1", tombs)
	}
	// The server counts the window on its own clock and is already past it, so what decides here is
	// this device's clock.
	h.Clock.Advance(uint64(31 * retentionDay))
	setClocks(7+30*retentionDay-1, a)
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 1 {
		t.Fatal("the tombstone was purged a millisecond before the window closed")
	}
	setClocks(7+30*retentionDay, a)
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 0 {
		t.Fatal("the tombstone outlived the window; the trash must empty itself")
	}
	if !folderIDs(a)["f2"] {
		t.Fatal("the purge took a live folder with it")
	}
}

func TestNoPurgeWhenTheClockIsBehind(t *testing.T) {
	_, a := registered(t)
	setClocks(1_000_000, a)
	addFolders(a, "f1")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	setClocks(5, a)
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 1 {
		t.Fatal("a clock behind the deletion purged it; an earlier clock must count as no time passed")
	}
}

// A device whose clock runs ahead of the server's thinks the window passed; the server, which does the
// deleting, refuses the purge. The round still succeeds, the trash keeps the record, and nothing is
// left in the outbox to be refused again on every later round.
func TestPurgeTheServerHoldsBackLeavesTheTombstone(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	setClocks(7+31*retentionDay, a)
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 1 {
		t.Fatal("the tombstone is gone although the server had not seen its window pass")
	}
	if c, err := a.state.Outbox(syncCtx, a.session.Personal); err != nil || c != nil {
		t.Fatalf("outbox after a refused purge = %v, %v; it must be empty", c, err)
	}
	a.sync()
}

func TestPullAppliesAPurge(t *testing.T) {
	h, a, b := pair(t)
	addFolders(a, "f1", "f2")
	a.sync()
	b.sync()
	removeFolder(a, "f1")
	a.sync()
	b.sync()
	setClocks(7+31*retentionDay, a, b)
	h.Clock.Advance(uint64(31 * retentionDay))
	a.sync()
	b.sync()
	if live, tombs := storedRecords(t, b); tombs != 0 || live == 0 {
		t.Fatalf("other device holds %d live, %d tombstones; it must apply the purge", live, tombs)
	}
}

func TestResyncAfterPrunedHistoryKeepsLocalEdits(t *testing.T) {
	h, a, b := pair(t)
	addFolders(a, "f1", "f2")
	a.sync()
	b.sync()
	removeFolder(a, "f1")
	a.sync()
	// b stays offline past the window and makes an edit there.
	addFolders(b, "f3")
	h.Clock.Advance(uint64(31 * retentionDay))
	setClocks(7+31*retentionDay, a, b)
	a.sync()
	b.sync()
	got := folderIDs(b)
	if got["f1"] || !got["f2"] || !got["f3"] {
		t.Fatalf("after a resync b has %v; want f2 and its own unpushed f3, without the purged f1", got)
	}
	a.sync()
	if !folderIDs(a)["f3"] {
		t.Fatal("the edit kept through the resync never reached the other device")
	}
}

func TestPeriodicRotationAfterHalfAWindow(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	setClocks(7+15*retentionDay-1, a)
	a.sync()
	if epoch := currentEpoch(t, a); epoch != 0 {
		t.Fatalf("epoch %d before half a window passed", epoch)
	}
	setClocks(7+15*retentionDay, a)
	a.sync()
	if epoch := currentEpoch(t, a); epoch != 1 {
		t.Fatalf("epoch %d after half a window with changes; the vault must rotate", epoch)
	}
	setClocks(7+31*retentionDay, a)
	a.sync()
	if epoch := currentEpoch(t, a); epoch != 2 {
		t.Fatalf("epoch %d; the rewrap after the first rotation counts as a change, so it rotates again", epoch)
	}
}

func TestRotationOnALaggingClockIsDatedAtThePreviousOne(t *testing.T) {
	_, a := registered(t)
	setClocks(3, a)
	if err := a.engine.RotateEpoch(syncCtx, a.session.Personal, nil, false, rand.Reader); err != nil {
		t.Fatalf("a device whose clock is behind the vault's creation could not rotate: %v", err)
	}
	ops, _ := a.state.Membership(syncCtx, a.session.Personal)
	if last := ops[len(ops)-1]; last.Kind != vaultsync.MemberRotateEpoch || last.TimeUnixMilli != 7 {
		t.Fatalf("last op %d at %d; want a rotation dated at the creation time 7", last.Kind, last.TimeUnixMilli)
	}
}

func currentEpoch(t *testing.T, d *device) uint32 {
	t.Helper()
	ops, err := d.state.Membership(syncCtx, d.session.Personal)
	if err != nil {
		t.Fatal(err)
	}
	var epoch uint32
	for _, op := range ops {
		if op.Kind == vaultsync.MemberRotateEpoch {
			epoch = op.Epoch
		}
	}
	return epoch
}

func TestPullRefusesAPurgeOfALiveRecord(t *testing.T) {
	w := newSyncWorld(t)
	folder := w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"})
	w.push(w.meta(), folder)
	w.pull()
	w.push(w.item(synctrust.TypeFolder, "f2", domain.ConnectionFolder{ID: "f2", Name: "Dev"}))
	w.server.tamper = func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
		for i := range cs {
			cs[i].Commit.Purged = []vaultsync.ID{folder.Record}
		}
		return cs
	}
	_, err := w.engine.Pull(syncCtx, w.cv.ID)
	if !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity: a server cannot purge a live record", err)
	}
	data, _ := w.vault.GetData()
	if len(data.Folders) != 1 {
		t.Fatal("a refused commit changed the vault")
	}
}

func TestResyncRefusesAnOlderSnapshot(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	old, err := (&svcServer{h: w.h, token: w.a.Access}).Snapshot(context.Background(), w.cv.ID)
	if err != nil {
		t.Fatal(err)
	}
	w.push(w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	w.pull()
	w.server.historyPruned = true
	w.server.tamperSnap = func(synctrust.RemoteSnapshot) synctrust.RemoteSnapshot { return old }
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, vaultsync.ErrRollback) {
		t.Fatalf("err = %v, want ErrRollback: a resync may not step back behind the verified head", err)
	}
}

func TestResyncAtTheVerifiedHeadChangesNothing(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	w.pull()
	w.server.historyPruned = true
	r, err := w.engine.Pull(syncCtx, w.cv.ID)
	if err != nil || r.Commits != 0 {
		t.Fatalf("resync at the same head: %+v, %v; want nothing to do", r, err)
	}
}

// A resync lands a snapshot only when the heads between the verified cursor and the snapshot link
// the two. The honest case still lands; every way of breaking the link is a fork and changes nothing.
func TestResyncRequiresTheSnapshotToExtendTheVerifiedChain(t *testing.T) {
	setup := func(t *testing.T) *syncWorld {
		w := newSyncWorld(t)
		w.push(w.meta(), w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
		w.pull()
		w.push(w.item(synctrust.TypeFolder, "f2", domain.ConnectionFolder{ID: "f2", Name: "Dev"}))
		w.push(w.item(synctrust.TypeFolder, "f3", domain.ConnectionFolder{ID: "f3", Name: "Test"}))
		w.server.historyPruned = true
		return w
	}
	t.Run("linked", func(t *testing.T) {
		w := setup(t)
		if r, err := w.engine.Pull(syncCtx, w.cv.ID); err != nil || r.Commits != 1 {
			t.Fatalf("resync over a linked chain: %+v, %v; it must land the snapshot", r, err)
		}
		if d, _ := w.vault.GetData(); len(d.Folders) != 3 {
			t.Fatalf("folders = %+v; the snapshot's state must land", d.Folders)
		}
	})
	breaks := map[string]func([]vaultsync.Head) []vaultsync.Head{
		"a head that does not name the one before": func(h []vaultsync.Head) []vaultsync.Head { h[0].PrevHash[0] ^= 1; return h },
		"a head withheld":                          func(h []vaultsync.Head) []vaultsync.Head { return h[1:] },
		"no heads at all":                          func([]vaultsync.Head) []vaultsync.Head { return nil },
		"the chain stops short":                    func(h []vaultsync.Head) []vaultsync.Head { return h[:1] },
		"a chain to another tip": func(h []vaultsync.Head) []vaultsync.Head {
			h[len(h)-1].ClientTimeUnixMilli++
			return h
		},
	}
	for name, tamper := range breaks {
		t.Run(name, func(t *testing.T) {
			w := setup(t)
			before, _ := w.vault.GetData()
			sv, _ := w.state.LoadVault(syncCtx, w.cv.ID)
			calls := 0
			w.server.tamperHeads = func(h []vaultsync.Head) []vaultsync.Head {
				// A chain that stops short is asked for again from where it stopped; the second answer
				// is honest so the test sees what the walk does with a chain that ends early.
				if calls++; calls > 1 {
					return nil
				}
				return tamper(h)
			}
			if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, vaultsync.ErrFork) || !errors.Is(err, synctrust.ErrIntegrity) {
				t.Fatalf("err = %v; a snapshot the heads do not tie to the verified chain is a fork", err)
			}
			assertUntouched(t, w, before, sv.Head.Seq)
		})
	}
}

func TestAFreshDeletionIsNotPurgedInTheRoundThatMadeIt(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	// Deleted long after it was written: the window counts from the deletion, not the write.
	setClocks(100*retentionDay, a)
	removeFolder(a, "f1")
	a.sync()
	if _, tombs := storedRecords(t, a); tombs != 1 {
		t.Fatal("a deletion made in this round was purged at once; its window starts when it lands")
	}
}
