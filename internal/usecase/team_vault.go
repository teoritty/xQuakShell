package usecase

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrInviteKeyChanged is an invite whose account key is not the one the admin compared: the
	// server now answers for the username with another key.
	ErrInviteKeyChanged = errors.New("sync: that account's key is not the one you compared")
	// ErrContactMismatch is an invite of an account whose key differs from the contact this user
	// already holds for it; the new key must be verified before anything is shared with it.
	ErrContactMismatch = errors.New("sync: that account's key changed since you last verified it")
	// ErrNotATeam is a team operation on the personal vault.
	ErrNotATeam = errors.New("sync: the personal vault has no members")
)

// teamRetentionDays is the trash window a new team vault starts with (spec D6).
const teamRetentionDays = 30

// Invitee is an account found by username, as the inviting admin compares it.
type Invitee struct {
	Account     vaultsync.ID
	Username    string
	Fingerprint string
	// Known and Verified say whether this user already holds a contact for the account, and
	// whether that contact was compared out of band.
	Known, Verified bool
}

// MemberInfo is one member of a team vault as its members see it.
type MemberInfo struct {
	Account     vaultsync.ID
	Username    string
	Fingerprint string
	Role        vaultsync.Role
	Verified    bool
	Current     bool
}

// CreateTeam creates a team vault owned by this account, with name in its vault-meta.
func (e *SyncEngine) CreateTeam(ctx context.Context, name string, random io.Reader) (vaultsync.ID, error) {
	if e.d.Crypto == nil {
		return vaultsync.ID{}, ErrNoCrypto
	}
	vault, err := vaultsync.NewID(random)
	if err != nil {
		return vaultsync.ID{}, err
	}
	vk := make([]byte, 32)
	if _, err := io.ReadFull(random, vk); err != nil {
		return vaultsync.ID{}, err
	}
	defer clear(vk)
	op, err := e.genesisOp(vault, vk)
	if err != nil {
		return vaultsync.ID{}, err
	}
	if err := e.d.Server.CreateVault(ctx, op); err != nil {
		return vaultsync.ID{}, err
	}
	return vault, e.ensureVaultMeta(ctx, vault, random, name)
}

func (e *SyncEngine) genesisOp(vault vaultsync.ID, vk []byte) (vaultsync.MemberOp, error) {
	account := e.d.Keys.Account()
	_, enc := e.d.Keys.CurrentKeys()
	wrapped, err := e.d.Crypto.WrapVaultKey(enc, vault, 0, account, vk)
	if err != nil {
		return vaultsync.MemberOp{}, err
	}
	wrap, err := vaultsync.NewKeyWrap(account, wrapped)
	if err != nil {
		return vaultsync.MemberOp{}, err
	}
	genesis, err := vaultsync.NewMembershipState(vault)
	if err != nil {
		return vaultsync.MemberOp{}, err
	}
	op := vaultsync.MemberOp{Vault: vault, Seq: 1, Prev: genesis.Hash(), Kind: vaultsync.MemberCreate, Author: account,
		AuthorDevice: e.d.Keys.Device(), Suite: vaultsync.Suite1, RetentionDays: teamRetentionDays, Wraps: []vaultsync.KeyWrap{wrap},
		TimeUnixMilli: uint64(max(e.d.NowUnixMilli(), 0)), SigAlg: vaultsync.SigEd25519}
	msg, err := op.SigningBytes()
	if err != nil {
		return op, err
	}
	op.Sig, err = e.d.Keys.Sign(msg)
	return op, err
}

// LookupInvitee finds an account to invite and the fingerprint the admin must compare.
func (e *SyncEngine) LookupInvitee(ctx context.Context, username string) (Invitee, error) {
	account, log, err := e.inviteeLog(ctx, username)
	if err != nil {
		return Invitee{}, err
	}
	inv := Invitee{Account: account, Username: username, Fingerprint: vaultsync.AccountFingerprint(log.genesis)}
	if c, ok := e.contact(account); ok {
		inv.Known, inv.Verified = true, c.Verified
	}
	return inv, nil
}

// invited is an invitee's log replayed by this device from the genesis keys the server returned.
type invited struct {
	genesis []byte
	state   *vaultsync.DeviceLogState
}

// inviteeLog replays an account's whole log itself: the encryption key a vault key is wrapped to is
// the one this replay ends with, never one the server names.
func (e *SyncEngine) inviteeLog(ctx context.Context, username string) (vaultsync.ID, invited, error) {
	account, ml, err := e.d.Server.Lookup(ctx, username)
	if err != nil {
		return vaultsync.ID{}, invited{}, err
	}
	st, err := vaultsync.NewDeviceLogState(account, ml.Suite, ml.IdentityKey, ml.EncryptionKey)
	if err != nil {
		return vaultsync.ID{}, invited{}, integrity("invitee device log", err)
	}
	for i := range ml.Entries {
		if err := st.Apply(&ml.Entries[i]); err != nil {
			return vaultsync.ID{}, invited{}, integrity("invitee device log", err)
		}
	}
	if st.PendingSuccession() {
		return vaultsync.ID{}, invited{}, integrity("invitee device log", fmt.Errorf("a revocation without its succession"))
	}
	return account, invited{genesis: ml.IdentityKey, state: st}, nil
}

// InviteRequest names who to add to which team vault and how. Fingerprint is what the admin was
// shown and compared; Verified says the comparison was made out of band.
type InviteRequest struct {
	Vault       vaultsync.ID
	Username    string
	Fingerprint string
	Role        vaultsync.Role
	Verified    bool
}

