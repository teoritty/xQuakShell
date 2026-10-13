package vaultsync_test

import (
	"bytes"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func TestPadSizes(t *testing.T) {
	cases := []struct{ plaintext, padded int }{
		{0, 256},   // the floor hides small records entirely
		{252, 256}, // 4-byte length prefix + 252 fills the floor exactly
		{300, 304}, // Padme rounds 304 to a multiple of 16
		{1000, 1024},
	}
	for _, c := range cases {
		got, err := vaultsync.Pad(make([]byte, c.plaintext))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.padded {
			t.Errorf("Pad(%d bytes) = %d bytes, want %d", c.plaintext, len(got), c.padded)
		}
	}
}

func TestPadOverheadStaysBelowTwelvePercent(t *testing.T) {
	for n := 1024; n <= 1<<22; n = n*5/4 + 7 {
		p := vaultsync.PaddedSize(n)
		if p < n+4 || float64(p-n)/float64(n) > 0.12 {
			t.Fatalf("PaddedSize(%d) = %d; Padme leaks O(log log n) bits for at most ~12%% overhead", n, p)
		}
	}
}

func TestUnpadRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 251, 252, 253, 4096, 70000} {
		in := bytes.Repeat([]byte{0x5a}, n)
		padded, err := vaultsync.Pad(in)
		if err != nil {
			t.Fatal(err)
		}
		out, err := vaultsync.Unpad(padded)
		if err != nil {
			t.Fatalf("Unpad(Pad(%d bytes)): %v", n, err)
		}
		if !bytes.Equal(out, in) {
			t.Fatalf("round trip of %d bytes changed the content", n)
		}
	}
}

func TestUnpadRejectsTampering(t *testing.T) {
	good, _ := vaultsync.Pad([]byte("secret"))
	cases := map[string][]byte{
		"short":              good[:255],
		"nonzero tail":       append(append([]byte{}, good[:len(good)-1]...), 1),
		"length past end":    append([]byte{0xff, 0xff, 0xff, 0xff}, good[4:]...),
		"non-canonical size": append(append([]byte{}, good...), make([]byte, 16)...),
	}
	for name, in := range cases {
		if _, err := vaultsync.Unpad(in); !errors.Is(err, vaultsync.ErrBadPadding) {
			t.Errorf("%s: err = %v, want ErrBadPadding", name, err)
		}
	}
}

func TestPadRefusesOversize(t *testing.T) {
	if _, err := vaultsync.Pad(make([]byte, vaultsync.MaxPlaintextSize+1)); !errors.Is(err, vaultsync.ErrPlaintextTooLarge) {
		t.Fatalf("err = %v, want ErrPlaintextTooLarge", err)
	}
}

func FuzzUnpad(f *testing.F) {
	seed, _ := vaultsync.Pad([]byte("seed"))
	f.Add(seed)
	f.Add(make([]byte, 256))
	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := vaultsync.Unpad(in)
		if err != nil {
			return
		}
		again, err := vaultsync.Pad(out)
		if err != nil || !bytes.Equal(again, in) {
			t.Fatalf("Unpad accepted %d bytes that are not the canonical padding of their content", len(in))
		}
	})
}
