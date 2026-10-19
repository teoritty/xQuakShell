package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrTeamNotJoined is an operation on a team vault this account was added to but has not joined.
	// Its content stays out of the local vault until the person accepts the invitation.
	ErrTeamNotJoined = errors.New("sync: that team has not been joined on this account")
	// ErrNoInvitation is an accept or decline of a vault that is not an invitation for this account.
	ErrNoInvitation = errors.New("sync: there is no invitation to that team")
)

// Invitation is a team vault the server lists for this account that the person has not joined yet:
// another account added them to it. Nothing of it reaches the local vault before it is accepted.
//
// Anyone with an account on the server can create a team and add any other account to it, and the
// server itself can list any vault it likes. Syncing every listed vault therefore let a stranger -
// or the operator - place connections, keys and passwords under a team row of this person's tree,
// trusting the stranger's key on first use as the team's creator. The person is shown who invited
// them, by fingerprint, and decides.
type Invitation struct {
	Vault vaultsync.ID
	// Name is the team's name as its members wrote it; for display only.
	Name string
	Role vaultsync.Role
	// Inviter is the account whose add-member op added this one, with its fingerprint and what this
	// user's contacts know of it.
	Inviter            vaultsync.ID
	InviterFingerprint string
	InviterUsername    string
	InviterKnown       bool
	InviterVerified    bool
	// MembershipSeq is the membership position the invitation was read at.
	MembershipSeq uint64
}

// joinedTeams sorts the team vaults the server lists into those this account joined and those still
// waiting for the person's answer. A vault counts as joined when the personal vault records it, or -
// for a team joined before joining was recorded - when this device already holds its items or its
// verified membership log; those are recorded now, so the account's other devices join them too.
func (e *SyncEngine) joinedTeams(ctx context.Context, listed []synctrust.RemoteVault) ([]vaultsync.ID, []synctrust.RemoteVault, error) {
	data, err := e.d.Vault.GetData()
	if err != nil {
		return nil, nil, err
	}
	personal := synctrust.PersonalVaultID(e.d.Keys.Account())
	local := localTeamVaults(data)
	var joined, legacy []vaultsync.ID
	var pending []synctrust.RemoteVault
	for _, v := range listed {
		if v.Vault == personal || slices.Contains(joined, v.Vault) {
			continue
		}
		if teamRecorded(data, v.Vault) {
			joined = append(joined, v.Vault)
			continue
		}
		before, err := e.joinedBefore(ctx, local, v.Vault)
		if err != nil {
			return nil, nil, err
		}
		if before {
			joined, legacy = append(joined, v.Vault), append(legacy, v.Vault)
			continue
		}
		pending = append(pending, v)
	}
	return joined, pending, e.recordTeams(ctx, legacy...)
}

// joinedBefore reports a team this device synced before joining was recorded: it holds the team's
// items, or the membership log a sync verified and stored. An invitation is never stored there -
// inspecting one replays its log in memory only, and every other path refuses a vault not joined.
func (e *SyncEngine) joinedBefore(ctx context.Context, local []vaultsync.ID, vault vaultsync.ID) (bool, error) {
	if slices.Contains(local, vault) {
		return true, nil
	}
	ops, err := e.d.State.Membership(ctx, vault)
	return len(ops) > 0, err
}

func teamRecorded(d *domain.VaultData, vault vaultsync.ID) bool {
	if d.Sync == nil {
		return false
	}
	v := vault.String()
	return slices.ContainsFunc(d.Sync.Teams, func(t domain.SyncTeam) bool { return t.Vault == v })
}

// recordTeams writes the given teams into the personal vault as joined; the next round pushes the
// records to the account's other devices.
func (e *SyncEngine) recordTeams(ctx context.Context, vaults ...vaultsync.ID) error {
	if len(vaults) == 0 {
		return nil
	}
	entries := make([]synctrust.Entry, 0, len(vaults))
	for _, v := range vaults {
		entries = append(entries, teamEntry(v))
	}
	return e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { return synctrust.Apply(d, entries) })
}

func teamEntry(vault vaultsync.ID) synctrust.Entry {
	data, _ := json.Marshal(domain.SyncTeam{Vault: vault.String()}) // A struct of one string always encodes.
	return synctrust.Entry{Type: synctrust.TypeTeam, Key: vault.String(), Data: data}
}

// requireJoined refuses a team vault the account has not joined, so no path - a pull, a member
// listing, the trash - stores or applies anything of one before the person accepts it.
func (e *SyncEngine) requireJoined(ctx context.Context, vault vaultsync.ID) error {
	if e.scopeOf(vault).Vault == "" {
		return nil
	}
	data, err := e.d.Vault.GetData()
	if err != nil {
		return err
	}
	if teamRecorded(data, vault) {
		return nil
	}
	ok, err := e.joinedBefore(ctx, localTeamVaults(data), vault)
	if err != nil || ok {
		return err
	}
	return ErrTeamNotJoined
}

