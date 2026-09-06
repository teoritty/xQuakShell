package domain_test

import (
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
)

func doc(version domain.VersionVector, conns ...domain.Connection) domain.ReplicaDocument {
	return domain.ReplicaDocument{
		Scope:       "com.example.sync",
		Version:     version,
		Connections: conns,
		Folders:     []domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
	}
}

func conn(id, host string) domain.Connection {
	return domain.Connection{ID: id, FolderID: "sync", Host: host}
}

func connIDs(d domain.ReplicaDocument) []string {
	ids := make([]string, 0, len(d.Connections))
	for _, c := range d.Connections {
		ids = append(ids, c.ID)
	}
	slices.Sort(ids)
	return ids
}

// A document for someone else's scope must never be applied, whatever its version says. Scope is
// what decides who may read these credentials, so applying one to the wrong folder would move them
// somewhere the user never agreed to.
func TestAReplicaForAnotherScopeIsRefused(t *testing.T) {
	local := doc(domain.VersionVector{"a": 1})
	remote := doc(domain.VersionVector{"a": 2})
	remote.Scope = "com.example.other"

	if _, err := domain.MergeReplica(local, remote); !errors.Is(err, domain.ErrScopeMismatch) {
		t.Fatalf("err = %v, want ErrScopeMismatch", err)
	}
}

// The ordinary case: the other device has everything this one has and more, so its content is taken
// whole. This is safe precisely because the vector says so - a remote that is strictly ahead has
// already seen and kept this device's edits.
func TestARemoteThatIsAheadIsTakenWhole(t *testing.T) {
	local := doc(domain.VersionVector{"a": 1}, conn("c1", "old"))
	remote := doc(domain.VersionVector{"a": 1, "b": 1}, conn("c1", "new"), conn("c2", "added"))

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if merged.Order != domain.ReplicaBehind {
		t.Fatalf("Order = %v, want ReplicaBehind", merged.Order)
	}
	if !slices.Equal(connIDs(merged.Result), []string{"c1", "c2"}) {
		t.Fatalf("result = %v, want both connections", connIDs(merged.Result))
	}
	for _, c := range merged.Result.Connections {
		if c.ID == "c1" && c.Host != "new" {
			t.Errorf("the newer side's edit was not taken: host = %q", c.Host)
		}
	}
	if len(merged.Conflicts) != 0 {
		t.Errorf("a one-sided update reported conflicts: %v", merged.Conflicts)
	}
}

// Nothing to do when this device is the newer one. The remote will catch up on the next push.
func TestARemoteThatIsBehindChangesNothing(t *testing.T) {
	local := doc(domain.VersionVector{"a": 2}, conn("c1", "mine"))
	remote := doc(domain.VersionVector{"a": 1}, conn("c1", "stale"))

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if merged.Order != domain.ReplicaAhead {
		t.Fatalf("Order = %v, want ReplicaAhead", merged.Order)
	}
	if merged.Result.Connections[0].Host != "mine" {
		t.Fatal("a stale remote overwrote the local connection")
	}
}

// The case a file sync gets wrong. Both devices added something while apart, and both additions are
// kept - neither side is picked, because there is nothing in either version to say which afternoon
// of work matters less.
func TestDivergentAdditionsAreBothKept(t *testing.T) {
	local := doc(domain.VersionVector{"a": 2, "b": 1}, conn("c1", "shared"), conn("mine", "x"))
	remote := doc(domain.VersionVector{"a": 1, "b": 2}, conn("c1", "shared"), conn("theirs", "y"))

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if merged.Order != domain.ReplicaDiverged {
		t.Fatalf("Order = %v, want ReplicaDiverged", merged.Order)
	}
	if !slices.Equal(connIDs(merged.Result), []string{"c1", "mine", "theirs"}) {
		t.Fatalf("result = %v, want every addition kept", connIDs(merged.Result))
	}
	if len(merged.Conflicts) != 0 {
		t.Errorf("additions that do not overlap were reported as conflicts: %v", merged.Conflicts)
	}
}

// The same object edited on both sides is the one thing that cannot be merged without guessing, so
// it is reported and left alone. The local value stands until the user says otherwise; overwriting
// it would be the silent loss this whole design exists to avoid.
func TestTheSameObjectEditedOnBothSidesIsAConflict(t *testing.T) {
	local := doc(domain.VersionVector{"a": 2, "b": 1}, conn("c1", "mine"))
	remote := doc(domain.VersionVector{"a": 1, "b": 2}, conn("c1", "theirs"))

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if !slices.Equal(merged.Conflicts, []string{"c1"}) {
		t.Fatalf("Conflicts = %v, want the object edited on both sides", merged.Conflicts)
	}
	if merged.Result.Connections[0].Host != "mine" {
		t.Fatal("a conflicting remote value overwrote the local one")
	}
}

