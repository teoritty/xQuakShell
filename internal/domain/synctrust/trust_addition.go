package synctrust

import (
	"encoding/json"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// ClassifyAddition finishes Classify for an item this device does not hold yet: whether it would
// sit beside something this device already trusts under a record of its own. held is this device's
// items as Snapshot produces them.
//
// A known_hosts line is filed under its marker, host field and key type, but a host is vouched for
// by every line whose first host names it. A second key type for the host, the same host written in
// OpenSSH's hashed form, or a marker in front of it is therefore another record - one that applied on
// arrival as a new host would - and the stored key no longer decides alone what this device accepts.
// Such a line waits for the master password like a replaced key does.
func ClassifyAddition(t RecordType, held map[vaultsync.ID]Entry, incoming json.RawMessage) (Verdict, error) {
	if t != TypeKnownHost || incoming == nil {
		return applyVerdict, nil
	}
	in, err := decodeData[knownHostData](incoming)
	if err != nil {
		return Verdict{}, err
	}
	host, ok := knownHostField(in.Line)
	if !ok {
		return Verdict{}, ErrRecordData
	}
	for _, e := range held {
		if e.Type != TypeKnownHost {
			continue
		}
		l, err := decodeData[knownHostData](e.Data)
		if err != nil {
			continue
		}
		if lh, ok := knownHostField(l.Line); ok && sameKnownHost(lh, host) {
			return pending("adds another key for a host this device already knows"), nil
		}
	}
	return applyVerdict, nil
}

// knownHostField is the host a known_hosts line vouches for, as the known_hosts lookup reads it: the
// first entry of the host list, after the marker if there is one.
func knownHostField(line string) (string, bool) {
	f := strings.Fields(line)
	i := 0
	if len(f) > 0 && strings.HasPrefix(f[0], "@") {
		i = 1
	}
	if len(f) <= i {
		return "", false
	}
	host, _, _ := strings.Cut(f[i], ",")
	return host, host != ""
}

// sameKnownHost reports whether two host fields may name one host. Two hashed fields carry different
// salts and cannot be compared without the name they hide, so they count as the same host: holding
// a line for a confirmation costs a click, applying one that shadows a stored key costs the
// connection.
func sameKnownHost(a, b string) bool {
	switch {
	case domain.IsHashedKnownHost(a) && domain.IsHashedKnownHost(b):
		return true
	case domain.IsHashedKnownHost(a):
		return domain.KnownHostMatches(a, b)
	default:
		return domain.KnownHostMatches(b, a)
	}
}