// Invitations lists the team vaults this account was added to and has not joined. Each one is
// verified as a sync would verify it - device logs anchored by the fingerprints the log records -
// but only in memory: nothing about it is stored, and no first-use contact is made for its creator.
// An invitation whose log does not verify is not shown.
//
// known are invitations listed before; one whose membership has not moved since is reused rather
// than verified again, which would cost a handful of requests per invitation on every page reload.
// What the person's contacts say of its inviter is read afresh either way.
func (e *SyncEngine) Invitations(ctx context.Context, known []Invitation) ([]Invitation, error) {
	listed, err := e.d.Server.Vaults(ctx)
	if err != nil {
		return nil, err
	}
	_, pending, err := e.joinedTeams(ctx, listed)
	if err != nil {
		return nil, err
	}
	var out []Invitation
	for _, v := range pending {
		if i := slices.IndexFunc(known, func(k Invitation) bool { return k.Vault == v.Vault && k.MembershipSeq == v.MembershipSeq }); i >= 0 {
			out = append(out, e.withContact(known[i]))
			continue
		}
		inv, _, err := e.inspectInvitation(ctx, v.Vault)
		switch {
		case errors.Is(err, synctrust.ErrIntegrity), errors.Is(err, ErrNoInvitation):
			continue
		case err != nil:
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

// inspectInvitation replays a vault's membership log from the server without storing it and
// describes the invitation it holds for this account, with the verified state for an answer.
func (e *SyncEngine) inspectInvitation(ctx context.Context, vault vaultsync.ID) (Invitation, *vaultsync.MembershipState, error) {
	own, _, err := e.verifiedDeviceLog(ctx)
	if err != nil {
		return Invitation{}, nil, err
	}
	t, err := e.newMemberTrust(ctx, own)
	if err != nil {
		return Invitation{}, nil, err
	}
	ops, err := e.d.Server.Membership(ctx, vault, 0)
	if err != nil {
		return Invitation{}, nil, err
	}
	m, err := vaultsync.NewMembershipState(vault)
	if err != nil {
		return Invitation{}, nil, err
	}
	self := e.d.Keys.Account()
	var inviter vaultsync.ID
	for i := range ops {
		if err := m.Apply(&ops[i], t.check); err != nil {
			return Invitation{}, nil, integrity("invitation membership", err)
		}
		t.observe(&ops[i])
		if ops[i].Kind == vaultsync.MemberAdd && ops[i].Subject == self {
			inviter = ops[i].Author
		}
	}
	if m.Deleted() || m.Role(self) == vaultsync.RoleNone || inviter.IsZero() {
		return Invitation{}, nil, ErrNoInvitation
	}
	inv := Invitation{Vault: vault, Role: m.Role(self), Inviter: inviter, MembershipSeq: m.Seq(),
		InviterFingerprint: t.fingerprint(inviter), Name: e.invitationName(ctx, vault, t, ops)}
	return e.withContact(inv), m, nil
}

// withContact fills in what this user's contacts say of an invitation's inviter.
func (e *SyncEngine) withContact(inv Invitation) Invitation {
	inv.InviterUsername, inv.InviterKnown, inv.InviterVerified = "", false, false
	if c, ok := e.contact(inv.Inviter); ok {
		inv.InviterUsername, inv.InviterKnown, inv.InviterVerified = c.Username, true, c.Verified
	}
	return inv
}

// invitationName reads the team's name from its vault-meta record, which this account's key wrap
// already opens. It is a label for the person deciding, never trusted for anything else, and an
// empty one when it does not verify or open.
func (e *SyncEngine) invitationName(ctx context.Context, vault vaultsync.ID, t *memberTrust, ops []vaultsync.MemberOp) string {
	versions, err := e.d.Server.Versions(ctx, vault, synctrust.VaultMetaID(vault))
	if err != nil || len(versions) == 0 {
		return ""
	}
	meta := versions[len(versions)-1]
	_, key, err := t.resolve(meta.Author)
	if err != nil || meta.Vault != vault || meta.Verify(key) != nil {
		return ""
	}
	vk, err := e.d.Keys.VaultKey(vault, meta.Epoch, ops)
	if err != nil {
		return ""
	}
	defer clear(vk)
	raw, err := e.d.Cipher.Open(vk, &meta)
	if err != nil {
		return ""
	}
	body, err := synctrust.DecodeBody(raw)
	if err != nil {
		return ""
	}
	return synctrust.VaultMetaName(body)
}

// AcceptTeam joins a team this account was invited to. The invitation is verified again, then the
// join is recorded in the personal vault; the next round syncs the team in and tells the account's
// other devices.
func (e *SyncEngine) AcceptTeam(ctx context.Context, vault vaultsync.ID) error {
	if err := e.requirePending(ctx, vault); err != nil {
		return err
	}
	if _, _, err := e.inspectInvitation(ctx, vault); err != nil {
		return err
	}
	return e.recordTeams(ctx, vault)
}

// DeclineTeam answers an invitation by leaving the team: a signed removal of this account at the
// log position the invitation was verified at. Nothing of the team was ever applied here, so there
// is nothing to drop.
func (e *SyncEngine) DeclineTeam(ctx context.Context, vault vaultsync.ID) error {
	if err := e.requirePending(ctx, vault); err != nil {
		return err
	}
	_, m, err := e.inspectInvitation(ctx, vault)
	if err != nil {
		return err
	}
	p := &pullRun{e: e, vault: vault}
	return p.appendOp(ctx, m, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: e.d.Keys.Account()})
}

// requirePending refuses an answer to anything but a team the server lists and this account has not
// joined.
func (e *SyncEngine) requirePending(ctx context.Context, vault vaultsync.ID) error {
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	listed, err := e.d.Server.Vaults(ctx)
	if err != nil {
		return err
	}
	_, pending, err := e.joinedTeams(ctx, listed)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(pending, func(v synctrust.RemoteVault) bool { return v.Vault == vault }) {
		return ErrNoInvitation
	}
	return nil
}
