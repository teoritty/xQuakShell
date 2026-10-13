package synctrust_test

import (
	"encoding/json"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMergeKeepsBothSidesOrdinaryEdits(t *testing.T) {
	base := domain.Connection{ID: "c", Name: "web", Host: "h", Port: 22, Tags: []string{"a"}}
	local, remote := base, base
	local.Name = "web-local"
	remote.Tags = []string{"a", "b"}
	merged, conflicts, err := synctrust.MergeData(synctrust.TypeConnection, mustJSON(t, base), mustJSON(t, local), mustJSON(t, remote))
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("merge: %v, conflicts %+v", err, conflicts)
	}
	var got domain.Connection
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "web-local" || len(got.Tags) != 2 || got.Port != 22 {
		t.Fatalf("merged = %+v; both edits must survive", got)
	}
}

func TestMergeCredentialConflictKeepsTheServersValue(t *testing.T) {
	base := domain.Connection{ID: "c", Host: "old", Port: 22}
	local, remote := base, base
	local.Host, remote.Host = "mine", "theirs"
	merged, conflicts, err := synctrust.MergeData(synctrust.TypeConnection, mustJSON(t, base), mustJSON(t, local), mustJSON(t, remote))
	if err != nil {
		t.Fatal(err)
	}
	var got domain.Connection
	_ = json.Unmarshal(merged, &got)
	if got.Host != "theirs" {
		t.Fatalf("host = %q; a credential conflict keeps the value the server accepted first", got.Host)
	}
	if len(conflicts) != 1 || conflicts[0].Path[0] != "host" || string(conflicts[0].Local) != `"mine"` || string(conflicts[0].Remote) != `"theirs"` {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestMergeOrdinaryConflictTakesThisDevicesValue(t *testing.T) {
	base := domain.ConnectionFolder{ID: "f", Name: "a"}
	local, remote := base, base
	local.Name, remote.Name = "mine", "theirs"
	merged, conflicts, _ := synctrust.MergeData(synctrust.TypeFolder, mustJSON(t, base), mustJSON(t, local), mustJSON(t, remote))
	var got domain.ConnectionFolder
	_ = json.Unmarshal(merged, &got)
	if got.Name != "mine" || len(conflicts) != 0 {
		t.Fatalf("merged %+v, conflicts %+v", got, conflicts)
	}
}

func TestMergeWithoutABaseTreatsBothAsNew(t *testing.T) {
	local := domain.TerminalSettings{FontFamily: "x", FontSize: 12}
	remote := domain.TerminalSettings{FontFamily: "x", FontSize: 14}
	merged, _, err := synctrust.MergeData(synctrust.SettingsType("terminal"), nil, mustJSON(t, local), mustJSON(t, remote))
	if err != nil {
		t.Fatal(err)
	}
	var got domain.TerminalSettings
	_ = json.Unmarshal(merged, &got)
	if got.FontSize != 12 {
		t.Fatalf("merged = %+v", got)
	}
}

func TestMergeKeepsLargeIntegersExact(t *testing.T) {
	base := domain.LockoutSettings{Enabled: true, IdleTimeout: 9007199254740993}
	local := base
	local.LockOnMinimize = true
	merged, _, _ := synctrust.MergeData(synctrust.SettingsType("lockout"), mustJSON(t, base), mustJSON(t, local), mustJSON(t, base))
	var got domain.LockoutSettings
	_ = json.Unmarshal(merged, &got)
	if got.IdleTimeout != 9007199254740993 {
		t.Fatalf("idle timeout = %d; numbers must not pass through float64", got.IdleTimeout)
	}
}

func TestMergeRefusesNonObjects(t *testing.T) {
	if _, _, err := synctrust.MergeData(synctrust.TypeFolder, nil, json.RawMessage(`[1]`), json.RawMessage(`{}`)); err == nil {
		t.Fatal("an array merged as a record")
	}
}

func TestConflictEntries(t *testing.T) {
	entries, err := synctrust.ConflictEntries(synctrust.TypePassword, "p1",
		[]synctrust.FieldConflict{{Path: []string{"value"}, Local: json.RawMessage(`"a"`), Remote: json.RawMessage(`"b"`)}}, 9)
	if err != nil || len(entries) != 1 || entries[0].Type != synctrust.TypeConflict {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	if err := synctrust.Validate(entries[0]); err != nil {
		t.Fatalf("a conflict entry the engine built does not validate: %v", err)
	}
}

func TestSameData(t *testing.T) {
	if !synctrust.SameData(json.RawMessage(`{"a":1,"b":[1,2]}`), json.RawMessage(`{"b":[1,2], "a":1}`)) {
		t.Error("the same object in another key order compared different")
	}
	if synctrust.SameData(json.RawMessage(`{"a":1}`), json.RawMessage(`{"a":1.0000000000000001}`)) {
		t.Error("two different numbers compared equal")
	}
	if synctrust.SameData(json.RawMessage(`{"a":1}`), json.RawMessage(`nope`)) {
		t.Error("undecodable data compared equal")
	}
}
