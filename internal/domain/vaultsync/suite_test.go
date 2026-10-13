package vaultsync_test

import (
	"bytes"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func TestSuite1IsFrozen(t *testing.T) {
	p, err := vaultsync.LookupSuite(vaultsync.Suite1, vaultsync.MinAcceptedSuite)
	if err != nil {
		t.Fatal(err)
	}
	wantArgon := vaultsync.Argon2Params{MemoryKiB: 262144, Iterations: 3, Threads: 4, KeyLen: 32}
	if p.Argon2 != wantArgon {
		t.Fatalf("suite 1 Argon2 = %+v, want %+v; a suite never changes - stronger parameters are a new suite id", p.Argon2, wantArgon)
	}
	sizes := []struct {
		name      string
		got, want int
	}{
		{"WrappedDEKSize", p.WrappedDEKSize, 72},
		{"AEADOverhead", p.AEADOverhead, 40},
		{"SigningKeySize", p.SigningKeySize, 32},
		{"EncryptionKeySize", p.EncryptionKeySize, 1216},
		{"VKWrapSize", p.VKWrapSize, 1168},
	}
	for _, s := range sizes {
		if s.got != s.want {
			t.Errorf("suite 1 %s = %d, want %d; sizes are part of the frozen suite", s.name, s.got, s.want)
		}
	}
}

func TestLookupSuiteRefusesBelowMinimum(t *testing.T) {
	_, err := vaultsync.LookupSuite(vaultsync.Suite1, vaultsync.Suite1+1)
	if !errors.Is(err, vaultsync.ErrSuiteBelowMinimum) {
		t.Fatalf("err = %v, want ErrSuiteBelowMinimum; a server must not talk a client down to an old suite", err)
	}
}

func TestLookupSuiteRefusesUnknown(t *testing.T) {
	for _, id := range []vaultsync.SuiteID{0, 2, 0xffff} {
		if _, err := vaultsync.LookupSuite(id, 0); !errors.Is(err, vaultsync.ErrUnsupportedSuite) {
			t.Errorf("LookupSuite(%d) err = %v, want ErrUnsupportedSuite", id, err)
		}
	}
}

func TestAEADInputsBindEveryCoordinate(t *testing.T) {
	v, r, m := vaultsync.ID{1}, vaultsync.ID{2}, vaultsync.ID{3}
	base, _ := vaultsync.BodyAAD(v, r, 5, 2, vaultsync.Suite1)
	variants := map[string]func() ([]byte, error){
		"vault":   func() ([]byte, error) { return vaultsync.BodyAAD(vaultsync.ID{9}, r, 5, 2, vaultsync.Suite1) },
		"record":  func() ([]byte, error) { return vaultsync.BodyAAD(v, vaultsync.ID{9}, 5, 2, vaultsync.Suite1) },
		"version": func() ([]byte, error) { return vaultsync.BodyAAD(v, r, 6, 2, vaultsync.Suite1) },
		"epoch":   func() ([]byte, error) { return vaultsync.BodyAAD(v, r, 5, 3, vaultsync.Suite1) },
		"suite":   func() ([]byte, error) { return vaultsync.BodyAAD(v, r, 5, 2, vaultsync.Suite1+1) },
		"label":   func() ([]byte, error) { return vaultsync.DEKWrapAAD(v, r, 5, 2) },
	}
	for name, build := range variants {
		got, err := build()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(got, base) {
			t.Errorf("changing %s left the AAD unchanged; a ciphertext moved there would still open", name)
		}
	}
	w1, _ := vaultsync.VKWrapInfo(v, 2, m)
	w2, _ := vaultsync.VKWrapInfo(v, 2, vaultsync.ID{4})
	if bytes.Equal(w1, w2) {
		t.Error("VK wrap info ignores the member; a wrap could be replayed to another member")
	}
	b1, _ := vaultsync.BundleAAD(m, vaultsync.Suite1)
	b2, _ := vaultsync.BundleAAD(vaultsync.ID{4}, vaultsync.Suite1)
	if bytes.Equal(b1, b2) {
		t.Error("bundle AAD ignores the account; one account's bundle would open under another's id")
	}
}
