package vaultsync

import (
	"bytes"
	"slices"
)

// Leaf is one record's entry in a vault state: its latest version and that version's envelope
// digest. Tombstones are leaves too until purged, so a server cannot make a deletion disappear.
type Leaf struct {
	Record  ID
	Version uint64
	Digest  [32]byte
}

// LeafOf derives the leaf for an envelope.
func LeafOf(e *Envelope) (Leaf, error) {
	d, err := e.Digest()
	if err != nil {
		return Leaf{}, err
	}
	return Leaf{Record: e.Record, Version: e.Version, Digest: d}, nil
}

// MerkleRoot commits to a whole vault state. Leaves are sorted by record id, so the root depends on
// the state and not on the order a device happened to hold it in. Leaf and node hashes use
// different labels, which is what stops an inner node being presented as a leaf; an odd node is
// promoted unchanged rather than duplicated, because duplication makes two different leaf lists
// share a root. A duplicate record id is refused: one state never holds a record twice.
func MerkleRoot(leaves []Leaf) ([32]byte, error) {
	sorted := slices.Clone(leaves)
	slices.SortFunc(sorted, func(a, b Leaf) int { return bytes.Compare(a.Record[:], b.Record[:]) })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Record == sorted[i-1].Record {
			return [32]byte{}, ErrDuplicateLeaf
		}
	}
	if len(sorted) == 0 {
		return NewCanon(LabelMerkleNode).Sum()
	}
	level := make([][32]byte, len(sorted))
	for i, l := range sorted {
		h, err := NewCanon(LabelMerkleLeaf).ID(l.Record).U64(l.Version).Hash(l.Digest).Sum()
		if err != nil {
			return [32]byte{}, err
		}
		level[i] = h
	}
	for len(level) > 1 {
		next, err := merkleLevel(level)
		if err != nil {
			return [32]byte{}, err
		}
		level = next
	}
	return level[0], nil
}

func merkleLevel(level [][32]byte) ([][32]byte, error) {
	next := make([][32]byte, 0, (len(level)+1)/2)
	for i := 0; i < len(level); i += 2 {
		if i+1 == len(level) {
			next = append(next, level[i])
			continue
		}
		h, err := NewCanon(LabelMerkleNode).Hash(level[i]).Hash(level[i+1]).Sum()
		if err != nil {
			return nil, err
		}
		next = append(next, h)
	}
	return next, nil
}
