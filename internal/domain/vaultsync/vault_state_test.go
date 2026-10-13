package vaultsync_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

// commitFixture is a team vault (owner, admin, editor, viewer) with a verified state to commit on.
type commitFixture struct {
	t     *testing.T
	m     *fixture
	state *vaultsync.VaultState
}

func newCommitFixture(t *testing.T) *commitFixture {
	t.Helper()
	m := newFixture(t)
	m.team()
	s, err := vaultsync.NewVaultState(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	return &commitFixture{t: t, m: m, state: s}
}

func (c *commitFixture) resolver() vaultsync.DeviceResolver {
	return func(device vaultsync.ID) (vaultsync.ID, []byte, error) {
		for _, p := range c.m.ps {
			if p.device == device {
				return p.account, p.key.Public().(ed25519.PublicKey), nil
			}
		}
		return vaultsync.ID{}, nil, vaultsync.ErrDeviceLog
	}
}

// record builds a signed live envelope at the next version for its record.
func (c *commitFixture) record(author vaultsync.ID, record byte, version uint64) vaultsync.Envelope {
	p := c.m.ps[author]
	e := vaultsync.Envelope{Vault: vaultID, Record: vaultsync.ID{record}, Version: version, Epoch: c.m.state.Epoch(),
		Suite: vaultsync.Suite1, WrappedDEK: bytes.Repeat([]byte{record}, 72), Body: bytes.Repeat([]byte{record}, 296),
		Author: p.device, SigAlg: vaultsync.SigEd25519}
	msg, err := e.SigningBytes()
	if err != nil {
		c.t.Fatal(err)
	}
	e.Sig = ed25519.Sign(p.key, msg)
	return e
}

// commit wraps records in a head whose root is computed over the state after applying them.
func (c *commitFixture) commit(author vaultsync.ID, records ...vaultsync.Envelope) vaultsync.Commit {
	c.t.Helper()
	leaves := map[vaultsync.ID]vaultsync.Leaf{}
	for _, l := range c.state.Leaves() {
		leaves[l.Record] = l
	}
	for i := range records {
		l, err := vaultsync.LeafOf(&records[i])
		if err != nil {
			c.t.Fatal(err)
		}
		leaves[l.Record] = l
	}
	var list []vaultsync.Leaf
	for _, l := range leaves {
		list = append(list, l)
	}
	root, err := vaultsync.MerkleRoot(list)
	if err != nil {
		c.t.Fatal(err)
	}
	return c.sign(author, root, records)
}

func (c *commitFixture) sign(author vaultsync.ID, root [32]byte, records []vaultsync.Envelope) vaultsync.Commit {
	p := c.m.ps[author]
	cur := c.state.Cursor()
	h := vaultsync.Head{Vault: vaultID, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: root,
		CurrentEpoch: c.m.state.Epoch(), MembershipSeq: c.m.state.Seq(), MembershipHead: c.m.state.Hash(),
		Commit: vaultsync.ID{byte(cur.Seq + 1)}, Author: p.device, ClientTimeUnixMilli: 1, SigAlg: vaultsync.SigEd25519}
	msg, err := h.SigningBytes()
	if err != nil {
		c.t.Fatal(err)
	}
	h.Sig = ed25519.Sign(p.key, msg)
	return vaultsync.Commit{Head: h, Records: records}
}

func (c *commitFixture) apply(commit vaultsync.Commit) error {
	next, err := c.state.ApplyCommit(commit, c.m.state, c.resolver())
	if err == nil {
		c.state = next
	}
	return err
}

func TestApplyCommitHappyPath(t *testing.T) {
	c := newCommitFixture(t)
	if err := c.apply(c.commit(editor, c.record(editor, 1, 1), c.record(editor, 2, 1))); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if err := c.apply(c.commit(admin, c.record(admin, 1, 2))); err != nil {
		t.Fatalf("second commit: %v", err)
	}
	if err := c.apply(c.commit(owner, c.record(owner, 3, 1))); err != nil {
		t.Fatalf("third commit: %v", err)
	}
	if v, ok := c.state.Version(vaultsync.ID{1}); !ok || v != 2 {
		t.Fatalf("record 1 version = %d, %v; want 2", v, ok)
	}
	if c.state.Cursor().Seq != 3 || len(c.state.Leaves()) != 3 {
		t.Fatalf("seq %d leaves %d; want 3 and 3", c.state.Cursor().Seq, len(c.state.Leaves()))
	}
}

func TestApplyCommitRules(t *testing.T) {
	cases := map[string]struct {
		build func(c *commitFixture) vaultsync.Commit
		want  error
	}{
		"record dropped after signing": {func(c *commitFixture) vaultsync.Commit {
			cm := c.commit(editor, c.record(editor, 1, 1), c.record(editor, 2, 1))
			cm.Records = cm.Records[:1]
			return cm
		}, vaultsync.ErrRootMismatch},
		"record by another device": {func(c *commitFixture) vaultsync.Commit {
			return c.commit(editor, c.record(admin, 1, 1))
		}, vaultsync.ErrCommitAuthor},
		"version skipped": {func(c *commitFixture) vaultsync.Commit {
			return c.commit(editor, c.record(editor, 1, 2))
		}, vaultsync.ErrRecordVersion},
		"duplicate record": {func(c *commitFixture) vaultsync.Commit {
			r := c.record(editor, 1, 1)
			return c.sign(editor, [32]byte{}, []vaultsync.Envelope{r, r})
		}, vaultsync.ErrRecordVersion},
		"viewer author": {func(c *commitFixture) vaultsync.Commit {
			return c.commit(viewer, c.record(viewer, 1, 1))
		}, vaultsync.ErrNotAuthorized},
		"outsider device": {func(c *commitFixture) vaultsync.Commit {
			return c.commit(outsider, c.record(outsider, 1, 1))
		}, vaultsync.ErrNotAuthorized},
		"record from another vault": {func(c *commitFixture) vaultsync.Commit {
			r := c.record(editor, 1, 1)
			r.Vault = vaultsync.ID{0x99}
			msg, _ := r.SigningBytes()
			r.Sig = ed25519.Sign(c.m.ps[editor].key, msg)
			return c.commit(editor, r)
		}, vaultsync.ErrCommitAuthor},
		"head tampered": {func(c *commitFixture) vaultsync.Commit {
			cm := c.commit(editor, c.record(editor, 1, 1))
			cm.Head.ClientTimeUnixMilli++
			return cm
		}, vaultsync.ErrBadSignature},
	}
	for name, tc := range cases {
		c := newCommitFixture(t)
		before := c.state.Cursor()
		if err := c.apply(tc.build(c)); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if c.state.Cursor() != before {
			t.Errorf("%s: a rejected commit moved the state", name)
		}
	}
}

func TestApplyCommitEpochMustMatchMembership(t *testing.T) {
	c := newCommitFixture(t)
	r := c.record(editor, 1, 1)
	r.Epoch = 5
	msg, _ := r.SigningBytes()
	r.Sig = ed25519.Sign(c.m.ps[editor].key, msg)
	if err := c.apply(c.commit(editor, r)); !errors.Is(err, vaultsync.ErrEpochMismatch) {
		t.Fatalf("err = %v, want ErrEpochMismatch; new versions must use the current vault key", err)
	}
	cm := c.commit(editor, c.record(editor, 1, 1))
	cm.Head.CurrentEpoch = 9
	msg, _ = cm.Head.SigningBytes()
	cm.Head.Sig = ed25519.Sign(c.m.ps[editor].key, msg)
	if err := c.apply(cm); !errors.Is(err, vaultsync.ErrEpochMismatch) {
		t.Fatalf("head epoch: err = %v, want ErrEpochMismatch", err)
	}
}

func TestApplyCommitMembershipMustMatch(t *testing.T) {
	c := newCommitFixture(t)
	stale := c.commit(editor, c.record(editor, 1, 1))
	c.m.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: 10})
	if err := c.apply(stale); !errors.Is(err, vaultsync.ErrMembershipMismatch) {
		t.Fatalf("err = %v, want ErrMembershipMismatch for a head made under an older membership state", err)
	}
}

