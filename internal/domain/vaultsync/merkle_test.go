package vaultsync_test

import (
	"encoding/hex"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func leaves(n int) []vaultsync.Leaf {
	out := make([]vaultsync.Leaf, n)
	for i := range out {
		out[i] = vaultsync.Leaf{Record: vaultsync.ID{byte(i + 1)}, Version: uint64(i + 1), Digest: [32]byte{byte(100 + i)}}
	}
	return out
}

func TestMerkleRootIgnoresInputOrder(t *testing.T) {
	ls := leaves(5)
	a, _ := vaultsync.MerkleRoot(ls)
	b, _ := vaultsync.MerkleRoot([]vaultsync.Leaf{ls[3], ls[0], ls[4], ls[2], ls[1]})
	if a != b {
		t.Fatal("root depends on input order; two honest devices would disagree about one state")
	}
}

func TestMerkleRootSensitiveToEveryLeafField(t *testing.T) {
	base, _ := vaultsync.MerkleRoot(leaves(3))
	for name, mutate := range map[string]func([]vaultsync.Leaf){
		"version": func(l []vaultsync.Leaf) { l[1].Version++ },
		"digest":  func(l []vaultsync.Leaf) { l[2].Digest[31] ^= 1 },
		"record":  func(l []vaultsync.Leaf) { l[0].Record[15] ^= 1 },
		"removal": nil,
	} {
		ls := leaves(3)
		if mutate != nil {
			mutate(ls)
		} else {
			ls = ls[:2]
		}
		got, _ := vaultsync.MerkleRoot(ls)
		if got == base {
			t.Errorf("changing %s left the root unchanged; the server could alter that undetected", name)
		}
	}
}

func TestMerkleRootRejectsDuplicates(t *testing.T) {
	ls := leaves(2)
	ls[1].Record = ls[0].Record
	if _, err := vaultsync.MerkleRoot(ls); !errors.Is(err, vaultsync.ErrDuplicateLeaf) {
		t.Fatalf("err = %v, want ErrDuplicateLeaf", err)
	}
}

func TestMerkleEmptyRootDiffersFromAnyState(t *testing.T) {
	empty, _ := vaultsync.MerkleRoot(nil)
	one, _ := vaultsync.MerkleRoot(leaves(1))
	if empty == one {
		t.Fatal("empty root equals a one-record root; a server could claim an emptied vault is unchanged")
	}
}

// The root of a fixed state is a wire contract: client and server must compute it identically,
// and a change here is a protocol major. goldenRoot was pinned once, the first time every other
// Merkle test passed; it must never be regenerated to make a failing run pass.
const goldenRoot = "9fb4cc7eda22d220a48735c3123129de86b448a0a21800000da0481aa4485fd1"

func TestMerkleRootGolden(t *testing.T) {
	got, err := vaultsync.MerkleRoot(leaves(5))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got[:]) != goldenRoot {
		t.Fatalf("root = %x, want %s; the Merkle construction changed", got, goldenRoot)
	}
}

func TestLeafOfCommitsToEnvelope(t *testing.T) {
	e := signedEnvelope(t, testKey(1))
	leaf, err := vaultsync.LeafOf(e)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := e.Digest()
	if leaf.Record != e.Record || leaf.Version != e.Version || leaf.Digest != digest {
		t.Fatalf("leaf = %+v; it must name the envelope's record, version and digest", leaf)
	}
	e.Sig[0] ^= 1
	other, _ := vaultsync.LeafOf(e)
	if other.Digest == leaf.Digest {
		t.Fatal("re-signing an envelope left its leaf unchanged")
	}
}
