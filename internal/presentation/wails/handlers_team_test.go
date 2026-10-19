package wails

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

type fakeTeams struct {
	err     error
	invited usecase.InviteRequest
	role    vaultsync.Role
	removed vaultsync.ID
	moved   usecase.MoveConnectionRequest
	// writable is what CanWriteTeam answers.
	writable bool
	// answered is the vault the last accept or decline named, and accepted which of the two it was.
	answered vaultsync.ID
	accepted bool
}

func (f *fakeTeams) Teams(context.Context) ([]usecase.TeamInfo, error) {
	return []usecase.TeamInfo{{Vault: vaultsync.ID{1}, Name: "Ops", Role: vaultsync.RoleOwner}}, f.err
}
func (f *fakeTeams) CreateTeam(context.Context, string) (vaultsync.ID, error) {
	return vaultsync.ID{1}, f.err
}
func (f *fakeTeams) LookupInvitee(_ context.Context, u string) (usecase.Invitee, error) {
	return usecase.Invitee{Username: u, Fingerprint: "11111 22222"}, f.err
}
func (f *fakeTeams) Invite(_ context.Context, r usecase.InviteRequest) error {
	f.invited = r
	return f.err
}
func (f *fakeTeams) TeamMembers(context.Context, vaultsync.ID) ([]usecase.MemberInfo, error) {
	return []usecase.MemberInfo{{Account: vaultsync.ID{2}, Role: vaultsync.RoleEditor, Fingerprint: "1"}}, f.err
}
func (f *fakeTeams) ChangeTeamRole(_ context.Context, _, _ vaultsync.ID, r vaultsync.Role) error {
	f.role = r
	return f.err
}
func (f *fakeTeams) RemoveTeamMember(_ context.Context, _, a vaultsync.ID) error {
	f.removed = a
	return f.err
}
func (f *fakeTeams) LeaveTeam(context.Context, vaultsync.ID) error { return f.err }
func (f *fakeTeams) VerifyTeamMember(context.Context, vaultsync.ID, vaultsync.ID, string) error {
	return f.err
}
func (f *fakeTeams) SetTeamPolicy(context.Context, vaultsync.ID, uint16, bool) error { return f.err }
func (f *fakeTeams) MoveConnection(_ context.Context, r usecase.MoveConnectionRequest) (string, error) {
	f.moved = r
	return "new-id", f.err
}
func (f *fakeTeams) CanWriteTeam(context.Context, vaultsync.ID) (bool, error) {
	return f.writable, f.err
}
func (f *fakeTeams) Invitations(context.Context) ([]usecase.Invitation, error) {
	return []usecase.Invitation{{Vault: vaultsync.ID{3}, Name: "Ops", Role: vaultsync.RoleEditor, Inviter: vaultsync.ID{4},
		InviterFingerprint: "33333 44444", InviterUsername: "bob", InviterKnown: true}}, f.err
}
func (f *fakeTeams) AcceptTeam(_ context.Context, v vaultsync.ID) error {
	f.answered, f.accepted = v, true
	return f.err
}
func (f *fakeTeams) DeclineTeam(_ context.Context, v vaultsync.ID) error {
	f.answered, f.accepted = v, false
	return f.err
}

type fakeBindings struct {
	bound domain.CredentialBinding
	err   error
}

func (f *fakeBindings) Bindings() ([]domain.CredentialBinding, error) {
	return []domain.CredentialBinding{{VaultID: "v", ConnectionID: "c", UserID: "u", IdentityID: "k", SuspendedBy: "carol"}}, f.err
}
func (f *fakeBindings) Bind(_ context.Context, b domain.CredentialBinding) error {
	f.bound = b
	return f.err
}
func (f *fakeBindings) Confirm(context.Context, string, string, string) error { return f.err }

func teamAPI() (*AppAPI, *fakeTeams, *fakeBindings) {
	a := &AppAPI{}
	t, b := &fakeTeams{}, &fakeBindings{}
	a.SetTeamServices(t, b)
	return a, t, b
}

var (
	hexV = hex.EncodeToString((&vaultsync.ID{1})[:])
	hexA = hex.EncodeToString((&vaultsync.ID{2})[:])
)

