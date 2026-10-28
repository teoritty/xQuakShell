package usecase_test

import (
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/usecase"
)

const dayMillis = 24 * 60 * 60 * 1000

// deletedRecord writes record 1, then deletes it, in a team vault owned by alice.
func deletedRecord(t *testing.T) (*servertest.Harness, *servertest.User, *servertest.ClientVault) {
	t.Helper()
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1), cv.Record(alice, 2)))
	cv.Accept(alice, cv.Commit(alice, cv.Tombstone(alice, 1)))
	return h, alice, cv
}

func versions(t *testing.T, h *servertest.Harness, u *servertest.User, cv *servertest.ClientVault, record byte) []vaultsync.Envelope {
	t.Helper()
	out, err := h.Svc.Versions(ctx, h.Principal(u.Access), cv.ID, vaultsync.ID{record})
	if err != nil {
		t.Fatalf("versions of %d: %v", record, err)
	}
	return out
}

func TestVersionsListsEveryRetainedVersion(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	got := versions(t, h, alice, cv, 1)
	if len(got) != 2 || got[0].Version != 1 || !got[1].Deleted {
		t.Fatalf("versions = %+v; want the live version then the tombstone, oldest first", got)
	}
}

func TestVersionsIsNotFoundForNonMembers(t *testing.T) {
	h, _, cv := deletedRecord(t)
	mallory := h.Register("mallory")
	if _, err := h.Svc.Versions(ctx, h.Principal(mallory.Access), cv.ID, vaultsync.ID{1}); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: a vault id must not be an existence oracle", err)
	}
}

// The server deletes every version of a purged record in the purge's own transaction, so it is
// where the trash window has to hold. A client's clock, or the signed time of the device that deleted,
// cannot move it: the window counts from when this server received the tombstone.
func TestPurgeWaitsForTheWindowOnTheServerClock(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	advance(h, alice, 30*dayMillis-1)
	if _, err := cv.Push(alice, cv.Purge(alice, 1)); !isErr(err, usecase.ErrPurgeNotDue) {
		t.Fatalf("purge one millisecond before the window passed: %v, want ErrPurgeNotDue", err)
	}
	if got := versions(t, h, alice, cv, 1); len(got) != 2 {
		t.Fatalf("%d versions after a refused purge; the trash must still hold the record", len(got))
	}
	advance(h, alice, 1)
	cv.Accept(alice, cv.Purge(alice, 1))
}

// Shred now is the managers' explicit choice to empty the trash: a tombstone committed before the
// shred rotation may go at once, ordered by the membership log rather than by any clock.
func TestPurgeAfterShredIsDueAtOnce(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	if _, err := cv.Push(alice, cv.Purge(alice, 1)); !isErr(err, usecase.ErrPurgeNotDue) {
		t.Fatalf("purge right after the delete: %v, want ErrPurgeNotDue", err)
	}
	rotate(t, h, alice, cv, true)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 2)))
	cv.Accept(alice, cv.Purge(alice, 1))
}

// An editor may delete but not purge; the server refuses the purge as every client would.
func TestEditorPurgeIsForbidden(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	bob := h.Register("bob")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	cv.Accept(bob, cv.Commit(bob, cv.Record(bob, 1)))
	cv.Accept(bob, cv.Commit(bob, cv.Tombstone(bob, 1)))
	advance(h, bob, 30*dayMillis)
	if _, err := cv.Push(bob, cv.Purge(bob, 1)); !isErr(err, usecase.ErrForbidden) {
		t.Fatalf("an editor's purge: %v, want ErrForbidden", err)
	}
}

func TestPurgeDeletesEveryVersionOfTheRecord(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	advance(h, alice, 30*dayMillis)
	seq := cv.Accept(alice, cv.Purge(alice, 1))
	if got := versions(t, h, alice, cv, 1); len(got) != 0 {
		t.Fatalf("%d versions of a purged record remain; the server must delete them with the purge", len(got))
	}
	if got := versions(t, h, alice, cv, 2); len(got) != 1 {
		t.Fatalf("a purge deleted versions of a record it did not name: %d left", len(got))
	}
	if _, _, err := h.Svc.Changes(ctx, h.Principal(alice.Access), cv.ID, 0, 10); !isErr(err, usecase.ErrHistoryPruned) {
		t.Fatalf("pull from 0: %v, want ErrHistoryPruned - the commits before the purge lost their records", err)
	}
	changes, _, err := h.Svc.Changes(ctx, h.Principal(alice.Access), cv.ID, 2, 10)
	if err != nil || len(changes) != 1 || changes[0].Seq != seq || len(changes[0].Commit.Purged) != 1 {
		t.Fatalf("pull from 2: %+v, %v; want the purge commit carrying its purged id", changes, err)
	}
	_, records, _ := h.FullSnapshot(h.Principal(alice.Access), cv.ID)
	if len(records) != 1 {
		t.Fatalf("snapshot holds %d records, want 1", len(records))
	}
}

