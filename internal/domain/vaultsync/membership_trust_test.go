package vaultsync_test

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

// digestOf is a party's identity digest; an account the fixture has no key for gets a stand-in, as
// an op adding an unknown account still has to record something.
func digestOf(p party) [32]byte {
	if p.key == nil {
		return [32]byte{0xfe}
	}
	d, err := vaultsync.IdentityDigest(p.key.Public().(ed25519.PublicKey))
	if err != nil {
		panic(err)
	}
	return d
}

func TestAddMemberMustRecordAFingerprint(t *testing.T) {
	f := newFixture(t)
	f.create()
	err := f.apply(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: admin, Role: vaultsync.RoleAdmin, Verified: true,
		Epoch: 0, Wraps: wrapsFor(admin)})
	if !errors.Is(err, vaultsync.ErrMembership) {
		t.Fatalf("err = %v, want ErrMembership: every member's key is anchored by the fingerprint its add records", err)
	}
}

func TestMembershipRemembersFingerprintsAndTheCreator(t *testing.T) {
	f := newFixture(t)
	f.team()
	if f.state.Creator() != owner {
		t.Fatalf("creator = %x, want the owner who created the vault", f.state.Creator())
	}
	got, ok := f.state.MemberFingerprint(editor)
	if !ok || got != digestOf(f.ps[editor]) {
		t.Fatal("the fingerprint an add-member recorded is not remembered")
	}
	if _, ok := f.state.MemberFingerprint(outsider); ok {
		t.Fatal("a non-member has a fingerprint")
	}
	f.must(owner, &vaultsync.MemberOp{Kind: vaultsync.MemberTransferOwnership, Subject: admin})
	if f.state.Creator() != owner {
		t.Fatal("transferring ownership changed who created the vault")
	}
}

func TestIdentityDigestSeparatesKeys(t *testing.T) {
	a, _ := vaultsync.IdentityDigest(testKey(1).Public().(ed25519.PublicKey))
	b, _ := vaultsync.IdentityDigest(testKey(2).Public().(ed25519.PublicKey))
	if a == b || a == ([32]byte{}) {
		t.Fatal("identity digests must differ per key and never be zero")
	}
}