func TestApplyCommitRefusesRevokedDevice(t *testing.T) {
	c := newCommitFixture(t)
	c.m.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1,
		Wraps: wrapsFor(owner, admin, editor, viewer), RevokedDevices: []vaultsync.ID{c.m.ps[editor].device}})
	if err := c.apply(c.commit(editor, c.record(editor, 1, 1))); !errors.Is(err, vaultsync.ErrRevokedDevice) {
		t.Fatalf("err = %v, want ErrRevokedDevice", err)
	}
}

func TestApplyCommitChainAndTombstones(t *testing.T) {
	c := newCommitFixture(t)
	if err := c.apply(c.commit(editor, c.record(editor, 1, 1))); err != nil {
		t.Fatal(err)
	}
	tomb := vaultsync.Envelope{Vault: vaultID, Record: vaultsync.ID{1}, Version: 2, Epoch: c.m.state.Epoch(), Suite: vaultsync.Suite1,
		Deleted: true, Author: c.m.ps[editor].device, SigAlg: vaultsync.SigEd25519}
	msg, _ := tomb.SigningBytes()
	tomb.Sig = ed25519.Sign(c.m.ps[editor].key, msg)
	if err := c.apply(c.commit(editor, tomb)); err != nil {
		t.Fatalf("a tombstone commit was refused: %v", err)
	}
	if len(c.state.Leaves()) != 1 {
		t.Fatal("a tombstone must stay a leaf until purged, or the server could hide the deletion")
	}
	replay := c.commit(editor, c.record(editor, 2, 1))
	replay.Head.PrevHash[0] ^= 1
	msg, _ = replay.Head.SigningBytes()
	replay.Head.Sig = ed25519.Sign(c.m.ps[editor].key, msg)
	if err := c.apply(replay); !errors.Is(err, vaultsync.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

func TestApplyCommitWrongVault(t *testing.T) {
	c := newCommitFixture(t)
	other, _ := vaultsync.NewVaultState(vaultsync.ID{0x55})
	if _, err := other.ApplyCommit(c.commit(editor, c.record(editor, 1, 1)), c.m.state, c.resolver()); !errors.Is(err, vaultsync.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken for a head of another vault", err)
	}
}