// Invite adds an account to a team vault with role, wrapping the vault's current key to the
// encryption key its replayed log ends with. fingerprint is what the admin was shown; an account
// whose key no longer has it is refused, and so is one that contradicts a stored contact.
func (e *SyncEngine) Invite(ctx context.Context, req InviteRequest) error {
	vault := req.Vault
	if e.scopeOf(vault).Vault == "" {
		return ErrNotATeam
	}
	account, inv, err := e.inviteeLog(ctx, req.Username)
	if err != nil {
		return err
	}
	if vaultsync.AccountFingerprint(inv.genesis) != req.Fingerprint {
		return ErrInviteKeyChanged
	}
	digest, err := vaultsync.IdentityDigest(inv.genesis)
	if err != nil {
		return err
	}
	if c, ok := e.contact(account); ok && c.IdentityDigest != hex.EncodeToString(digest[:]) {
		return ErrContactMismatch
	}
	p, m, err := e.caughtUp(ctx, vault)
	if err != nil {
		return err
	}
	wrap, err := e.wrapTo(vault, m.Epoch(), p.ops, account, inv.state.EncryptionKey())
	if err != nil {
		return err
	}
	op := vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: account, Role: req.Role, Verified: req.Verified, Fingerprint: digest,
		Epoch: m.Epoch(), Wraps: []vaultsync.KeyWrap{wrap}}
	if err := p.appendOp(ctx, m, op); err != nil {
		return err
	}
	return e.storeContact(ctx, domain.SyncContact{Account: account.String(), IdentityDigest: hex.EncodeToString(digest[:]),
		Username: req.Username, Verified: req.Verified})
}

// wrapTo wraps the vault key of epoch to a member's encryption key.
func (e *SyncEngine) wrapTo(vault vaultsync.ID, epoch uint32, ops []vaultsync.MemberOp, member vaultsync.ID, encKey []byte) (vaultsync.KeyWrap, error) {
	if e.d.Crypto == nil {
		return vaultsync.KeyWrap{}, ErrNoCrypto
	}
	vk, err := e.d.Keys.VaultKey(vault, epoch, ops)
	if err != nil {
		return vaultsync.KeyWrap{}, err
	}
	defer clear(vk)
	wrapped, err := e.d.Crypto.WrapVaultKey(encKey, vault, epoch, member, vk)
	if err != nil {
		return vaultsync.KeyWrap{}, err
	}
	return vaultsync.NewKeyWrap(member, wrapped)
}

func (e *SyncEngine) contact(account vaultsync.ID) (domain.SyncContact, bool) {
	data, err := e.d.Vault.GetData()
	if err != nil || data.Sync == nil {
		return domain.SyncContact{}, false
	}
	for _, c := range data.Sync.Contacts {
		if c.Account == account.String() {
			return c, true
		}
	}
	return domain.SyncContact{}, false
}

// storeContact writes a contact into the personal vault, keeping a verification already made.
func (e *SyncEngine) storeContact(ctx context.Context, c domain.SyncContact) error {
	if old, ok := e.contact(mustParseID(c.Account)); ok && old.Verified {
		c.Verified = true
	}
	return e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		return synctrust.Apply(d, []synctrust.Entry{contactEntry(c)})
	})
}

func mustParseID(s string) vaultsync.ID {
	id, _ := vaultsync.ParseID(s) // Contacts are written from parsed ids only.
	return id
}

// Members lists a team vault's members from its verified membership log.
func (e *SyncEngine) Members(ctx context.Context, vault vaultsync.ID) ([]MemberInfo, error) {
	p, m, err := withMembers(e.startPull(ctx, vault))
	if err != nil {
		return nil, err
	}
	if p.trust == nil {
		return nil, ErrNotATeam
	}
	verified := map[vaultsync.ID]bool{}
	for _, op := range p.ops {
		if op.Kind == vaultsync.MemberAdd {
			verified[op.Subject] = op.Verified
		}
	}
	self := e.d.Keys.Account()
	var out []MemberInfo
	for _, account := range m.Members() {
		info := MemberInfo{Account: account, Role: m.Role(account), Verified: verified[account], Current: account == self}
		if c, ok := e.contact(account); ok {
			info.Username, info.Verified = c.Username, info.Verified || c.Verified
		}
		info.Fingerprint = p.trust.fingerprint(account)
		out = append(out, info)
	}
	return out, nil
}

// fingerprint renders a member's genesis key for people to compare; empty when its log is not
// loaded and cannot be.
func (t *memberTrust) fingerprint(account vaultsync.ID) string {
	if account == t.e.d.Keys.Account() {
		_, identity, _ := t.e.d.Keys.AccountKeys()
		return vaultsync.AccountFingerprint(identity)
	}
	if _, err := t.logFor(account); err != nil {
		return ""
	}
	return vaultsync.AccountFingerprint(ed25519.PublicKey(t.genesis[account]))
}

// TeamName reads a team vault's name from its vault-meta.
func (e *SyncEngine) TeamName(ctx context.Context, vault vaultsync.ID) (string, error) {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return "", err
	}
	meta, ok := p.base[synctrust.VaultMetaID(vault)]
	if !ok {
		return "", nil
	}
	body, err := p.open(&meta)
	if err != nil {
		return "", integrity("vault-meta", err)
	}
	return synctrust.VaultMetaName(body), nil
}
