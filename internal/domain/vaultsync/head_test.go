package vaultsync_test

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func nextHead(t *testing.T, key ed25519.PrivateKey, cur vaultsync.ChainCursor, membershipSeq uint64) *vaultsync.Head {
	t.Helper()
	h := &vaultsync.Head{
		Vault: vaultsync.ID{1}, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: [32]byte{byte(cur.Seq)},
		CurrentEpoch: 0, MembershipSeq: membershipSeq, MembershipHead: [32]byte{9},
		Commit: vaultsync.ID{byte(cur.Seq + 1)}, Author: vaultsync.ID{4}, ClientTimeUnixMilli: 1,
		SigAlg: vaultsync.SigEd25519,
	}
	msg, err := h.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	h.Sig = ed25519.Sign(key, msg)
	return h
}

func TestHeadChainAdvances(t *testing.T) {
	key := testKey(1)
	cur, err := vaultsync.GenesisCursor(vaultsync.ID{1})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		h := nextHead(t, key, cur, 1)
		if err := h.Verify(key.Public().(ed25519.PublicKey)); err != nil {
			t.Fatalf("head %d: %v", i+1, err)
		}
		next, err := cur.Advance(h)
		if err != nil {
			t.Fatalf("advance to %d: %v", i+1, err)
		}
		if next.Seq != cur.Seq+1 {
			t.Fatalf("seq = %d, want %d", next.Seq, cur.Seq+1)
		}
		cur = next
	}
}

func TestHeadChainRejectsBreaks(t *testing.T) {
	key := testKey(1)
	genesis, _ := vaultsync.GenesisCursor(vaultsync.ID{1})
	first := nextHead(t, key, genesis, 2)
	cur, _ := genesis.Advance(first)

	skip := nextHead(t, key, vaultsync.ChainCursor{Seq: cur.Seq + 1, Hash: cur.Hash, MembershipSeq: 2}, 2)
	if _, err := cur.Advance(skip); !errors.Is(err, vaultsync.ErrChainBroken) {
		t.Errorf("skipped seq: err = %v, want ErrChainBroken", err)
	}
	wrongPrev := nextHead(t, key, vaultsync.ChainCursor{Seq: cur.Seq, Hash: [32]byte{0xee}, MembershipSeq: 2}, 2)
	if _, err := cur.Advance(wrongPrev); !errors.Is(err, vaultsync.ErrChainBroken) {
		t.Errorf("wrong prev hash: err = %v, want ErrChainBroken", err)
	}
	regressed := nextHead(t, key, cur, 1)
	if _, err := cur.Advance(regressed); !errors.Is(err, vaultsync.ErrMembershipRegressed) {
		t.Errorf("older membership: err = %v, want ErrMembershipRegressed; a commit must not claim an earlier membership state", err)
	}
	same := nextHead(t, key, cur, 2)
	if _, err := cur.Advance(same); err != nil {
		t.Errorf("equal membership position: %v; consecutive commits under one membership state are normal", err)
	}
}

func TestHeadGenesisIsPerVault(t *testing.T) {
	a, _ := vaultsync.GenesisCursor(vaultsync.ID{1})
	b, _ := vaultsync.GenesisCursor(vaultsync.ID{2})
	if a.Hash == b.Hash {
		t.Fatal("two vaults share a genesis hash; one vault's first commit would link into another")
	}
}

func TestCheckObservedDetectsRollbackAndFork(t *testing.T) {
	cur := vaultsync.ChainCursor{Seq: 10, Hash: [32]byte{1}}
	if err := vaultsync.CheckObserved(cur, 9, [32]byte{5}); !errors.Is(err, vaultsync.ErrRollback) {
		t.Errorf("older seq: err = %v, want ErrRollback", err)
	}
	if err := vaultsync.CheckObserved(cur, 10, [32]byte{2}); !errors.Is(err, vaultsync.ErrFork) {
		t.Errorf("same seq other hash: err = %v, want ErrFork", err)
	}
	if err := vaultsync.CheckObserved(cur, 10, [32]byte{1}); err != nil {
		t.Errorf("same head: %v", err)
	}
	if err := vaultsync.CheckObserved(cur, 11, [32]byte{3}); err != nil {
		t.Errorf("newer head: %v", err)
	}
}

func TestHeadAnyTamperFails(t *testing.T) {
	key := testKey(1)
	pub := key.Public().(ed25519.PublicKey)
	genesis, _ := vaultsync.GenesisCursor(vaultsync.ID{1})
	mutators := []func(*vaultsync.Head){
		func(h *vaultsync.Head) { h.Vault[0] ^= 1 },
		func(h *vaultsync.Head) { h.Seq++ },
		func(h *vaultsync.Head) { h.PrevHash[0] ^= 1 },
		func(h *vaultsync.Head) { h.MerkleRoot[31] ^= 1 },
		func(h *vaultsync.Head) { h.CurrentEpoch++ },
		func(h *vaultsync.Head) { h.MembershipSeq++ },
		func(h *vaultsync.Head) { h.MembershipHead[0] ^= 1 },
		func(h *vaultsync.Head) { h.Commit[0] ^= 1 },
		func(h *vaultsync.Head) { h.Author[0] ^= 1 },
		func(h *vaultsync.Head) { h.ClientTimeUnixMilli++ },
		func(h *vaultsync.Head) { h.SigAlg++ },
		func(h *vaultsync.Head) { h.Sig[10] ^= 1 },
	}
	for i, mutate := range mutators {
		h := nextHead(t, key, genesis, 1)
		mutate(h)
		if err := h.Verify(pub); !errors.Is(err, vaultsync.ErrIntegrity) {
			t.Errorf("head mutation %d: err = %v, want an integrity error", i, err)
		}
	}
}

func TestHeadHashCoversSignature(t *testing.T) {
	genesis, _ := vaultsync.GenesisCursor(vaultsync.ID{1})
	h := nextHead(t, testKey(1), genesis, 1)
	a, _ := h.Hash()
	h.Sig[0] ^= 1
	b, _ := h.Hash()
	if a == b {
		t.Fatal("head hash ignores the signature; the next head would not commit to who signed this one")
	}
}

func TestAdvanceAllWalksALinkedChainAndNothingElse(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	start, _ := vaultsync.GenesisCursor(vaultsync.ID{1})
	cur, chain := start, []vaultsync.Head{}
	for range 3 {
		h := nextHead(t, key, cur, 0)
		chain = append(chain, *h)
		var err error
		if cur, err = cur.Advance(h); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := start.AdvanceAll(chain); err != nil || got != cur {
		t.Fatalf("walk = %+v, %v; want the cursor after the last head %+v", got, err, cur)
	}
	if got, err := start.AdvanceAll(nil); err != nil || got != start {
		t.Fatalf("an empty walk = %+v, %v; want the cursor unchanged", got, err)
	}
	gap := []vaultsync.Head{chain[0], chain[2]}
	if _, err := start.AdvanceAll(gap); !errors.Is(err, vaultsync.ErrChainBroken) {
		t.Fatalf("a chain with a head missing: %v, want ErrChainBroken", err)
	}
	forged := append([]vaultsync.Head(nil), chain...)
	forged[1].PrevHash[0] ^= 1
	if _, err := start.AdvanceAll(forged); !errors.Is(err, vaultsync.ErrChainBroken) {
		t.Fatalf("a head naming another predecessor: %v, want ErrChainBroken", err)
	}
}
