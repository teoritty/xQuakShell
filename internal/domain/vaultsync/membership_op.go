package vaultsync

import "fmt"

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

// KeyWrap is a vault key HPKE-sealed to one member's account encryption key.
type KeyWrap struct {
	Member     ID
	Ciphertext []byte
}

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
	TimeUnixMilli   uint64
	SigAlg          SigAlg
	Sig             []byte
}

// SigningBytes encodes every field except the signature.
func (op *MemberOp) SigningBytes() ([]byte, error) {
	c := NewCanon(LabelMemberSig).ID(op.Vault).U64(op.Seq).Hash(op.Prev).U16(uint16(op.Kind)).
		ID(op.Author).ID(op.AuthorDevice).ID(op.Subject).U16(uint16(op.Role)).Bool(op.Verified).
		Hash(op.Fingerprint).U16(uint16(op.Suite)).U32(op.Epoch)
	c.U64(uint64(len(op.Wraps)))
	for _, w := range op.Wraps {
		c.ID(w.Member).Bytes(w.Ciphertext)
	}
	c.IDs(op.RevokedDevices).U16(op.RetentionDays).Bool(op.RequireVerified).U64(op.TimeUnixMilli).U16(uint16(op.SigAlg))
	return c.Encoded()
}

func (op *MemberOp) hash(msg []byte) ([32]byte, error) {
	return NewCanon(LabelMemberHash).Bytes(msg).Bytes(op.Sig).Sum()
}

func rejected(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMembership, fmt.Sprintf(format, args...))
}