func TestTeamHandlersHappyPath(t *testing.T) {
	a, f, b := teamAPI()
	if teams, err := a.ListTeams(); err != nil || len(teams) != 1 || teams[0].Role != "owner" || teams[0].Vault != hexV {
		t.Fatalf("teams = %+v, %v", teams, err)
	}
	if id, err := a.CreateTeam("  Ops  "); err != nil || id != hexV {
		t.Fatalf("create = %q, %v", id, err)
	}
	if inv, err := a.LookupTeamInvitee("bob"); err != nil || inv.Fingerprint != "11111 22222" {
		t.Fatalf("lookup = %+v, %v", inv, err)
	}
	if err := a.InviteToTeam(TeamInviteRequest{Vault: hexV, Username: "bob", Fingerprint: "11111 22222", Role: "editor", Verified: true}); err != nil ||
		f.invited.Role != vaultsync.RoleEditor || !f.invited.Verified {
		t.Fatalf("invite: %v, %+v", err, f.invited)
	}
	if err := a.ChangeTeamRole(hexV, hexA, "viewer"); err != nil || f.role != vaultsync.RoleViewer {
		t.Fatalf("role: %v", err)
	}
	if err := a.RemoveTeamMember(hexV, hexA); err != nil || f.removed != (vaultsync.ID{2}) {
		t.Fatalf("remove: %v", err)
	}
	if members, err := a.ListTeamMembers(hexV); err != nil || len(members) != 1 || members[0].Role != "editor" {
		t.Fatalf("members = %+v, %v", members, err)
	}
	if list, err := a.ListCredentialBindings(); err != nil || len(list) != 1 || list[0].SuspendedBy != "carol" {
		t.Fatalf("bindings = %+v, %v", list, err)
	}
	if err := a.BindCredential(CredentialBindingDTO{Vault: hexV, Connection: "c", User: "u", Password: "p"}); err != nil || b.bound.PasswordID != "p" {
		t.Fatalf("bind: %v", err)
	}
	for _, err := range []error{a.LeaveTeam(hexV), a.VerifyTeamMember(hexV, hexA, "1"), a.SetTeamPolicy(hexV, 30, true), a.ConfirmCredentialBinding(hexV, "c", "u")} {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTeamHandlersRefuseInvalidInput(t *testing.T) {
	a, _, _ := teamAPI()
	for name, err := range map[string]error{
		"empty name":     func() error { _, err := a.CreateTeam(" "); return err }(),
		"long name":      func() error { _, err := a.CreateTeam(strings.Repeat("x", 129)); return err }(),
		"owner role":     a.InviteToTeam(TeamInviteRequest{Vault: hexV, Username: "bob", Fingerprint: "1", Role: "owner"}),
		"bad vault":      a.InviteToTeam(TeamInviteRequest{Vault: "zz", Username: "bob", Fingerprint: "1", Role: "editor"}),
		"no fingerprint": a.InviteToTeam(TeamInviteRequest{Vault: hexV, Username: "bob", Role: "editor"}),
		"bad account":    a.RemoveTeamMember(hexV, "00"),
		"bad role":       a.ChangeTeamRole(hexV, hexA, "god"),
		"bad days":       a.SetTeamPolicy(hexV, 366, false),
		"bad binding":    a.BindCredential(CredentialBindingDTO{Vault: "x", Connection: "c", User: "u", Identity: "k"}),
		"zero vault":     a.LeaveTeam(hex.EncodeToString(make([]byte, 16))),
		"move no conn":   func() error { _, err := a.MoveConnectionToVault("", hexV, ""); return err }(),
		"move bad vault": func() error { _, err := a.MoveConnectionToVault("c", "zz", ""); return err }(),
		"new in bad":     a.requireWritableTeam("zz"),
	} {
		if !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("%s: %v, want the invalid-request error", name, err)
		}
	}
}

func TestTeamHandlersMapRefusalsToSentences(t *testing.T) {
	a, f, b := teamAPI()
	f.err = synctrust.ErrRemoteNotFound
	if _, err := a.ListTeamMembers(hexV); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a team the account is not in: %v", err)
	}
	f.err = synctrust.ErrRemoteForbidden
	if err := a.RemoveTeamMember(hexV, hexA); err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("a non-admin removing: %v", err)
	}
	f.err = usecase.ErrInviteKeyChanged
	if err := a.InviteToTeam(TeamInviteRequest{Vault: hexV, Username: "bob", Fingerprint: "1", Role: "editor"}); err == nil || !strings.Contains(err.Error(), "compare") {
		t.Fatalf("a changed key: %v", err)
	}
	b.err = domain.ErrBindingSuspended
	if err := a.ConfirmCredentialBinding(hexV, "c", "u"); err == nil || !strings.Contains(err.Error(), "address") {
		t.Fatalf("suspended: %v", err)
	}
}

