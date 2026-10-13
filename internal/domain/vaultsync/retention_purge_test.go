package vaultsync_test

import (
	"crypto/ed25519"
	"errors"
	"math"
	"slices"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

// tombstone builds a signed deletion of record at version.
func (c *commitFixture) tombstone(author vaultsync.ID, record byte, version uint64) vaultsync.Envelope {
	p := c.m.ps[author]
	e := vaultsync.Envelope{Vault: vaultID, Record: vaultsync.ID{record}, Version: version, Epoch: c.m.state.Epoch(),
		Suite: vaultsync.Suite1, Deleted: true, Author: p.device, SigAlg: vaultsync.SigEd25519}
	msg, err := e.SigningBytes()
	if err != nil {
		c.t.Fatal(err)
	}
	e.Sig = ed25519.Sign(p.key, msg)
	return e
}

// purge signs a commit that removes the given leaves, its root computed over the state without them.
func (c *commitFixture) purge(author vaultsync.ID, purged ...vaultsync.ID) vaultsync.Commit {
	c.t.Helper()
	var list []vaultsync.Leaf
	for _, l := range c.state.Leaves() {
		if !slices.Contains(purged, l.Record) {
			list = append(list, l)
		}
	}
	root, err := vaultsync.MerkleRoot(list)
	if err != nil {
		c.t.Fatal(err)
	}
	commit := c.sign(author, root, nil)
	commit.Purged = purged
	return commit
}

// deleted writes record and then deletes it, leaving a tombstone leaf.
func (c *commitFixture) deleted(record byte) {
	c.t.Helper()
	if err := c.apply(c.commit(editor, c.record(editor, record, 1))); err != nil {
		c.t.Fatal(err)
	}
	if err := c.apply(c.commit(editor, c.tombstone(editor, record, 2))); err != nil {
		c.t.Fatal(err)
	}
}

func TestPurgeRemovesATombstone(t *testing.T) {
	c := newCommitFixture(t)
	c.deleted(1)
	if err := c.apply(c.commit(editor, c.record(editor, 2, 1))); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(c.purge(editor, vaultsync.ID{1})); err != nil {
		t.Fatalf("purge of a tombstone was refused: %v", err)
	}
	if _, ok := c.state.Version(vaultsync.ID{1}); ok {
		t.Fatal("the purged record is still a leaf")
	}
	if _, ok := c.state.Version(vaultsync.ID{2}); !ok {
		t.Fatal("a purge removed a record it did not name")
	}
}

func TestPurgeRefusals(t *testing.T) {
	cases := []struct {
		name  string
		build func(c *commitFixture) vaultsync.Commit
	}{
		{"live record", func(c *commitFixture) vaultsync.Commit {
			if err := c.apply(c.commit(editor, c.record(editor, 2, 1))); err != nil {
				c.t.Fatal(err)
			}
			return c.purge(editor, vaultsync.ID{2})
		}},
		{"unknown record", func(c *commitFixture) vaultsync.Commit { return c.purge(editor, vaultsync.ID{9}) }},
		{"duplicate", func(c *commitFixture) vaultsync.Commit {
			p := c.purge(editor, vaultsync.ID{1})
			p.Purged = append(p.Purged, vaultsync.ID{1})
			return p
		}},
		{"also written", func(c *commitFixture) vaultsync.Commit {
			// A tombstone written and purged in one commit: the leaf the purge finds is a tombstone,
			// so only the written check refuses it.
			p := c.commit(editor, c.tombstone(editor, 1, 3))
			p.Purged = []vaultsync.ID{{1}}
			return p
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCommitFixture(t)
			c.deleted(1)
			before := len(c.state.Leaves())
			commit := tc.build(c)
			if err := c.apply(commit); !errors.Is(err, vaultsync.ErrPurge) {
				t.Fatalf("err = %v, want ErrPurge: a purge may remove only a tombstone, once", err)
			}
			if len(c.state.Leaves()) < before {
				t.Fatal("a refused purge changed the state")
			}
		})
	}
}

func TestPurgeIsAnIntegrityError(t *testing.T) {
	if !errors.Is(vaultsync.ErrPurge, vaultsync.ErrIntegrity) {
		t.Fatal("a refused purge must stop sync like any other verification failure")
	}
}

func TestPrunedWrapStillVerifies(t *testing.T) {
	f := newFixture(t)
	create := f.op(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberCreate, Wraps: wrapsFor(owner), RetentionDays: 30})
	if err := f.state.Apply(create, checker(f.ps)); err != nil {
		t.Fatal(err)
	}
	rotate := f.op(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner), TimeUnixMilli: 5})
	if err := f.state.Apply(rotate, checker(f.ps)); err != nil {
		t.Fatal(err)
	}
	replay, err := vaultsync.NewMembershipState(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []*vaultsync.MemberOp{create, rotate} {
		stripped := *o
		stripped.Wraps = nil
		for _, w := range o.Wraps {
			stripped.Wraps = append(stripped.Wraps, w.Strip())
		}
		if !stripped.Wraps[0].Pruned() || o.Wraps[0].Pruned() {
			t.Fatal("only a stripped wrap reports itself pruned")
		}
		if err := replay.Apply(&stripped, checker(f.ps)); err != nil {
			t.Fatalf("a log with pruned wraps must still verify: %v", err)
		}
	}
	if replay.Hash() != f.state.Hash() {
		t.Fatal("stripping wraps changed the membership head")
	}
}

