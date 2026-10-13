package usecase

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// errMemberKey is a co-member's genesis key that does not match the fingerprint anchoring it.
var errMemberKey = errors.New("a member's account key does not match the fingerprint the vault recorded")

// memberTrust verifies the devices of a team vault's members (spec §5.9, §6.6). Each account's
// device log is replayed from genesis keys the server returns, trusted only when their identity key
// has the digest the membership log recorded when that account was added. The vault's creator,
// whom no op anchors, is checked against this user's contacts, and trusted on first use otherwise -
// the contact it then becomes holds that key from there on.
type memberTrust struct {
	e        *SyncEngine
	ctx      context.Context
	own      *vaultsync.DeviceLogState
	contacts []domain.SyncContact
	anchors  map[vaultsync.ID][32]byte
	creator  vaultsync.ID
	logs     map[vaultsync.ID]*vaultsync.DeviceLogState
	// firstUse is the creator contact to store once the membership log verified.
	firstUse *domain.SyncContact
}

func (e *SyncEngine) newMemberTrust(ctx context.Context, own *vaultsync.DeviceLogState) (*memberTrust, error) {
	data, err := e.d.Vault.GetData()
	if err != nil {
		return nil, err
	}
	t := &memberTrust{e: e, ctx: ctx, own: own, anchors: map[vaultsync.ID][32]byte{}, logs: map[vaultsync.ID]*vaultsync.DeviceLogState{}}
	if data.Sync != nil {
		t.contacts = data.Sync.Contacts
	}
	return t, nil
}

// observe learns the anchors an op establishes, after the op verified.
func (t *memberTrust) observe(op *vaultsync.MemberOp) {
	switch op.Kind {
	case vaultsync.MemberCreate:
		t.creator = op.Author
	case vaultsync.MemberAdd:
		t.anchors[op.Subject] = op.Fingerprint
	}
}

func (t *memberTrust) check(account, device vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
	// The first signature checked is the create op's: the membership rules refuse any other first
	// op, and observe runs only after an op verified, too late to name the creator for its own check.
	if t.creator.IsZero() {
		t.creator = account
	}
	log, err := t.logFor(account)
	if err != nil {
		return err
	}
	key, err := log.DeviceKey(device)
	if err != nil {
		return err
	}
	return vaultsync.VerifySignature(alg, key, msg, sig)
}

// resolve finds a device among the members' logs, loading the logs of members not yet seen.
func (t *memberTrust) resolve(device vaultsync.ID) (vaultsync.ID, []byte, error) {
	accounts := []vaultsync.ID{t.e.d.Keys.Account(), t.creator}
	for a := range t.anchors {
		accounts = append(accounts, a)
	}
	sortIDs(accounts[2:])
	for _, a := range slices.Compact(accounts) {
		if a.IsZero() {
			continue
		}
		log, err := t.logFor(a)
		if err != nil {
			return vaultsync.ID{}, nil, err
		}
		if key, err := log.DeviceKey(device); err == nil {
			return a, key, nil
		}
	}
	return vaultsync.ID{}, nil, fmt.Errorf("device %s: %w", device, vaultsync.ErrDeviceLog)
}

func (t *memberTrust) logFor(account vaultsync.ID) (*vaultsync.DeviceLogState, error) {
	if account == t.e.d.Keys.Account() {
		return t.own, nil
	}
	if log, ok := t.logs[account]; ok {
		return log, nil
	}
	ml, err := t.e.d.Server.MemberLog(t.ctx, account)
	if err != nil {
		return nil, err
	}
	if err := t.trustKey(account, ml.IdentityKey); err != nil {
		return nil, err
	}
	log, err := vaultsync.NewDeviceLogState(account, ml.Suite, ml.IdentityKey, ml.EncryptionKey)
	if err != nil {
		return nil, integrity("member device log", err)
	}
	for i := range ml.Entries {
		if err := log.Apply(&ml.Entries[i]); err != nil {
			return nil, integrity("member device log", err)
		}
	}
	if log.PendingSuccession() {
		return nil, integrity("member device log", fmt.Errorf("a revocation without its succession"))
	}
	t.logs[account] = log
	return log, nil
}

// trustKey accepts an account's genesis identity key only against an anchor: the fingerprint its
// add-member op recorded, or for the creator a contact - or none yet, which is first use.
func (t *memberTrust) trustKey(account vaultsync.ID, identityKey []byte) error {
	digest, err := vaultsync.IdentityDigest(identityKey)
	if err != nil {
		return integrity("member key", err)
	}
	if want, ok := t.anchors[account]; ok {
		if want != digest {
			return integrity("member key", errMemberKey)
		}
		return nil
	}
	if account != t.creator {
		return integrity("member key", fmt.Errorf("account %s is not a member", account))
	}
	hexDigest := hex.EncodeToString(digest[:])
	for _, c := range t.contacts {
		if c.Account == account.String() {
			if c.IdentityDigest != hexDigest {
				return integrity("member key", errMemberKey)
			}
			return nil
		}
	}
	t.firstUse = &domain.SyncContact{Account: account.String(), IdentityDigest: hexDigest}
	return nil
}

// storeFirstUse records the creator trusted on first use as a contact, once its vault verified.
func (t *memberTrust) storeFirstUse(ctx context.Context) error {
	if t.firstUse == nil {
		return nil
	}
	c := *t.firstUse
	return t.e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		return synctrust.Apply(d, []synctrust.Entry{contactEntry(c)})
	})
}

func contactEntry(c domain.SyncContact) synctrust.Entry {
	data, _ := json.Marshal(c) // A struct of strings and a bool always encodes.
	return synctrust.Entry{Type: synctrust.TypeContact, Key: c.Account, Data: data}
}

// vaultTrust verifies a vault's membership log and returns how its devices resolve: the personal
// vault through this account's log alone, a team vault through every member's.
func (e *SyncEngine) vaultTrust(ctx context.Context, vault vaultsync.ID, own *vaultsync.DeviceLogState) (vaultsync.DeviceResolver, vaultsync.SignatureCheck, []vaultsync.MemberOp, error) {
	if e.scopeOf(vault).Vault == "" {
		resolve := e.resolver(own)
		check := signatureCheck(resolve)
		ops, err := e.verifiedMembership(ctx, vault, check, nil)
		return resolve, check, ops, err
	}
	t, err := e.newMemberTrust(ctx, own)
	if err != nil {
		return nil, nil, nil, err
	}
	ops, err := e.verifiedMembership(ctx, vault, t.check, t.observe)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := t.storeFirstUse(ctx); err != nil {
		return nil, nil, nil, err
	}
	return t.resolve, t.check, ops, nil
}