// Deletion never arrives over the wire (I9). An object this device has and the newer remote does
// not is reported so the user can be asked, and kept until they are - a wiped server must not be
// able to empty a vault by serving a document with nothing in it.
func TestSomethingMissingFromTheRemoteIsProposedNotRemoved(t *testing.T) {
	local := doc(domain.VersionVector{"a": 1}, conn("c1", "x"), conn("c2", "y"))
	remote := doc(domain.VersionVector{"a": 2}, conn("c1", "x"))

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if !slices.Equal(merged.Removed, []string{"c2"}) {
		t.Fatalf("Removed = %v, want the object the remote no longer has", merged.Removed)
	}
	if !slices.Equal(connIDs(merged.Result), []string{"c1", "c2"}) {
		t.Fatalf("result = %v, want the object kept until the user is asked", connIDs(merged.Result))
	}
}

// The extreme of the same rule, and the scenario that prompted it: everything gone from the server.
// The local vault is untouched and the whole loss is reported as a proposal.
func TestAnEmptyRemoteRemovesNothing(t *testing.T) {
	local := doc(domain.VersionVector{"a": 1}, conn("c1", "x"), conn("c2", "y"))
	remote := doc(domain.VersionVector{"a": 2})

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if !slices.Equal(connIDs(merged.Result), []string{"c1", "c2"}) {
		t.Fatalf("an empty remote emptied the result: %v", connIDs(merged.Result))
	}
	if !slices.Equal(merged.Removed, []string{"c1", "c2"}) {
		t.Fatalf("Removed = %v, want both proposed", merged.Removed)
	}
}

// The merged version has to record that this device has now seen the other's history, or the very
// next comparison reports the divergence that was just resolved.
func TestTheMergedVersionRecordsBothHistories(t *testing.T) {
	local := doc(domain.VersionVector{"a": 2, "b": 1})
	remote := doc(domain.VersionVector{"a": 1, "b": 2})

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if merged.Result.Version["a"] != 2 || merged.Result.Version["b"] != 2 {
		t.Fatalf("Version = %v, want the highest seen of each", merged.Result.Version)
	}
}

// Folders merge on the same terms as connections: the tree inside the scope is the user's own
// arrangement, and losing it would leave the connections in a shape nobody chose.
func TestFoldersMergeTheSameWayConnectionsDo(t *testing.T) {
	local := doc(domain.VersionVector{"a": 2, "b": 1})
	local.Folders = append(local.Folders, domain.ConnectionFolder{ID: "mine", Name: "Mine", ParentID: "sync"})
	remote := doc(domain.VersionVector{"a": 1, "b": 2})
	remote.Folders = append(remote.Folders, domain.ConnectionFolder{ID: "theirs", Name: "Theirs", ParentID: "sync"})

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	ids := make([]string, 0, len(merged.Result.Folders))
	for _, folder := range merged.Result.Folders {
		ids = append(ids, folder.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"mine", "sync", "theirs"}) {
		t.Fatalf("folders = %v, want both sides kept", ids)
	}
}

// Secrets travel with their objects, so they merge with them. A connection that arrived without the
// password it references would be a connection that cannot be used.
func TestSecretsArriveWithTheObjectsThatNeedThem(t *testing.T) {
	local := doc(domain.VersionVector{"a": 1})
	remote := doc(domain.VersionVector{"a": 2}, conn("c1", "x"))
	remote.Passwords = map[string]domain.PasswordBlob{"p1": {Value: []byte("secret")}}

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if _, arrived := merged.Result.Passwords["p1"]; !arrived {
		t.Fatal("the password the arriving connection needs did not come with it")
	}
}

// Nothing to do at all is not an error, and must not report work that did not happen.
func TestIdenticalReplicasMergeToNothing(t *testing.T) {
	local := doc(domain.VersionVector{"a": 1}, conn("c1", "x"))
	remote := doc(domain.VersionVector{"a": 1}, conn("c1", "x"))

	merged, err := domain.MergeReplica(local, remote)
	if err != nil {
		t.Fatalf("MergeReplica: %v", err)
	}

	if merged.Order != domain.ReplicaSame {
		t.Errorf("Order = %v, want ReplicaSame", merged.Order)
	}
	if len(merged.Conflicts) != 0 || len(merged.Removed) != 0 {
		t.Errorf("identical replicas reported conflicts %v and removals %v", merged.Conflicts, merged.Removed)
	}
}
