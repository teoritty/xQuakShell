package vaultsync_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func TestParseIDRoundTrip(t *testing.T) {
	id, err := vaultsync.NewID(bytes.NewReader(bytes.Repeat([]byte{0xab}, vaultsync.IDSize)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := vaultsync.ParseID(id.String())
	if err != nil {
		t.Fatalf("ParseID(%q): %v", id.String(), err)
	}
	if got != id {
		t.Fatalf("round trip = %v, want %v; an id must survive its own text form", got, id)
	}
}

func TestParseIDRejectsMalformed(t *testing.T) {
	for _, in := range []string{
		"",
		strings.Repeat("zz", vaultsync.IDSize),
		strings.Repeat("ab", vaultsync.IDSize-1),
		strings.Repeat("ab", vaultsync.IDSize+1),
		strings.Repeat("00", vaultsync.IDSize),
	} {
		if _, err := vaultsync.ParseID(in); !errors.Is(err, vaultsync.ErrMalformedID) {
			t.Errorf("ParseID(%q) err = %v, want ErrMalformedID", in, err)
		}
	}
}

func TestParseIDRejectsNonCanonicalCase(t *testing.T) {
	upper := strings.Repeat("AB", vaultsync.IDSize)
	if _, err := vaultsync.ParseID(upper); !errors.Is(err, vaultsync.ErrMalformedID) {
		t.Fatalf("ParseID(%q) err = %v, want ErrMalformedID; two spellings of one id would be two map keys for one record", upper, err)
	}
}

func TestNewIDRefusesShortRandomness(t *testing.T) {
	if _, err := vaultsync.NewID(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("NewID accepted 3 bytes of randomness; a short read must never become an id")
	}
}

func TestNewIDRefusesZero(t *testing.T) {
	_, err := vaultsync.NewID(bytes.NewReader(make([]byte, vaultsync.IDSize)))
	if !errors.Is(err, vaultsync.ErrZeroID) {
		t.Fatalf("err = %v, want ErrZeroID; the zero id means 'absent' everywhere", err)
	}
}

func TestIntegrityErrorsMatchErrIntegrity(t *testing.T) {
	for _, err := range []error{
		vaultsync.ErrMalformedEnvelope, vaultsync.ErrBadPadding, vaultsync.ErrBadSignature,
		vaultsync.ErrUnsupportedSigAlg, vaultsync.ErrDuplicateLeaf, vaultsync.ErrChainBroken,
		vaultsync.ErrMembershipRegressed, vaultsync.ErrRollback, vaultsync.ErrFork,
		vaultsync.ErrDeviceLog, vaultsync.ErrSupersededKey, vaultsync.ErrSuccessionRequired,
		vaultsync.ErrMembership, vaultsync.ErrNotAuthorized, vaultsync.ErrRevokedDevice,
		vaultsync.ErrRotationRequired, vaultsync.ErrUnverifiedMember, vaultsync.ErrVaultDeleted,
	} {
		if !errors.Is(err, vaultsync.ErrIntegrity) {
			t.Errorf("%v does not match ErrIntegrity; callers stop a vault's sync on ErrIntegrity alone", err)
		}
	}
	if errors.Is(vaultsync.ErrMalformedID, vaultsync.ErrIntegrity) {
		t.Error("ErrMalformedID matches ErrIntegrity; a local parse error is not a server attack")
	}
}