// An empty vault is the personal one: a move there goes out as an empty destination, never as a
// parse failure, and the new id comes back for the tree to select.
func TestMoveConnectionToVaultPassesTheDestinationThrough(t *testing.T) {
	a, f, _ := teamAPI()
	if id, err := a.MoveConnectionToVault("c", "", ""); err != nil || id != "new-id" || f.moved.To != "" || f.moved.Connection != "c" {
		t.Fatalf("to personal: %q, %v, %+v", id, err, f.moved)
	}
	if _, err := a.MoveConnectionToVault("c", hexV, "f"); err != nil || f.moved.To != hexV || f.moved.Folder != "f" {
		t.Fatalf("to a team folder: %v, %+v", err, f.moved)
	}
	f.err = domain.ErrVaultMismatch
	if _, err := a.MoveConnectionToVault("c", hexV, "f"); err == nil || !strings.Contains(err.Error(), "another vault") {
		t.Fatalf("a folder of another vault: %v", err)
	}
}

// A new item in a team needs a role that writes; the personal vault needs nothing, sync or not.
func TestCreatingInATeamNeedsARoleThatWrites(t *testing.T) {
	a, f, _ := teamAPI()
	if err := a.requireWritableTeam(hexV); err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("a viewer creating in the team: %v, want the role refusal", err)
	}
	f.writable = true
	if err := a.requireWritableTeam(hexV); err != nil {
		t.Fatalf("an editor creating in the team: %v", err)
	}
	if err := (&AppAPI{}).requireWritableTeam(""); err != nil {
		t.Fatalf("the personal vault without sync: %v", err)
	}
	if err := (&AppAPI{}).requireWritableTeam(hexV); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("a team without sync: %v, want errSyncUnavailable", err)
	}
}

func TestTeamHandlersWithoutSync(t *testing.T) {
	a := &AppAPI{}
	if _, err := a.ListTeams(); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("teams: %v", err)
	}
	if _, err := a.ListCredentialBindings(); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("bindings: %v", err)
	}
	if err := a.InviteToTeam(TeamInviteRequest{}); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("invite: %v", err)
	}
}

func TestTeamInvitationsMapAndValidate(t *testing.T) {
	a, f, _ := teamAPI()
	invs, err := a.ListTeamInvitations()
	if err != nil || len(invs) != 1 {
		t.Fatalf("invitations = %+v, %v", invs, err)
	}
	want := TeamInvitationDTO{Vault: vaultsync.ID{3}.String(), Name: "Ops", Role: "editor",
		Inviter: vaultsync.ID{4}.String(), InviterFingerprint: "33333 44444", InviterUsername: "bob", InviterKnown: true}
	if invs[0] != want {
		t.Fatalf("invitation = %+v, want %+v", invs[0], want)
	}
	if err := a.AcceptTeamInvitation("not-hex"); !errors.Is(err, errInvalidSyncRequest) {
		t.Fatalf("accepting a malformed id: %v, want errInvalidSyncRequest", err)
	}
	if err := a.AcceptTeamInvitation(want.Vault); err != nil || f.answered != (vaultsync.ID{3}) || !f.accepted {
		t.Fatalf("accept = %v; answered %v accepted %v", err, f.answered, f.accepted)
	}
	if err := a.DeclineTeamInvitation(want.Vault); err != nil || f.accepted {
		t.Fatalf("decline = %v; accepted %v", err, f.accepted)
	}
	f.err = usecase.ErrNoInvitation
	err = a.AcceptTeamInvitation(want.Vault)
	if err == nil || strings.Contains(err.Error(), "sync:") {
		t.Fatalf("a closed invitation's error = %v; want the user-facing message", err)
	}
}
