package vaultsync_test

import (
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func ver(maj, min, patch uint16) vaultsync.ProtocolVersion {
	return vaultsync.ProtocolVersion{Major: maj, Minor: min, Patch: patch}
}

func TestNegotiateProtocol(t *testing.T) {
	cases := []struct {
		name                 string
		client, lo, hi, want vaultsync.ProtocolVersion
		wantErr              bool
	}{
		{"same", ver(1, 2, 0), ver(1, 0, 0), ver(1, 2, 0), ver(1, 2, 0), false},
		{"client newer minor speaks server max", ver(1, 4, 0), ver(1, 0, 0), ver(1, 2, 3), ver(1, 2, 3), false},
		{"client older minor within range", ver(1, 1, 0), ver(1, 0, 0), ver(1, 3, 0), ver(1, 1, 0), false},
		{"client at server minimum", ver(1, 1, 0), ver(1, 1, 0), ver(1, 3, 0), ver(1, 1, 0), false},
		{"client below server minimum", ver(1, 0, 0), ver(1, 1, 0), ver(1, 3, 0), vaultsync.ProtocolVersion{}, true},
		{"different major", ver(2, 0, 0), ver(1, 0, 0), ver(1, 9, 0), vaultsync.ProtocolVersion{}, true},
		{"single-version server range", ver(1, 2, 0), ver(1, 2, 0), ver(1, 2, 0), ver(1, 2, 0), false},
		{"inverted server range", ver(1, 0, 0), ver(1, 2, 0), ver(1, 1, 0), vaultsync.ProtocolVersion{}, true},
	}
	for _, c := range cases {
		got, err := vaultsync.NegotiateProtocol(c.client, c.lo, c.hi)
		if c.wantErr {
			if !errors.Is(err, vaultsync.ErrProtocolUnsupported) {
				t.Errorf("%s: err = %v, want ErrProtocolUnsupported", c.name, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestProtocolVersionCompareAndString(t *testing.T) {
	if ver(1, 2, 3).Compare(ver(1, 2, 4)) >= 0 || ver(1, 3, 0).Compare(ver(1, 2, 9)) <= 0 || ver(2, 0, 0).Compare(ver(1, 9, 9)) <= 0 {
		t.Error("Compare does not order by major, then minor, then patch")
	}
	if ver(1, 2, 3).Compare(ver(1, 2, 3)) != 0 {
		t.Error("equal versions do not compare equal")
	}
	if got := ver(1, 2, 3).String(); got != "1.2.3" {
		t.Errorf("String = %q, want 1.2.3", got)
	}
}

func TestRecordAccess(t *testing.T) {
	known := map[string]uint16{"connection": 2}
	if got := vaultsync.RecordAccess(known, "connection", 2); got != vaultsync.SchemaWritable {
		t.Errorf("same schema: %d, want writable", got)
	}
	if got := vaultsync.RecordAccess(known, "connection", 1); got != vaultsync.SchemaWritable {
		t.Errorf("older schema: %d, want writable (this build upgrades it on write)", got)
	}
	if got := vaultsync.RecordAccess(known, "connection", 3); got != vaultsync.SchemaReadOnly {
		t.Errorf("newer schema: %d, want read-only; writing it would drop fields this build cannot see", got)
	}
	if got := vaultsync.RecordAccess(known, "future-type", 1); got != vaultsync.SchemaOpaque {
		t.Errorf("unknown type: %d, want opaque", got)
	}
}

func TestHelloContextBindsTheOffer(t *testing.T) {
	key := []byte("identity")
	base, _ := vaultsync.HelloContext(key, ver(1, 0, 0), ver(1, 2, 0), []vaultsync.SuiteID{1})
	variants := map[string][]byte{}
	variants["key"], _ = vaultsync.HelloContext([]byte("other"), ver(1, 0, 0), ver(1, 2, 0), []vaultsync.SuiteID{1})
	variants["max"], _ = vaultsync.HelloContext(key, ver(1, 0, 0), ver(1, 1, 0), []vaultsync.SuiteID{1})
	variants["min"], _ = vaultsync.HelloContext(key, ver(1, 1, 0), ver(1, 2, 0), []vaultsync.SuiteID{1})
	variants["suites"], _ = vaultsync.HelloContext(key, ver(1, 0, 0), ver(1, 2, 0), []vaultsync.SuiteID{1, 2})
	for name, got := range variants {
		if string(got) == string(base) {
			t.Errorf("changing the offered %s left the AKE context unchanged; a downgrade would go unnoticed", name)
		}
	}
}
