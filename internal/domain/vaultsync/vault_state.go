package vaultsync

import (
	"fmt"
	"maps"
	"slices"
)

// Commit is one step of a vault's feed: a signed head, the record versions it carries, and the
// tombstones it purges (spec §10.1). Purged needs no signature of its own: the head's Merkle root is
// over the state without those leaves, so a purge the author did not make fails the root check.
type Commit struct {
	Head    Head
	Records []Envelope
	Purged  []ID
}

// DeviceResolver returns the account a device belongs to and its signing key. The server answers
// from its devices table; a client answers from the device logs of the vault's members.
type DeviceResolver func(device ID) (account ID, signingKey []byte, err error)

// VaultState is a verified vault: the latest version of every record, tombstones included, and the
// chain cursor of the last commit that produced it.
type VaultState struct {
	vault  ID
	cursor ChainCursor
	leaves map[ID]Leaf
}

// NewVaultState is the state of a vault before its first commit.
func NewVaultState(vault ID) (*VaultState, error) {
	cur, err := GenesisCursor(vault)
	if err != nil {
		return nil, err
	}
	return &VaultState{vault: vault, cursor: cur, leaves: map[ID]Leaf{}}, nil
}

// Cursor is the position of the last applied commit.
func (s *VaultState) Cursor() ChainCursor { return s.cursor }

// Version is a record's latest version, if the vault holds it.
func (s *VaultState) Version(record ID) (uint64, bool) {
	l, ok := s.leaves[record]
	return l.Version, ok
}

// Leaves lists the state's leaves; order is irrelevant to MerkleRoot.
func (s *VaultState) Leaves() []Leaf { return slices.Collect(maps.Values(s.leaves)) }

// ApplyCommit verifies a commit against this state and the membership state its head names, and
// returns the state after it (spec section 7.3). It never mutates s, so a refused commit leaves
// nothing behind. The server runs it as defence in depth; for a client it is the authority.
//
// m must be the membership state at the head's membership position. The server always passes the
// current one, because it refuses commits made under an older one; a client replays the log to the
// position each head names.
func (s *VaultState) ApplyCommit(c Commit, m *MembershipState, resolve DeviceResolver) (*VaultState, error) {
	h := &c.Head
	if h.Vault != s.vault {
		return nil, fmt.Errorf("head of vault %s applied to %s: %w", h.Vault, s.vault, ErrChainBroken)
	}
	if err := VerifyLink(s.cursor, h); err != nil {
		return nil, err
	}
	if h.MembershipSeq != m.Seq() || h.MembershipHead != m.Hash() {
		return nil, ErrMembershipMismatch
	}
	account, key, err := authorizeAuthor(h, m, resolve)
	if err != nil {
		return nil, err
	}
	// A purge destroys every version of a record, the trash copy included, so it is the vault
	// managers' decision (spec §10.1) and never an editor's: an editor may delete, and the deletion
	// stays restorable for the retention window.
	if len(c.Purged) > 0 && !m.Role(account).CanManage() {
		return nil, fmt.Errorf("purge by account %s: %w", account, ErrNotAuthorized)
	}
	if h.CurrentEpoch != m.Epoch() {
		return nil, fmt.Errorf("head epoch %d, vault epoch %d: %w", h.CurrentEpoch, m.Epoch(), ErrEpochMismatch)
	}
	next := &VaultState{vault: s.vault, leaves: maps.Clone(s.leaves)}
	if err := next.applyRecords(c.Records, h.Author, key, m.Epoch()); err != nil {
		return nil, err
	}
	if err := next.applyPurges(c.Purged, c.Records); err != nil {
		return nil, err
	}
	root, err := MerkleRoot(next.Leaves())
	if err != nil {
		return nil, err
	}
	if root != h.MerkleRoot {
		return nil, ErrRootMismatch
	}
	if next.cursor, err = s.cursor.Advance(h); err != nil {
		return nil, err
	}
	return next, nil
}

// authorizeAuthor resolves the head's device and checks it may write here: certified, not revoked
// by a rotation at or before this membership position, belonging to a member who can write, and
// the signer of the head.
func authorizeAuthor(h *Head, m *MembershipState, resolve DeviceResolver) (ID, []byte, error) {
	account, key, err := resolve(h.Author)
	if err != nil {
		return ID{}, nil, fmt.Errorf("author device %s: %w", h.Author, err)
	}
	if m.IsDeviceRevoked(h.Author) {
		return ID{}, nil, ErrRevokedDevice
	}
	if !m.Role(account).CanWrite() {
		return ID{}, nil, fmt.Errorf("account %s: %w", account, ErrNotAuthorized)
	}
	if err := h.Verify(key); err != nil {
		return ID{}, nil, err
	}
	return account, key, nil
}

