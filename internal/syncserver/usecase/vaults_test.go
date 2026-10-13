package usecase_test

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/usecase"
)

func TestTeamVaultLifecycle(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, carol := h.Register("alice"), h.Register("bob"), h.Register("carol")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	cv.Users[carol.Account] = carol
	if err := cv.AppendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: carol.Account, Role: vaultsync.RoleViewer, Fingerprint: servertest.Fingerprint(carol),
		Verified: true, Epoch: 0, Wraps: servertest.WrapFor(carol.Account)}); err != nil {
		t.Fatal(err)
	}
	if seq := cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1), cv.Record(alice, 2))); seq != 1 {
		t.Fatalf("first commit seq = %d", seq)
	}
	if seq := cv.Accept(bob, cv.Commit(bob, cv.Record(bob, 1))); seq != 2 {
		t.Fatalf("second commit seq = %d", seq)
	}
	changes, more, err := h.Svc.Changes(ctx, h.Principal(carol.Access), cv.ID, 0, 10)
	if err != nil || more || len(changes) != 2 || len(changes[0].Commit.Records) != 2 {
		t.Fatalf("changes = %d commits, more %v, %v", len(changes), more, err)
	}
	head, records, err := h.Svc.Snapshot(ctx, h.Principal(carol.Access), cv.ID)
	if err != nil || head == nil || head.Seq != 2 || len(records) != 2 {
		t.Fatalf("snapshot: head %+v, %d records, %v", head, len(records), err)
	}
	if _, err := vaultsync.RestoreVaultState(cv.ID, head, records); err != nil {
		t.Fatalf("the snapshot does not reproduce its own head: %v", err)
	}
	if _, err := cv.Push(carol, cv.Commit(carol, cv.Record(carol, 3))); !isErr(err, usecase.ErrForbidden) {
		t.Fatalf("viewer commit: %v, want ErrForbidden", err)
	}
	vaults, _ := h.Svc.Vaults(ctx, h.Principal(bob.Access))
	if len(vaults) != 2 {
		t.Fatalf("bob sees %d vaults, want his personal vault and the team vault", len(vaults))
	}
	if entries, err := h.Svc.DeviceLog(ctx, h.Principal(bob.Access), alice.Account, 0); err != nil || len(entries) != 1 {
		t.Fatalf("a co-member cannot read the device log it needs to verify signatures: %d, %v", len(entries), err)
	}
}

func TestCommitIsIdempotent(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	c := cv.Commit(alice, cv.Record(alice, 1))
	seq := cv.Accept(alice, c)
	again, err := h.Svc.Commit(ctx, h.Principal(alice.Access), cv.ID, 0, c)
	if err != nil || again != seq {
		t.Fatalf("a retried commit = %d, %v; want the original seq %d", again, err, seq)
	}
	changes, _, _ := h.Svc.Changes(ctx, h.Principal(alice.Access), cv.ID, 0, 10)
	if len(changes) != 1 {
		t.Fatalf("%d commits stored after a retry, want 1", len(changes))
	}
}

func TestStaleCommitConflicts(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob := h.Register("alice"), h.Register("bob")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	stale := cv.Commit(bob, cv.Record(bob, 2))
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	_, err := h.Svc.Commit(ctx, h.Principal(bob.Access), cv.ID, 0, stale)
	var conflict *usecase.ConflictError
	if !isErr(err, usecase.ErrConflict) || !asConflict(err, &conflict) || conflict.CurrentSeq != 1 {
		t.Fatalf("stale commit: %v; want a conflict naming seq 1", err)
	}
}

func TestOutsidersAndRemovedMembersSeeNothing(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, dave := h.Register("alice"), h.Register("bob"), h.Register("dave")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	pd := h.Principal(dave.Access)
	if _, _, err := h.Svc.Changes(ctx, pd, cv.ID, 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("outsider changes: %v", err)
	}
	if _, _, err := h.Svc.Snapshot(ctx, pd, cv.ID); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("outsider snapshot: %v", err)
	}
	if _, err := h.Svc.Membership(ctx, pd, cv.ID, 0); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("outsider membership: %v", err)
	}
	if _, _, err := h.Svc.Changes(ctx, pd, servertest.RandomID(t), 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Errorf("a vault that does not exist must look the same as one the caller is not in: %v", err)
	}
	if err := cv.AppendOps(alice,
		vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: bob.Account},
		vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: servertest.WrapFor(alice.Account)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Svc.Changes(ctx, h.Principal(bob.Access), cv.ID, 0, 10); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("removed member still reads the vault: %v", err)
	}
}

