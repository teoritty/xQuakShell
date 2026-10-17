package synctrust

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// FieldConflict is one credential field two devices set to different values, as JSON.
type FieldConflict struct {
	Path   []string
	Local  json.RawMessage
	Remote json.RawMessage
}

// MergeData merges this device's unpushed version of a record with the remote one against the
// version both started from (spec §7.4), using the record type's credential policy. A nil base means
// both sides created the record independently. Credential conflicts keep the remote value and are
// returned so the caller can keep the local one in a conflict record.
func MergeData(t RecordType, base, local, remote json.RawMessage) (json.RawMessage, []FieldConflict, error) {
	b := map[string]any{}
	if base != nil {
		var err error
		if b, err = decodeObject(base); err != nil {
			return nil, nil, err
		}
	}
	l, err := decodeObject(local)
	if err != nil {
		return nil, nil, err
	}
	r, err := decodeObject(remote)
	if err != nil {
		return nil, nil, err
	}
	res := vaultsync.MergeObjects(b, l, r, FieldPolicyFor(t))
	merged, err := json.Marshal(res.Value)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: encode merged record: %w", ErrRecordData, err)
	}
	conflicts := make([]FieldConflict, 0, len(res.Conflicts))
	for _, c := range res.Conflicts {
		lv, lerr := json.Marshal(c.Local)
		rv, rerr := json.Marshal(c.Remote)
		if lerr != nil || rerr != nil {
			return nil, nil, fmt.Errorf("%w: encode conflict", ErrRecordData)
		}
		conflicts = append(conflicts, FieldConflict{Path: c.Path, Local: lv, Remote: rv})
	}
	return merged, conflicts, nil
}

// decodeObject decodes record data keeping numbers exact: a float64 round trip would corrupt any
// integer above 2^53, and lockout timeouts are nanosecond durations.
func decodeObject(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil || v == nil {
		return nil, fmt.Errorf("%w: record data is not an object", ErrRecordData)
	}
	return v, nil
}

// ConflictEntries turns merge conflicts on one item into conflict records.
func ConflictEntries(t RecordType, itemKey string, conflicts []FieldConflict, atUnixMilli int64) ([]Entry, error) {
	out := make([]Entry, 0, len(conflicts))
	for _, c := range conflicts {
		sc := domain.SyncConflict{ID: domain.ConflictID(string(t), itemKey, c.Path), RecordType: string(t), ItemKey: itemKey,
			Path: c.Path, Local: c.Local, Remote: c.Remote, AtUnixMilli: atUnixMilli}
		data, err := json.Marshal(sc)
		if err != nil {
			return nil, fmt.Errorf("%w: encode conflict: %w", ErrRecordData, err)
		}
		out = append(out, Entry{Type: TypeConflict, Key: sc.ID, Data: data})
	}
	return out, nil
}

// SameData reports whether two record datas hold the same value, ignoring key order and spacing:
// two builds may encode one value differently, and treating that as an edit would push a commit
// for nothing on every sync.
func SameData(a, b json.RawMessage) bool {
	x, errA := decodeObject(a)
	y, errB := decodeObject(b)
	return errA == nil && errB == nil && reflect.DeepEqual(x, y)
}
