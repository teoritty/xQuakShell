package usecase_test

import (
	"strings"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/usecase"
)

// The invite list never shows a usable code: only an id derived from the stored hash, the expiry,
// and whether it was used.
func TestInviteListShowsNoUsableCode(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	code := h.Invite()
	h.Register("alice") // consumes a second invite of its own
	list, err := h.Svc.ListInvites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("invites = %+v, want 2", list)
	}
	used := 0
	for _, inv := range list {
		if strings.Contains(code, inv.ID) || strings.Contains(inv.ID, code) || len(inv.ID) != 12 {
			t.Fatalf("invite id %q; an id is a short hash prefix, never the code", inv.ID)
		}
		if inv.Used {
			used++
		}
	}
	if used != 1 {
		t.Fatalf("%d used invites, want 1", used)
	}
}

// Revoking by id deletes an unused invite, so the code stops working; a used one is history and
// stays; an id must name exactly one invite.
func TestRevokeInviteByID(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	code := h.Invite()
	list, _ := h.Svc.ListInvites(ctx)
	if err := h.Svc.RevokeInvite(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Svc.RegisterStart(ctx, code, "bob", vaultsync.ID{9}, []byte("req")); err == nil {
		t.Fatal("a revoked invite still starts a registration")
	}
	if err := h.Svc.RevokeInvite(ctx, list[0].ID); !isErr(err, usecase.ErrInviteNotFound) {
		t.Fatalf("revoking it again: %v, want ErrInviteNotFound", err)
	}
	h.Register("alice")
	list, _ = h.Svc.ListInvites(ctx)
	if err := h.Svc.RevokeInvite(ctx, list[0].ID); !isErr(err, usecase.ErrInviteNotFound) {
		t.Fatalf("revoking a used invite: %v, want ErrInviteNotFound", err)
	}
	if err := h.Svc.RevokeInvite(ctx, ""); !isErr(err, usecase.ErrInviteNotFound) {
		t.Fatalf("an empty id matches everything: %v", err)
	}
}

func TestStatsCountWhatTheServerHolds(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	// Registration creates each account's personal vault.
	h.Register("alice")
	h.Register("bob")
	if err := h.Svc.SetFrozen(ctx, "bob", true); err != nil {
		t.Fatal(err)
	}
	st, err := h.Svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Accounts != 2 || st.Frozen != 1 || st.Vaults != 2 {
		t.Fatalf("stats = %+v; want 2 accounts, 1 frozen, 2 vaults", st)
	}
}

// Deleting an account deletes every vault it alone belongs to - its personal vault and a team with
// no other member - with their commits and record versions (spec 9.3). A team other accounts still
// share stays.
func TestDeleteAccountDeletesTheVaultsItAloneHolds(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	bob := h.Register("bob")
	h.NewTeamVault(alice)
	h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	before, _ := h.Svc.Stats(ctx)
	if before.Vaults != 4 {
		t.Fatalf("vaults before = %d, want 4 (two personal, two teams)", before.Vaults)
	}
	if err := h.Svc.DeleteAccount(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	after, _ := h.Svc.Stats(ctx)
	if after.Accounts != 1 || after.Vaults != 2 {
		t.Fatalf("after deleting alice: %+v; want bob's personal vault and the team bob shares", after)
	}
	if _, err := h.Svc.Vaults(ctx, h.Principal(bob.Access)); err != nil {
		t.Fatalf("bob's vaults: %v", err)
	}
}