func TestMembershipRules(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, carol := h.Register("alice"), h.Register("bob"), h.Register("carol")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	cv.Users[carol.Account] = carol
	err := cv.AppendOps(bob, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: carol.Account, Role: vaultsync.RoleViewer, Fingerprint: servertest.Fingerprint(carol),
		Verified: true, Wraps: servertest.WrapFor(carol.Account)})
	if !isErr(err, usecase.ErrForbidden) {
		t.Errorf("an editor added a member: %v, want ErrForbidden", err)
	}
	err = cv.AppendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: bob.Account})
	if !isErr(err, usecase.ErrMalformed) {
		t.Errorf("a removal without rotation: %v, want ErrMalformed", err)
	}
	ops, err := h.Svc.Membership(ctx, h.Principal(bob.Access), cv.ID, 1)
	if err != nil || len(ops) != 1 {
		t.Fatalf("membership after seq 1 = %d ops, %v; want only the add", len(ops), err)
	}
}

func TestCommitLimits(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob := h.Register("alice"), h.Register("bob")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	c := cv.Commit(alice, cv.Record(alice, 1))
	if _, err := h.Svc.Commit(ctx, h.Principal(bob.Access), cv.ID, 0, c); !isErr(err, usecase.ErrForbidden) {
		t.Errorf("a commit pushed by a device other than its author: %v, want ErrForbidden", err)
	}
	many := make([]vaultsync.Envelope, 1001)
	if _, err := h.Svc.Commit(ctx, h.Principal(alice.Access), cv.ID, 0, vaultsync.Commit{Head: c.Head, Records: many}); !isErr(err, usecase.ErrTooLarge) {
		t.Errorf("1001 records: %v, want ErrTooLarge", err)
	}
}

func TestQuota(t *testing.T) {
	h := servertest.NewHarness(t, 1000)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	if _, err := cv.Push(alice, cv.Commit(alice, cv.Record(alice, 2), cv.Record(alice, 3))); !isErr(err, usecase.ErrQuotaExceeded) {
		t.Fatalf("over quota: %v, want ErrQuotaExceeded", err)
	}
}

func TestPersonalVaultCommit(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.PersonalVault(alice)
	if len(cv.OpsSoFar) != 1 {
		t.Fatalf("personal vault membership = %d ops, want the genesis", len(cv.OpsSoFar))
	}
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
}

func asConflict(err error, target **usecase.ConflictError) bool { return errors.As(err, target) }

func TestAddMemberWithAnotherAccountsFingerprintIsRefused(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, carol := h.Register("alice"), h.Register("bob"), h.Register("carol")
	cv := h.NewTeamVault(alice)
	cv.Users[bob.Account] = bob
	err := cv.AppendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: bob.Account, Role: vaultsync.RoleViewer,
		Fingerprint: servertest.Fingerprint(carol), Verified: true, Wraps: servertest.WrapFor(bob.Account)})
	if !isErr(err, usecase.ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed: the recorded fingerprint must be the added account's own", err)
	}
}

func TestCoMembersGetEachOthersGenesisKeys(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, mallory := h.Register("alice"), h.Register("bob"), h.Register("mallory")
	h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	keys, err := h.Svc.AccountKeys(ctx, h.Principal(bob.Access), alice.Account)
	if err != nil {
		t.Fatal(err)
	}
	if string(keys.IdentityKey) != string(alice.AIK.Public().(ed25519.PublicKey)) || string(keys.EncryptionKey) != string(alice.AEK) {
		t.Fatal("the keys returned are not the account's genesis keys")
	}
	if _, err := h.Svc.AccountKeys(ctx, h.Principal(mallory.Access), alice.Account); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for an account that shares no vault", err)
	}
}
