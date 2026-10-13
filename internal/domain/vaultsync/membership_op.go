package vaultsync

import (
	"fmt"
	"slices"
)

// Role is what a member may do in a vault. The order matters: each role includes the ones below.
type Role uint8

const (
	RoleNone Role = iota
	RoleViewer
	RoleEditor
	RoleAdmin
	RoleOwner
)

// CanWrite reports whether the role may commit records. A viewer can still read every secret: it
// holds the vault key, and no cryptography grants use without sight.
func (r Role) CanWrite() bool { return r >= RoleEditor }

// CanManage reports whether the role may change membership and rotate keys.
func (r Role) CanManage() bool { return r >= RoleAdmin }

// MemberOpKind is the kind of a membership operation (spec section 5.9).
type MemberOpKind uint8

const (
	MemberCreate MemberOpKind = iota + 1
	MemberAdd
	MemberChangeRole
	MemberRemove
	MemberRotateEpoch
	MemberRewrap
	MemberSetPolicy
	MemberTransferOwnership
	MemberDeleteVault
)

// KeyWrap is a vault key HPKE-sealed to one member's account encryption key. A membership op signs
// the member and the digest, not the ciphertext, so the server can strip the ciphertext of a retired
// epoch's wraps (spec §10.1) and the log still verifies: a stripped wrap is pruned, valid in a
// replayed log and useless for opening anything.
type KeyWrap struct {
	Member     ID
	Digest     [32]byte
	Ciphertext []byte
}

// NewKeyWrap binds a sealed vault key to its digest. An empty ciphertext is refused: it would be
// indistinguishable from a pruned wrap.
func NewKeyWrap(member ID, ciphertext []byte) (KeyWrap, error) {
	if len(ciphertext) == 0 {
		return KeyWrap{}, rejected("empty wrap for %s", member)
	}
	d, err := wrapDigest(ciphertext)
	if err != nil {
		return KeyWrap{}, err
	}
	return KeyWrap{Member: member, Digest: d, Ciphertext: slices.Clone(ciphertext)}, nil
}

func wrapDigest(ciphertext []byte) ([32]byte, error) {
	return NewCanon(LabelVKWrapDigest).Bytes(ciphertext).Sum()
}

// Pruned reports a wrap whose ciphertext the server removed after its epoch retired.
func (w KeyWrap) Pruned() bool { return len(w.Ciphertext) == 0 }

// Strip is the wrap as the server keeps it once the epoch retired: the signed digest without the key.
func (w KeyWrap) Strip() KeyWrap { return KeyWrap{Member: w.Member, Digest: w.Digest} }

// MemberOp is one link of a vault's membership log. Fields a kind does not use stay zero and are
// still signed, so every kind signs one fixed layout.
type MemberOp struct {
	Vault           ID
	Seq             uint64
	Prev            [32]byte
	Kind            MemberOpKind
	Author          ID
	AuthorDevice    ID
	Subject         ID
	Role            Role
	Verified        bool
	Fingerprint     [32]byte
	Suite           SuiteID
	Epoch           uint32
	Wraps           []KeyWrap
	RevokedDevices  []ID
	RetentionDays   uint16
	RequireVerified bool
	// Shred, on a rotation only, retires every older epoch as soon as no current version uses it.
	Shred         bool
	TimeUnixMilli uint64
	SigAlg        SigAlg
	Sig           []byte
}

// SigningBytes encodes every field except the signature.
func (op *MemberOp) SigningBytes() ([]byte, error) {
	c := NewCanon(LabelMemberSig).ID(op.Vault).U64(op.Seq).Hash(op.Prev).U16(uint16(op.Kind)).
		ID(op.Author).ID(op.AuthorDevice).ID(op.Subject).U16(uint16(op.Role)).Bool(op.Verified).
		Hash(op.Fingerprint).U16(uint16(op.Suite)).U32(op.Epoch)
	c.U64(uint64(len(op.Wraps)))
	for _, w := range op.Wraps {
		c.ID(w.Member).Hash(w.Digest)
	}
	c.IDs(op.RevokedDevices).U16(op.RetentionDays).Bool(op.RequireVerified).Bool(op.Shred).U64(op.TimeUnixMilli).U16(uint16(op.SigAlg))
	return c.Encoded()
}

func (op *MemberOp) hash(msg []byte) ([32]byte, error) {
	return NewCanon(LabelMemberHash).Bytes(msg).Bytes(op.Sig).Sum()
}

func rejected(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMembership, fmt.Sprintf(format, args...))
}
