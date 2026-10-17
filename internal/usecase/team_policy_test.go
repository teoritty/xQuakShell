package usecase_test

import (
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

func TestARequireVerifiedTeamRefusesAnUnverifiedInvite(t *testing.T) {
	h, a := registered(t)
	team, err := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.engine.SetTeamPolicy(syncCtx, team, 14, true); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	bob := h.Register("bob")
	err = a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: fingerprintOf(bob), Role: vaultsync.RoleEditor})
	if !errors.Is(err, vaultsync.ErrUnverifiedMember) {
		t.Fatalf("err = %v, want ErrUnverifiedMember", err)
	}
	if m := serverMembership(t, h, asServerUser(t, h, a), team); m.RetentionDays() != 14 || !m.RequireVerified() {
		t.Fatalf("policy = %d days, verified %v", m.RetentionDays(), m.RequireVerified())
	}
}

func TestVerifyingAMembersFingerprint(t *testing.T) {
	h, a := registered(t)
	team, _ := a.engine.CreateTeam(syncCtx, "Ops", rand.Reader)
	bob := h.Register("bob")
	if err := a.engine.Invite(syncCtx, usecase.InviteRequest{Vault: team, Username: "bob", Fingerprint: fingerprintOf(bob), Role: vaultsync.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	for _, m := range mustMembers(t, a, team) {
		if m.Account == bob.Account && m.Verified {
			t.Fatal("an unverified invite shows the member verified")
		}
	}
	if err := a.engine.VerifyMember(syncCtx, team, bob.Account, "00000"); !errors.Is(err, usecase.ErrInviteKeyChanged) {
		t.Fatalf("err = %v, want ErrInviteKeyChanged for a fingerprint that is not the member's", err)
	}
	if err := a.engine.VerifyMember(syncCtx, team, bob.Account, fingerprintOf(bob)); err != nil {
		t.Fatal(err)
	}
	for _, m := range mustMembers(t, a, team) {
		if m.Account == bob.Account && !m.Verified {
			t.Fatal("a member verified by fingerprint is not shown verified")
		}
	}
}

func mustMembers(t *testing.T, d *device, team vaultsync.ID) []usecase.MemberInfo {
	t.Helper()
	out, err := d.engine.Members(syncCtx, team)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