// The heads survive the pruning that turns a pull from before it into ErrHistoryPruned: they are what
// lets the client check that the snapshot it moves onto extends the chain it verified.
func TestHeadsOutlivePrunedHistory(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	advance(h, alice, 30*dayMillis)
	cv.Accept(alice, cv.Purge(alice, 1))
	if _, _, err := h.Svc.Changes(ctx, h.Principal(alice.Access), cv.ID, 0, 10); !isErr(err, usecase.ErrHistoryPruned) {
		t.Fatalf("pull from 0: %v, want ErrHistoryPruned", err)
	}
	tip := cv.State.Cursor()
	heads, err := h.Svc.Heads(ctx, h.Principal(alice.Access), cv.ID, 0, tip.Seq)
	if err != nil {
		t.Fatal(err)
	}
	genesis, _ := vaultsync.GenesisCursor(cv.ID)
	if got, err := genesis.AdvanceAll(heads); err != nil || got != tip {
		t.Fatalf("the heads walk from genesis to %+v, %v; want the vault's head %+v", got, err, tip)
	}
	if later, err := h.Svc.Heads(ctx, h.Principal(alice.Access), cv.ID, 1, 2); err != nil || len(later) != 1 || later[0].Seq != 2 {
		t.Fatalf("heads after 1 up to 2 = %+v, %v; want only seq 2", later, err)
	}
}

func TestHeadsIsNotFoundForNonMembers(t *testing.T) {
	h, _, cv := deletedRecord(t)
	mallory := h.Register("mallory")
	if _, err := h.Svc.Heads(ctx, h.Principal(mallory.Access), cv.ID, 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: a vault id must not be an existence oracle", err)
	}
}

func TestCommitRefusesPurgeOfALiveRecord(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	_ = h
	if _, err := cv.Push(alice, cv.Purge(alice, 2)); !isErr(err, usecase.ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed: a purge may remove only a tombstone", err)
	}
}

func TestCommitCountsPurgesAgainstTheLimit(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	_ = h
	c := cv.Purge(alice, 1)
	for i := 0; i < 1000; i++ {
		c.Purged = append(c.Purged, vaultsync.ID{byte(i), byte(i >> 8), 0xee})
	}
	if _, err := cv.Push(alice, c); !isErr(err, usecase.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge: purged ids count towards the per-commit limit", err)
	}
}

func TestHistoryIsKeptForTheWindowAfterItIsSuperseded(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	advance(h, alice, 29*dayMillis)
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := versions(t, h, alice, cv, 1); len(got) != 2 {
		t.Fatalf("%d versions after 29 days; the trash must keep the last live version for the window", len(got))
	}
	advance(h, alice, 2*dayMillis)
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	got := versions(t, h, alice, cv, 1)
	if len(got) != 1 || !got[0].Deleted {
		t.Fatalf("versions after 31 days = %+v; want only the tombstone, which waits for a client's purge", got)
	}
	if _, _, err := h.Svc.Changes(ctx, h.Principal(alice.Access), cv.ID, 0, 10); !isErr(err, usecase.ErrHistoryPruned) {
		t.Fatalf("pull from 0 after pruning: %v, want ErrHistoryPruned", err)
	}
}

func TestOldVersionWrittenLongAgoSurvivesUntilSuperseded(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	advance(h, alice, 100*dayMillis)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := versions(t, h, alice, cv, 1); len(got) != 2 {
		t.Fatalf("%d versions; a version written 100 days ago but superseded today is still history inside the window", len(got))
	}
}

// advance moves the server clock and signs the device in again, as a real client would once its
// access token expired.
func advance(h *servertest.Harness, u *servertest.User, ms uint64) {
	h.Clock.Advance(ms)
	u.Access = h.DeviceLogin(u, u.Device, u.DSK)
}

