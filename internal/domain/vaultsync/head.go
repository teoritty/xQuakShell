package vaultsync

import "fmt"

// Head is the signed summary every commit carries (spec section 7.1). It links to the previous
// head, commits to the whole vault state through MerkleRoot, and pins the membership state the
// commit was made under, so a server can neither reorder history nor replay a commit under a
// different set of members.
type Head struct {
	Vault               ID
	Seq                 uint64
	PrevHash            [32]byte
	MerkleRoot          [32]byte
	CurrentEpoch        uint32
	MembershipSeq       uint64
	MembershipHead      [32]byte
	Commit              ID
	Author              ID
	ClientTimeUnixMilli uint64
	SigAlg              SigAlg
	Sig                 []byte
}

// SigningBytes is the canonical encoding the committing device signs.
func (h *Head) SigningBytes() ([]byte, error) {
	return NewCanon(LabelHeadSig).ID(h.Vault).U64(h.Seq).Hash(h.PrevHash).Hash(h.MerkleRoot).
		U32(h.CurrentEpoch).U64(h.MembershipSeq).Hash(h.MembershipHead).ID(h.Commit).ID(h.Author).
		U64(h.ClientTimeUnixMilli).U16(uint16(h.SigAlg)).Encoded()
}

// Hash identifies this head for the next one's PrevHash; it covers the signature too.
func (h *Head) Hash() ([32]byte, error) {
	msg, err := h.SigningBytes()
	if err != nil {
		return [32]byte{}, err
	}
	return NewCanon(LabelHeadHash).Bytes(msg).Bytes(h.Sig).Sum()
}

// Verify checks the author device's signature. Whether that device may write to this vault is the
// membership log's question, answered by the caller.
func (h *Head) Verify(publicKey []byte) error {
	msg, err := h.SigningBytes()
	if err != nil {
		return err
	}
	return VerifySignature(h.SigAlg, publicKey, msg, h.Sig)
}

// ChainCursor is the last head a client verified for a vault, persisted between syncs.
type ChainCursor struct {
	Seq           uint64
	Hash          [32]byte
	MembershipSeq uint64
}

// GenesisCursor is the position before a vault's first commit. It is derived from the vault id so
// the first commit of one vault can never be presented as the first commit of another.
func GenesisCursor(vault ID) (ChainCursor, error) {
	h, err := NewCanon(LabelHeadHash).ID(vault).Sum()
	if err != nil {
		return ChainCursor{}, err
	}
	return ChainCursor{Hash: h}, nil
}

// VerifyLink checks that next directly extends the cursor and does not claim an older membership
// state than the commit before it.
func VerifyLink(cur ChainCursor, next *Head) error {
	if next.Seq != cur.Seq+1 {
		return fmt.Errorf("seq %d after %d: %w", next.Seq, cur.Seq, ErrChainBroken)
	}
	if next.PrevHash != cur.Hash {
		return fmt.Errorf("seq %d: %w", next.Seq, ErrChainBroken)
	}
	if next.MembershipSeq < cur.MembershipSeq {
		return fmt.Errorf("membership %d after %d: %w", next.MembershipSeq, cur.MembershipSeq, ErrMembershipRegressed)
	}
	return nil
}

// Advance returns the cursor after next, or the reason next does not extend this one.
func (c ChainCursor) Advance(next *Head) (ChainCursor, error) {
	if err := VerifyLink(c, next); err != nil {
		return ChainCursor{}, err
	}
	h, err := next.Hash()
	if err != nil {
		return ChainCursor{}, err
	}
	return ChainCursor{Seq: next.Seq, Hash: h, MembershipSeq: next.MembershipSeq}, nil
}

// CheckObserved compares a head the server claims against the client's verified cursor. An older
// seq is a rollback; the same seq with another hash means the server has shown someone a different
// history - a fork.
func CheckObserved(cur ChainCursor, seq uint64, hash [32]byte) error {
	if seq < cur.Seq {
		return fmt.Errorf("seq %d below verified %d: %w", seq, cur.Seq, ErrRollback)
	}
	if seq == cur.Seq && hash != cur.Hash {
		return fmt.Errorf("seq %d: %w", seq, ErrFork)
	}
	return nil
}
