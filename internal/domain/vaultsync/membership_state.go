package vaultsync

import (
	"bytes"
	"maps"
	"slices"
)

// SignatureCheck verifies that device belongs to account and signed message. The caller resolves
// the device key through the author's device log; keeping that lookup outside lets the membership
// rules be tested and reused without a log of every account in the vault.
type SignatureCheck func(account, device ID, alg SigAlg, message, signature []byte) error

// MembershipState is a vault's membership log replayed from genesis.
type MembershipState struct {
	vault           ID
	seq             uint64
	hash            [32]byte
	members         map[ID]Role
	owner           ID
	creator         ID
	fingerprints    map[ID][32]byte
	epoch           uint32
	revoked         map[ID]struct{}
	retentionDays   uint16
	requireVerified bool
	deleted         bool
	pendingRotation bool
	rotations       []Rotation
	// shredSeq is the log position of the latest rotation that carried shred, zero before any.
	shredSeq uint64
	params   SuiteParams
}

// NewMembershipState starts a log at its genesis position.
func NewMembershipState(vault ID) (*MembershipState, error) {
	genesis, err := NewCanon(LabelMemberHash).ID(vault).Sum()
	if err != nil {
		return nil, err
	}
	return &MembershipState{vault: vault, hash: genesis, members: map[ID]Role{}, revoked: map[ID]struct{}{},
		fingerprints: map[ID][32]byte{}}, nil
}

// Apply verifies one operation and applies it to a copy, replacing the state only when every check
// passes. A rejected op therefore leaves the state exactly as it was.
func (s *MembershipState) Apply(op *MemberOp, check SignatureCheck) error {
	if op.Vault != s.vault || op.Seq != s.seq+1 || op.Prev != s.hash {
		return rejected("op does not extend the log at seq %d", s.seq)
	}
	msg, err := op.SigningBytes()
	if err != nil {
		return err
	}
	if _, revoked := s.revoked[op.AuthorDevice]; revoked {
		return ErrRevokedDevice
	}
	if err := check(op.Author, op.AuthorDevice, op.SigAlg, msg, op.Sig); err != nil {
		return err
	}
	params, err := s.preconditions(op)
	if err != nil {
		return err
	}
	next := s.clone()
	next.params = params
	if err := next.applyKind(op); err != nil {
		return err
	}
	h, err := op.hash(msg)
	if err != nil {
		return err
	}
	next.seq, next.hash = op.Seq, h
	*s = *next
	return nil
}

// preconditions checks what must hold before any kind-specific rule runs, and returns the suite
// the op's wraps are measured against. It reads the state and never writes it: Apply installs the
// suite on the copy, so a rejected op cannot leave even that behind.
func (s *MembershipState) preconditions(op *MemberOp) (SuiteParams, error) {
	if s.deleted {
		return SuiteParams{}, ErrVaultDeleted
	}
	if s.pendingRotation && op.Kind != MemberRotateEpoch {
		return SuiteParams{}, ErrRotationRequired
	}
	if (s.seq == 0) != (op.Kind == MemberCreate) {
		return SuiteParams{}, rejected("create must be the first op and only the first")
	}
	params, err := LookupSuite(op.Suite, MinAcceptedSuite)
	if err != nil {
		return SuiteParams{}, rejected("%v", err)
	}
	return params, nil
}

func (s *MembershipState) clone() *MembershipState {
	c := *s
	c.members = maps.Clone(s.members)
	c.fingerprints = maps.Clone(s.fingerprints)
	c.revoked = maps.Clone(s.revoked)
	c.rotations = slices.Clone(s.rotations)
	return &c
}

// Seq is the position of the last applied op.
func (s *MembershipState) Seq() uint64 { return s.seq }

// Hash is the chain hash a vault head's MembershipHead refers to.
func (s *MembershipState) Hash() [32]byte { return s.hash }

// Creator is the account that created the vault; ownership transfers do not change it. Nothing in
// the log anchors its keys, so a member trusts them through a contact (spec §6.6).
func (s *MembershipState) Creator() ID { return s.creator }

// MemberFingerprint is the identity-key digest the op that added a current member recorded.
func (s *MembershipState) MemberFingerprint(account ID) ([32]byte, bool) {
	fp, ok := s.fingerprints[account]
	return fp, ok && s.members[account] != RoleNone
}

// Role is an account's role, RoleNone for a non-member.
func (s *MembershipState) Role(account ID) Role { return s.members[account] }

// Epoch is the current vault key epoch.
func (s *MembershipState) Epoch() uint32 { return s.epoch }

// Owner is the single owning account.
func (s *MembershipState) Owner() ID { return s.owner }

// IsDeviceRevoked reports whether a rotation listed this device; commits by it at or after that
// membership position are refused.
func (s *MembershipState) IsDeviceRevoked(device ID) bool {
	_, ok := s.revoked[device]
	return ok
}

// RetentionDays is the vault's trash and history window.
func (s *MembershipState) RetentionDays() uint16 { return s.retentionDays }

// RequireVerified reports whether unverified members are refused.
func (s *MembershipState) RequireVerified() bool { return s.requireVerified }

// Rotations lists when each epoch began, epoch 0 at creation.
func (s *MembershipState) Rotations() []Rotation { return slices.Clone(s.rotations) }

// ShredSeq is the log position of the latest Shred now rotation, zero when there was none. A
// tombstone whose commit was made under an earlier membership position predates that shred, and
// its trash window no longer protects it.
func (s *MembershipState) ShredSeq() uint64 { return s.shredSeq }

// Deleted reports a deleted vault; nothing more is accepted.
func (s *MembershipState) Deleted() bool { return s.deleted }

// PendingRotation reports a removal not yet followed by a rotation.
func (s *MembershipState) PendingRotation() bool { return s.pendingRotation }

// Members lists member accounts in a stable order.
func (s *MembershipState) Members() []ID {
	ids := slices.Collect(maps.Keys(s.members))
	slices.SortFunc(ids, func(a, b ID) int { return bytes.Compare(a[:], b[:]) })
	return ids
}