// applyRecords checks each record belongs to this commit and follows its record's current version,
// then installs it as the latest leaf.
func (s *VaultState) applyRecords(records []Envelope, author ID, key []byte, epoch uint32) error {
	seen := make(map[ID]bool, len(records))
	for i := range records {
		e := &records[i]
		if e.Vault != s.vault || e.Author != author {
			return fmt.Errorf("record %s: %w", e.Record, ErrCommitAuthor)
		}
		if err := e.Verify(key); err != nil {
			return err
		}
		if seen[e.Record] || e.Version != s.leaves[e.Record].Version+1 {
			return fmt.Errorf("record %s version %d: %w", e.Record, e.Version, ErrRecordVersion)
		}
		if e.Epoch != epoch {
			return fmt.Errorf("record %s epoch %d, vault epoch %d: %w", e.Record, e.Epoch, epoch, ErrEpochMismatch)
		}
		seen[e.Record] = true
		leaf, err := LeafOf(e)
		if err != nil {
			return err
		}
		s.leaves[e.Record] = leaf
	}
	return nil
}

// applyPurges removes tombstone leaves. A purge of a live record would be a deletion nobody can see
// in the trash, and a purge of an unknown record or a record the same commit writes has no meaning;
// all three are refused rather than ignored, because each is a server or device lying about state.
func (s *VaultState) applyPurges(purged []ID, records []Envelope) error {
	written := make(map[ID]bool, len(records))
	for i := range records {
		written[records[i].Record] = true
	}
	for _, id := range purged {
		// A duplicate finds its leaf already gone and fails the lookup like an unknown record.
		leaf, ok := s.leaves[id]
		if !ok || written[id] || !leaf.Deleted {
			return fmt.Errorf("record %s: %w", id, ErrPurge)
		}
		delete(s.leaves, id)
	}
	return nil
}

// RestoreVaultState rebuilds a state from a head and the latest version of every record, as a
// snapshot delivers them or as the server stored them. The records must reproduce the head's Merkle
// root exactly, so a snapshot that drops, adds or alters a record is refused. It checks no
// signature: a client verifies the head's author separately, through the device log, before
// trusting a snapshot; the server restores only states it already verified commit by commit.
//
// A nil head is a vault with no commits yet.
func RestoreVaultState(vault ID, head *Head, records []Envelope) (*VaultState, error) {
	if head != nil && head.Vault != vault {
		return nil, fmt.Errorf("head of vault %s restored as %s: %w", head.Vault, vault, ErrChainBroken)
	}
	leaves := make([]Leaf, 0, len(records))
	for i := range records {
		if records[i].Vault != vault {
			return nil, ErrRootMismatch
		}
		leaf, err := LeafOf(&records[i])
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, leaf)
	}
	return RestoreVaultStateFromLeaves(vault, head, leaves)
}

// RestoreVaultStateFromLeaves is RestoreVaultState for a holder that kept each current version's
// leaf rather than its envelope: the server, which would otherwise read and hash every ciphertext a
// vault holds to verify each commit. The leaves must reproduce the head's Merkle root exactly, which
// is what binds them to the envelopes they were derived from.
func RestoreVaultStateFromLeaves(vault ID, head *Head, leaves []Leaf) (*VaultState, error) {
	s, err := NewVaultState(vault)
	if err != nil {
		return nil, err
	}
	if head == nil {
		if len(leaves) != 0 {
			return nil, ErrRootMismatch
		}
		return s, nil
	}
	if head.Vault != vault {
		return nil, fmt.Errorf("head of vault %s restored as %s: %w", head.Vault, vault, ErrChainBroken)
	}
	for _, leaf := range leaves {
		if _, dup := s.leaves[leaf.Record]; dup {
			return nil, ErrRootMismatch
		}
		s.leaves[leaf.Record] = leaf
	}
	root, err := MerkleRoot(s.Leaves())
	if err != nil || root != head.MerkleRoot {
		return nil, ErrRootMismatch
	}
	hash, err := head.Hash()
	if err != nil {
		return nil, err
	}
	s.cursor = ChainCursor{Seq: head.Seq, Hash: hash, MembershipSeq: head.MembershipSeq}
	return s, nil
}

// Len counts the records the state holds, tombstones included: what a per-vault record limit
// is kept against.
func (s *VaultState) Len() int { return len(s.leaves) }