// rotate appends a rotation of alice's vault dated now on the server clock.
func rotate(t *testing.T, h *servertest.Harness, alice *servertest.User, cv *servertest.ClientVault, shred bool) {
	t.Helper()
	op := vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: cv.M.Epoch() + 1, Wraps: servertest.WrapFor(alice.Account),
		Shred: shred, TimeUnixMilli: h.Clock.NowUnixMilli()}
	if err := cv.AppendOps(alice, op); err != nil {
		t.Fatal(err)
	}
}

func epochZeroWrapsPruned(t *testing.T, h *servertest.Harness, alice *servertest.User, cv *servertest.ClientVault) bool {
	t.Helper()
	ops, err := h.Svc.Membership(ctx, h.Principal(alice.Access), cv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	cv.OpsSoFar = nil
	cv.Learn(ops...) // Replay fails the test if the stripped log no longer verifies.
	return ops[0].Wraps[0].Pruned()
}

func TestShredRotationPrunesOnceTheRewrapLands(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	rotate(t, h, alice, cv, true)
	if epochZeroWrapsPruned(t, h, alice, cv) {
		t.Fatal("epoch 0 retired while record 2 is still sealed under it")
	}
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 2)))
	if got := versions(t, h, alice, cv, 2); len(got) != 1 || got[0].Epoch != 1 {
		t.Fatalf("record 2 versions = %+v; after a shred only the rewrapped one may remain", got)
	}
	// Record 1's tombstone is still under epoch 0, but a tombstone carries no ciphertext and holds
	// no epoch in use.
	if !epochZeroWrapsPruned(t, h, alice, cv) {
		t.Fatal("the wraps of the shredded epoch kept their ciphertext")
	}
	cv.Accept(alice, cv.Purge(alice, 1))
}

func TestRetiredEpochWrapsAreStrippedAfterTheWindow(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	// The tombstone ages out of the trash first, so the purge below is due; the window that retires
	// epoch 0 then counts from the rotation.
	advance(h, alice, 30*dayMillis)
	rotate(t, h, alice, cv, false)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 2)))
	cv.Accept(alice, cv.Purge(alice, 1))
	advance(h, alice, 29*dayMillis)
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	if epochZeroWrapsPruned(t, h, alice, cv) {
		t.Fatal("epoch 0 retired before a full window after its successor began")
	}
	advance(h, alice, 2*dayMillis)
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	if !epochZeroWrapsPruned(t, h, alice, cv) {
		t.Fatal("epoch 0 wraps kept their ciphertext a window after the rotation")
	}
}

func TestPruningReturnsQuota(t *testing.T) {
	h, alice, cv := deletedRecord(t)
	advance(h, alice, 30*dayMillis)
	before := vaultBytes(t, h, alice, cv)
	cv.Accept(alice, cv.Purge(alice, 1))
	if after := vaultBytes(t, h, alice, cv); after >= before {
		t.Fatalf("bytes %d after a purge, %d before; deleted ciphertext must stop counting against the quota", after, before)
	}
}

func vaultBytes(t *testing.T, h *servertest.Harness, u *servertest.User, cv *servertest.ClientVault) uint64 {
	t.Helper()
	var bytes uint64
	err := h.DB.WithTx(ctx, func(tx usecase.Tx) error {
		v, _, err := tx.VaultByID(cv.ID)
		bytes = v.Bytes
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func TestShredOfAnEmptyVaultRetiresOnTheAppend(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	rotate(t, h, alice, cv, true)
	if !epochZeroWrapsPruned(t, h, alice, cv) {
		t.Fatal("nothing uses epoch 0, so the shred rotation itself must retire it")
	}
}

func TestHistoryIsPrunedExactlyOneWindowAfterItWasSuperseded(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	advance(h, alice, 100*dayMillis)
	cv := h.NewTeamVault(alice)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	advance(h, alice, 30*dayMillis-1)
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := versions(t, h, alice, cv, 1); len(got) != 2 {
		t.Fatalf("%d versions one millisecond before the window closed", len(got))
	}
	advance(h, alice, 1)
	if err := h.Svc.PruneAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := versions(t, h, alice, cv, 1); len(got) != 1 {
		t.Fatalf("%d versions when the window closed; history is kept for the window, not longer", len(got))
	}
}
