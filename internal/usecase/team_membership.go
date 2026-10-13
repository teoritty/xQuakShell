package usecase

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// ChangeRole sets a member's role in a team vault.
func (e *SyncEngine) ChangeRole(ctx context.Context, vault, account vaultsync.ID, role vaultsync.Role) error {
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	return p.appendOp(ctx, m, vaultsync.MemberOp{Kind: vaultsync.MemberChangeRole, Subject: account, Role: role})
}

// RemoveMember removes an account and rotates the vault key in the same append (spec §5.9): the new
// key is wrapped to every remaining member's current encryption key, from their verified logs, and
// every live record is then re-encrypted under it.
func (e *SyncEngine) RemoveMember(ctx context.Context, vault, account vaultsync.ID, random io.Reader) error {
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	remaining := slices.DeleteFunc(m.Members(), func(a vaultsync.ID) bool { return a == account })
	rotate, err := p.rotationFor(m, remaining, random)
	if err != nil {
		return err
	}
	if err := p.appendOps(ctx, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: account}, rotate); err != nil {
		return err
	}
	return e.reencrypt(ctx, vault)
}

// LeaveTeam removes this account from a team vault. It does not rotate - it would know the new key -
// and leaves that to the next admin device that syncs; the local copy of the team goes at once.
func (e *SyncEngine) LeaveTeam(ctx context.Context, vault vaultsync.ID) error {
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	if err := p.appendOp(ctx, m, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: e.d.Keys.Account()}); err != nil {
		return err
	}
	return e.forgetTeam(ctx, vault)
}

// rotationFor builds a rotate-epoch op wrapping a fresh vault key to each of members, dated no
// earlier than the previous rotation.
func (p *pullRun) rotationFor(m *vaultsync.MembershipState, members []vaultsync.ID, random io.Reader) (vaultsync.MemberOp, error) {
	if p.e.d.Crypto == nil {
		return vaultsync.MemberOp{}, ErrNoCrypto
	}
	vk := make([]byte, 32)
	if _, err := io.ReadFull(random, vk); err != nil {
		return vaultsync.MemberOp{}, err
	}
	defer clear(vk)
	epoch := m.Epoch() + 1
	wraps := make([]vaultsync.KeyWrap, 0, len(members))
	for _, member := range members {
		enc, err := p.encryptionKeyOf(member)
		if err != nil {
			return vaultsync.MemberOp{}, err
		}
		wrapped, err := p.e.d.Crypto.WrapVaultKey(enc, p.vault, epoch, member, vk)
		if err != nil {
			return vaultsync.MemberOp{}, err
		}
		w, err := vaultsync.NewKeyWrap(member, enc, wrapped)
		if err != nil {
			return vaultsync.MemberOp{}, err
		}
		wraps = append(wraps, w)
	}
	rotations := m.Rotations()
	return vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: epoch, Wraps: wraps,
		TimeUnixMilli: rotations[len(rotations)-1].StartedUnixMilli}, nil
}

// encryptionKeyOf is a member's current encryption key: this device's own, or the one a member's
// verified log ends with.
func (p *pullRun) encryptionKeyOf(member vaultsync.ID) ([]byte, error) {
	if member == p.e.d.Keys.Account() {
		_, enc := p.e.d.Keys.CurrentKeys()
		return enc, nil
	}
	if p.trust == nil {
		return nil, ErrNotATeam
	}
	log, err := p.trust.logFor(member)
	if err != nil {
		return nil, err
	}
	return log.EncryptionKey(), nil
}

// appendOps signs ops in sequence at the next positions of the membership log and sends them as
// one atomic append, each op chained to the one before it as the log will hold them.
func (p *pullRun) appendOps(ctx context.Context, ops ...vaultsync.MemberOp) error {
	scratch, err := p.scratchMembership()
	if err != nil {
		return err
	}
	signed := make([]vaultsync.MemberOp, 0, len(ops))
	for _, op := range ops {
		op.Vault, op.Seq, op.Prev = p.vault, scratch.Seq()+1, scratch.Hash()
		op.Author, op.AuthorDevice, op.Suite, op.SigAlg = p.e.d.Keys.Account(), p.e.d.Keys.Device(), vaultsync.Suite1, vaultsync.SigEd25519
		op.TimeUnixMilli = max(uint64(max(p.e.d.NowUnixMilli(), 0)), op.TimeUnixMilli)
		msg, err := op.SigningBytes()
		if err != nil {
			return err
		}
		if op.Sig, err = p.e.d.Keys.Sign(msg); err != nil {
			return err
		}
		if err := scratch.Apply(&op, p.members.check); err != nil {
			return fmt.Errorf("membership op %d: %w", op.Kind, err)
		}
		signed = append(signed, op)
	}
	if err := p.e.d.Server.AppendMembership(ctx, p.vault, signed); err != nil {
		return fmt.Errorf("membership ops: %w", err)
	}
	return nil
}

