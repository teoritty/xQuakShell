package vaultsync_test

import (
	"bytes"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func TestCanonLayout(t *testing.T) {
	got, err := vaultsync.NewCanon("L").U16(1).Bool(true).Encoded()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 0, 1, 'L', 0, 0, 0, 2, 0, 1, 0, 0, 0, 1, 1}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoding = %x, want %x; this layout is a wire contract shared with the server", got, want)
	}
}

func TestCanonIsInjectiveAcrossFieldBoundaries(t *testing.T) {
	a, _ := vaultsync.NewCanon("L").String("ab").String("c").Encoded()
	b, _ := vaultsync.NewCanon("L").String("a").String("bc").Encoded()
	if bytes.Equal(a, b) {
		t.Fatal(`("ab","c") and ("a","bc") encode alike; a signature over one would verify for the other`)
	}
}

func TestCanonLabelSeparatesPurposes(t *testing.T) {
	a, _ := vaultsync.NewCanon("xqs-sync/v1/record-sig").U64(7).Sum()
	b, _ := vaultsync.NewCanon("xqs-sync/v1/head-sig").U64(7).Sum()
	if a == b {
		t.Fatal("two labels produced one digest; bytes signed for one purpose must not verify for another")
	}
}

func TestCanonIDListCommitsToLength(t *testing.T) {
	x := vaultsync.ID{1}
	y := vaultsync.ID{2}
	a, _ := vaultsync.NewCanon("L").IDs([]vaultsync.ID{x}).IDs([]vaultsync.ID{y}).Encoded()
	b, _ := vaultsync.NewCanon("L").IDs([]vaultsync.ID{x, y}).IDs(nil).Encoded()
	if bytes.Equal(a, b) {
		t.Fatal("([x],[y]) and ([x,y],[]) encode alike; a list boundary moved without changing the signature")
	}
}

func TestCanonIntegersAreFixedWidth(t *testing.T) {
	a, _ := vaultsync.NewCanon("L").U64(1).Encoded()
	b, _ := vaultsync.NewCanon("L").U64(1 << 40).Encoded()
	if len(a) != len(b) {
		t.Fatalf("U64 widths %d and %d differ; integer width must not depend on value", len(a), len(b))
	}
}
