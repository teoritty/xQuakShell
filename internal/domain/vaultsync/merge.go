package vaultsync

import (
	"maps"
	"reflect"
	"slices"
)

// FieldClass decides how a true conflict on one field is resolved.
type FieldClass uint8

const (
	// FieldOrdinary conflicts resolve to the rebasing device's value: by server order its write is
	// the later one, and only that device merges, so every device converges on its result.
	FieldOrdinary FieldClass = iota
	// FieldCredential conflicts keep the value already on the server and are reported, because two
	// different passwords or hosts are a choice for a person, not for an algorithm.
	FieldCredential
)

// FieldPolicy classifies a field by its path, e.g. ["users", "[id=u1]", "password"].
type FieldPolicy func(path []string) FieldClass

// AbsentValue marks a key missing from one side. It takes part in comparisons like any value, so
// "deleted here, unchanged there" and "deleted here, edited there" are told apart.
type AbsentValue struct{}

// Absent stands in for a missing key so merge can compare it with real values instead of
// special-casing every lookup; callers drop it from results before encoding.
var Absent = AbsentValue{}

// Conflict is a credential field both sides changed to different values.
type Conflict struct {
	Path   []string
	Local  any
	Remote any
}

// MergeResult is the merged object and the conflicts the caller must surface.
type MergeResult struct {
	Value     map[string]any
	Conflicts []Conflict
}

// MergeObjects merges local (the rebasing device's version) and remote (the version the server
// accepted first) against their common base (spec section 7.4). Different fields both survive;
// objects merge field by field; arrays of objects with an "id" merge element by element; other
// arrays are atomic; an edit beats a deletion; a true conflict follows the field's class.
func MergeObjects(base, local, remote map[string]any, policy FieldPolicy) MergeResult {
	m := merger{policy: policy}
	return MergeResult{Value: m.object(nil, base, local, remote), Conflicts: m.conflicts}
}

type merger struct {
	policy    FieldPolicy
	conflicts []Conflict
}

func (m *merger) object(path []string, base, local, remote map[string]any) map[string]any {
	keys := slices.Collect(maps.Keys(base))
	keys = slices.AppendSeq(keys, maps.Keys(local))
	keys = slices.AppendSeq(keys, maps.Keys(remote))
	slices.Sort(keys)
	out := map[string]any{}
	for _, key := range slices.Compact(keys) {
		v := m.value(childPath(path, key), lookup(base, key), lookup(local, key), lookup(remote, key))
		if v != Absent {
			out[key] = v
		}
	}
	return out
}

func lookup(obj map[string]any, key string) any {
	if v, ok := obj[key]; ok {
		return v
	}
	return Absent
}

// childPath copies rather than appends in place: sibling keys must not share a backing array, or a
// conflict recorded for one would have its path rewritten by the next.
func childPath(path []string, segment string) []string {
	return append(slices.Clip(path), segment)
}

func (m *merger) value(path []string, base, local, remote any) any {
	switch {
	case reflect.DeepEqual(local, remote):
		return local
	case reflect.DeepEqual(local, base):
		return remote
	case reflect.DeepEqual(remote, base):
		return local
	case local == Absent:
		return remote
	case remote == Absent:
		return local
	}
	if lo, ro, bo, ok := asObjects(base, local, remote); ok {
		return m.object(path, bo, lo, ro)
	}
	if merged, ok := m.idArray(path, base, local, remote); ok {
		return merged
	}
	return m.resolve(path, local, remote)
}

func (m *merger) resolve(path []string, local, remote any) any {
	if m.policy(path) == FieldCredential {
		m.conflicts = append(m.conflicts, Conflict{Path: slices.Clone(path), Local: local, Remote: remote})
		return remote
	}
	return local
}

// asObjects succeeds when both sides are objects and the base is an object or absent.
func asObjects(base, local, remote any) (l, r, b map[string]any, ok bool) {
	l, okL := local.(map[string]any)
	r, okR := remote.(map[string]any)
	if !okL || !okR {
		return nil, nil, nil, false
	}
	if base == Absent {
		return l, r, map[string]any{}, true
	}
	b, okB := base.(map[string]any)
	return l, r, b, okB
}