// scratchMembership replays the verified log into a state of its own, so signing a batch does not
// move the run's own cursor.
func (p *pullRun) scratchMembership() (*vaultsync.MembershipState, error) {
	s, err := vaultsync.NewMembershipState(p.vault)
	if err != nil {
		return nil, err
	}
	for i := range p.ops {
		if err := s.Apply(&p.ops[i], p.members.check); err != nil {
			return nil, integrity("membership", err)
		}
	}
	return s, nil
}

// SetTeamPolicy sets a team vault's retention window and whether it admits unverified members.
func (e *SyncEngine) SetTeamPolicy(ctx context.Context, vault vaultsync.ID, retentionDays uint16, requireVerified bool) error {
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	return p.appendOp(ctx, m, vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: retentionDays, RequireVerified: requireVerified})
}

// VerifyMember records that this user compared a member's fingerprint out of band. The fingerprint
// is checked against the member's genesis key as this device verified it, never taken on trust.
func (e *SyncEngine) VerifyMember(ctx context.Context, vault, account vaultsync.ID, fingerprint string) error {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return err
	}
	if p.trust == nil {
		return ErrNotATeam
	}
	if _, err := p.trust.logFor(account); err != nil {
		return err
	}
	genesis := p.trust.genesis[account]
	if vaultsync.AccountFingerprint(genesis) != fingerprint {
		return ErrInviteKeyChanged
	}
	digest, err := vaultsync.IdentityDigest(genesis)
	if err != nil {
		return err
	}
	c, _ := e.contact(account)
	c.Account, c.IdentityDigest, c.Verified = account.String(), hex.EncodeToString(digest[:]), true
	return e.storeContact(ctx, c)
}

// MoveConnectionToTeam moves a personal connection into a team vault this account may write to. The
// host is shared, the credential is not (spec §6.7): every user named with an inline key or password
// becomes a personal-source user, and this account binds it to the credential it used before, which
// stays in the personal vault. The connection lands at the team's top level; the next round pushes
// it to the team.
func (e *SyncEngine) MoveConnectionToTeam(ctx context.Context, connection string, vault vaultsync.ID) error {
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	_, m, err := withMembers(e.startPull(ctx, vault))
	if err != nil {
		return err
	}
	if !m.Role(e.d.Keys.Account()).CanWrite() {
		return vaultsync.ErrNotAuthorized
	}
	return e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		i := slices.IndexFunc(d.Connections, func(c domain.Connection) bool { return c.ID == connection && c.VaultID == "" })
		if i < 0 {
			return domain.ErrConnectionNotFound
		}
		c := &d.Connections[i]
		c.VaultID, c.FolderID = vault.String(), ""
		for j := range c.Users {
			if b, ok := bindingFor(c, &c.Users[j]); ok {
				d.Sync = ensureSync(d.Sync)
				d.Sync.Bindings = append(slices.DeleteFunc(d.Sync.Bindings, func(x domain.CredentialBinding) bool { return x.Key() == b.Key() }), b)
			}
		}
		return nil
	})
}

// bindingFor turns an inline key or password of a user into this account's binding, clearing the
// reference from the shared user. Plugin authentication carries no personal credential to bind.
func bindingFor(c *domain.Connection, u *domain.ConnectionUser) (domain.CredentialBinding, bool) {
	b := domain.CredentialBinding{VaultID: c.VaultID, ConnectionID: c.ID, UserID: u.ID}
	switch {
	case u.Auth == domain.AuthMethodKey && u.KeyAuth != nil && len(u.KeyAuth.IdentityIDs) > 0:
		b.IdentityID = u.KeyAuth.IdentityIDs[0]
	case u.Auth == domain.AuthMethodPassword && u.PassAuth != nil && u.PassAuth.VaultRef != "":
		b.PasswordID = u.PassAuth.VaultRef
	default:
		return b, false
	}
	u.KeyAuth, u.PassAuth, u.CredentialSource = nil, nil, domain.CredentialSourcePersonal
	return b, true
}

func ensureSync(s *domain.VaultSync) *domain.VaultSync {
	if s == nil {
		return &domain.VaultSync{}
	}
	return s
}