func TestWrapWithWrongDigestIsRefused(t *testing.T) {
	f := newFixture(t)
	w := wrap(owner)
	w.Ciphertext[0] ^= 1
	err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberCreate, Wraps: []vaultsync.KeyWrap{w}, RetentionDays: 30})
	if !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership: a ciphertext must match the digest the op signs", err)
	}
}

func TestNewKeyWrapRefusesAnEmptyCiphertext(t *testing.T) {
	if _, err := vaultsync.NewKeyWrap(owner, aekBytes(1), nil); err == nil {
		t.Fatal("an empty wrap would be indistinguishable from a pruned one")
	}
}

func TestRotationTimeCannotGoBack(t *testing.T) {
	f := newFixture(t)
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberCreate, Wraps: wrapsFor(owner), RetentionDays: 30, TimeUnixMilli: 100})
	err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner), TimeUnixMilli: 99})
	if !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership: a rotation dated before the last one would retire epochs early", err)
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner), TimeUnixMilli: 100})
}

func TestShredOnlyOnRotation(t *testing.T) {
	f := newFixture(t)
	f.create()
	err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 10, Shred: true})
	if !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership: only a rotation may shred", err)
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1, Wraps: wrapsFor(owner), Shred: true})
	rs := f.state.Rotations()
	if !rs[len(rs)-1].Shred {
		t.Fatal("the rotation did not record shred")
	}
}

func TestRotationAtMaxEpochIsRefused(t *testing.T) {
	if err := vaultsync.CheckNextEpoch(math.MaxUint32); !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership: the epoch must not wrap to 0", err)
	}
	if err := vaultsync.CheckNextEpoch(math.MaxUint32 - 1); err != nil {
		t.Fatalf("the last epoch below the limit was refused: %v", err)
	}
}

func TestShredRetiresEveryOlderEpochAtOnce(t *testing.T) {
	rs := []vaultsync.Rotation{{Epoch: 0, StartedUnixMilli: 0}, {Epoch: 1, StartedUnixMilli: day}, {Epoch: 2, StartedUnixMilli: 2 * day, Shred: true}}
	for _, e := range []uint32{0, 1} {
		if !vaultsync.EpochRetired(e, rs, notInUse, 30, 2*day) {
			t.Fatalf("epoch %d not retired after a shred rotation", e)
		}
	}
	if vaultsync.EpochRetired(1, rs, func(e uint32) bool { return e == 1 }, 30, 2*day) {
		t.Fatal("a shred must not retire an epoch a current version still uses")
	}
	if vaultsync.EpochRetired(2, rs, notInUse, 30, 2*day) {
		t.Fatal("the current epoch retired")
	}
	early := []vaultsync.Rotation{{Epoch: 0}, {Epoch: 1, StartedUnixMilli: day, Shred: true}, {Epoch: 2, StartedUnixMilli: 2 * day}}
	if vaultsync.EpochRetired(1, early, notInUse, 30, 2*day) {
		t.Fatal("a shred retires only the epochs before it, not the one it started")
	}
}

func TestRetiredEpochsListsOnlyUnused(t *testing.T) {
	rs := []vaultsync.Rotation{{Epoch: 0, StartedUnixMilli: 0}, {Epoch: 1, StartedUnixMilli: day}, {Epoch: 2, StartedUnixMilli: 40 * day}}
	inUse := func(e uint32) bool { return e == 2 }
	if got := vaultsync.RetiredEpochs(rs, inUse, 30, 45*day); !slices.Equal(got, []uint32{0}) {
		t.Fatalf("retired = %v, want [0]: epoch 1's successor began 5 days ago", got)
	}
	if got := vaultsync.RetiredEpochs(rs, inUse, 30, 71*day); !slices.Equal(got, []uint32{0, 1}) {
		t.Fatalf("retired = %v, want [0 1]", got)
	}
}
