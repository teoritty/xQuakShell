package vaultsync_test

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func obj(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

// credentialPolicy treats "host" and "password" anywhere in the path as credential fields.
func credentialPolicy(path []string) vaultsync.FieldClass {
	last := path[len(path)-1]
	if last == "host" || last == "password" {
		return vaultsync.FieldCredential
	}
	return vaultsync.FieldOrdinary
}

func merge(t *testing.T, base, local, remote string) vaultsync.MergeResult {
	t.Helper()
	return vaultsync.MergeObjects(obj(t, base), obj(t, local), obj(t, remote), credentialPolicy)
}

func assertValue(t *testing.T, got vaultsync.MergeResult, want string) {
	t.Helper()
	if !reflect.DeepEqual(got.Value, obj(t, want)) {
		gotJSON, _ := json.Marshal(got.Value)
		t.Fatalf("merged = %s, want %s", gotJSON, want)
	}
}

func TestMergeKeepsChangesToDifferentFields(t *testing.T) {
	got := merge(t, `{"name":"a","port":22}`, `{"name":"b","port":22}`, `{"name":"a","port":2222}`)
	assertValue(t, got, `{"name":"b","port":2222}`)
	if len(got.Conflicts) != 0 {
		t.Fatalf("conflicts = %v, want none", got.Conflicts)
	}
}

func TestMergeOrdinaryConflictLocalWins(t *testing.T) {
	got := merge(t, `{"name":"a"}`, `{"name":"mine"}`, `{"name":"theirs"}`)
	assertValue(t, got, `{"name":"mine"}`)
	if len(got.Conflicts) != 0 {
		t.Fatal("an ordinary field produced a conflict; the rebasing device's later write wins silently")
	}
}

func TestMergeCredentialConflictKeepsRemoteAndReports(t *testing.T) {
	got := merge(t, `{"host":"a"}`, `{"host":"mine"}`, `{"host":"theirs"}`)
	assertValue(t, got, `{"host":"theirs"}`)
	if len(got.Conflicts) != 1 || !slices.Equal(got.Conflicts[0].Path, []string{"host"}) {
		t.Fatalf("conflicts = %+v; a credential field must never be resolved silently", got.Conflicts)
	}
	if got.Conflicts[0].Local != "mine" || got.Conflicts[0].Remote != "theirs" {
		t.Fatalf("conflict values = %+v, want mine/theirs", got.Conflicts[0])
	}
}

func TestMergeRecursesIntoObjects(t *testing.T) {
	got := merge(t, `{"jump":{"user":"u","port":1}}`, `{"jump":{"user":"v","port":1}}`, `{"jump":{"user":"u","port":2}}`)
	assertValue(t, got, `{"jump":{"user":"v","port":2}}`)
}

func TestMergeObjectAddedOnBothSides(t *testing.T) {
	got := merge(t, `{}`, `{"jump":{"user":"v"}}`, `{"jump":{"port":2}}`)
	assertValue(t, got, `{"jump":{"user":"v","port":2}}`)
}

func TestMergeArraysByID(t *testing.T) {
	base := `{"users":[{"id":"u1","name":"a"}]}`
	got := merge(t, base, `{"users":[{"id":"u1","name":"b"}]}`, `{"users":[{"id":"u1","name":"a"},{"id":"u2","name":"c"}]}`)
	assertValue(t, got, `{"users":[{"id":"u1","name":"b"},{"id":"u2","name":"c"}]}`)
}

func TestMergeArrayElementCredentialConflictPath(t *testing.T) {
	got := merge(t, `{"users":[{"id":"u1","password":"p"}]}`, `{"users":[{"id":"u1","password":"x"}]}`, `{"users":[{"id":"u1","password":"y"}]}`)
	want := []string{"users", "[id=u1]", "password"}
	if len(got.Conflicts) != 1 || !slices.Equal(got.Conflicts[0].Path, want) {
		t.Fatalf("conflicts = %+v, want one at %v", got.Conflicts, want)
	}
}

func TestMergeEditBeatsDelete(t *testing.T) {
	got := merge(t, `{"users":[{"id":"u1","name":"a"}],"note":"n"}`, `{"users":[]}`, `{"users":[{"id":"u1","name":"edited"}],"note":"changed"}`)
	assertValue(t, got, `{"users":[{"id":"u1","name":"edited"}],"note":"changed"}`)
	got = merge(t, `{"note":"n"}`, `{"note":"changed"}`, `{}`)
	assertValue(t, got, `{"note":"changed"}`)
}

func TestMergeDeleteAgainstUnchangedDeletes(t *testing.T) {
	got := merge(t, `{"users":[{"id":"u1"}],"note":"n"}`, `{"users":[]}`, `{"users":[{"id":"u1"}],"note":"n"}`)
	assertValue(t, got, `{"users":[]}`)
}

func TestMergeAtomicArrays(t *testing.T) {
	got := merge(t, `{"tags":["a"]}`, `{"tags":["a","b"]}`, `{"tags":["c"]}`)
	assertValue(t, got, `{"tags":["a","b"]}`)
}

func TestMergeArraysWithoutUniqueIDsAreAtomic(t *testing.T) {
	got := merge(t, `{"u":[{"id":"x"}]}`, `{"u":[{"id":"x"},{"id":"x"}]}`, `{"u":[{"id":"y"}]}`)
	assertValue(t, got, `{"u":[{"id":"x"},{"id":"x"}]}`)
	got = merge(t, `{"u":[{"n":1}]}`, `{"u":[{"n":2}]}`, `{"u":[{"n":3}]}`)
	assertValue(t, got, `{"u":[{"n":2}]}`)
}

func TestMergeIdentities(t *testing.T) {
	samples := []string{`{}`, `{"a":1}`, `{"a":{"b":[{"id":"x","host":"h"}]},"tags":["t"]}`}
	for _, b := range samples {
		for _, x := range samples {
			if got := merge(t, b, x, x); !reflect.DeepEqual(got.Value, obj(t, x)) {
				t.Errorf("merge(%s, x, x) != x for x=%s", b, x)
			}
			if got := merge(t, b, b, x); !reflect.DeepEqual(got.Value, obj(t, x)) {
				t.Errorf("merge(b, b, r) != r for b=%s r=%s", b, x)
			}
			if got := merge(t, b, x, b); !reflect.DeepEqual(got.Value, obj(t, x)) {
				t.Errorf("merge(b, l, b) != l for b=%s l=%s", b, x)
			}
		}
	}
}

// Generated flat objects: a key changed on one side only takes that side's value, and the result
// never holds a value neither side had - merge does not invent data.
func TestMergePropertiesOnGeneratedObjects(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	keys := []string{"name", "host", "port", "note"}
	values := []any{json.Number("1"), json.Number("2"), "x", "y", vaultsync.Absent}
	pick := func() any { return values[r.IntN(len(values))] }
	build := func(m map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range m {
			if v != vaultsync.Absent {
				out[k] = v
			}
		}
		return out
	}
	for i := range 2000 {
		b, l, rm := map[string]any{}, map[string]any{}, map[string]any{}
		for _, k := range keys {
			b[k], l[k], rm[k] = pick(), pick(), pick()
		}
		got := vaultsync.MergeObjects(build(b), build(l), build(rm), credentialPolicy).Value
		for _, k := range keys {
			v, ok := got[k]
			if !ok {
				v = vaultsync.Absent
			}
			switch {
			case l[k] == b[k] && v != rm[k]:
				t.Fatalf("case %d key %s: only remote changed (%v→%v) but result is %v", i, k, b[k], rm[k], v)
			case rm[k] == b[k] && v != l[k]:
				t.Fatalf("case %d key %s: only local changed (%v→%v) but result is %v", i, k, b[k], l[k], v)
			case v != l[k] && v != rm[k]:
				t.Fatalf("case %d key %s: result %v is neither local %v nor remote %v", i, k, v, l[k], rm[k])
			}
		}
	}
}

func FuzzMergeObjects(f *testing.F) {
	f.Add(`{"a":1}`, `{"a":2}`, `{"a":3}`)
	f.Add(`{"u":[{"id":"x"}]}`, `{"u":[]}`, `{"u":[{"id":"x","p":1}]}`)
	f.Fuzz(func(t *testing.T, b, l, r string) {
		var mb, ml, mr map[string]any
		if json.Unmarshal([]byte(b), &mb) != nil || json.Unmarshal([]byte(l), &ml) != nil || json.Unmarshal([]byte(r), &mr) != nil {
			return
		}
		got := vaultsync.MergeObjects(mb, ml, mr, credentialPolicy)
		if _, err := json.Marshal(got.Value); err != nil {
			t.Fatalf("merge produced an unencodable value: %v", err)
		}
	})
}
