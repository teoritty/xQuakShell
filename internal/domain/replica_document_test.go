package domain_test

import (
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
)

// replicaVault is one scope and one folder outside it:
//
//	sync/       <- the scope of com.example.sync, holds "in-scope"
//	personal/   <- holds "outside"
func replicaVault(t *testing.T) (*domain.VaultData, domain.ScopeIndex) {
	t.Helper()
	data := domain.NewVaultData()
	data.Folders = []domain.ConnectionFolder{
		{ID: "sync", Name: "Sync"},
		{ID: "personal", Name: "Personal"},
	}
	data.Connections = []domain.Connection{
		{ID: "in-scope", FolderID: "sync", Host: "a", Users: []domain.ConnectionUser{{
			ID: "u1", PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"},
		}}},
		{ID: "outside", FolderID: "personal", Host: "b", Users: []domain.ConnectionUser{{
			ID: "u2", PassAuth: &domain.PasswordAuthConfig{VaultRef: "p2"},
		}}},
	}
	data.Passwords = map[string]domain.PasswordBlob{
		"p1": {Value: []byte("in")},
		"p2": {Value: []byte("out")},
	}
	index, err := domain.NewScopeIndex(data.Folders,
		[]domain.ScopeRoot{{FolderID: "sync", PluginID: "com.example.sync"}})
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}
	return data, index
}

func build(t *testing.T, data *domain.VaultData, index domain.ScopeIndex) domain.ReplicaDocument {
	t.Helper()
	doc, err := domain.BuildReplicaDocument(data, index, "com.example.sync",
		domain.VersionVector{"this-device": 1})
	if err != nil {
		t.Fatalf("BuildReplicaDocument: %v", err)
	}
	return doc
}

// What is in the folder leaves the machine, and what is beside it does not. This is the whole
// promise of the scope, expressed as the thing that actually gets serialized.
func TestOnlyTheScopeTravels(t *testing.T) {
	data, index := replicaVault(t)

	doc := build(t, data, index)

	if len(doc.Connections) != 1 || doc.Connections[0].ID != "in-scope" {
		t.Fatalf("connections = %v, want only the one inside the scope", doc.Connections)
	}
	if len(doc.Folders) != 1 || doc.Folders[0].ID != "sync" {
		t.Fatalf("folders = %v, want only the scope folder", doc.Folders)
	}
}

// A connection arrives useless without the password it references, so the closure travels with it.
// This is what "putting something in the folder sends its secrets too" means in the code.
func TestTheClosureOfAConnectionTravelsWithIt(t *testing.T) {
	data, index := replicaVault(t)

	doc := build(t, data, index)

	if _, carried := doc.Passwords["p1"]; !carried {
		t.Error("the password the in-scope connection uses did not travel")
	}
	if _, carried := doc.Passwords["p2"]; carried {
		t.Error("a password only an out-of-scope connection uses travelled")
	}
}

// A key the user was promised could not leave the vault must stop the document being built at all,
// rather than travelling or being quietly dropped.
//
// Dropping it would be worse than refusing: the connection would arrive on the other device
// referencing a key that is not there, and the user would be told nothing.
func TestANonExportableKeyInTheScopeRefusesTheWholeDocument(t *testing.T) {
	data, index := replicaVault(t)
	data.Identities = map[string]domain.SSHIdentity{
		"k1": {ID: "k1", NonExportable: true},
	}
	data.KeyBlobs = map[string]domain.IdentityBlob{"k1": {PEMData: []byte("pem")}}
	data.Connections[0].Users[0].KeyAuth = &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}}

	_, err := domain.BuildReplicaDocument(data, index, "com.example.sync", nil)

	if !errors.Is(err, domain.ErrNonExportableInScope) {
		t.Fatalf("err = %v, want ErrNonExportableInScope", err)
	}
}

// An exportable key travels with its blob, or the connection cannot authenticate on the other side.
func TestAnExportableKeyTravelsWithItsBlob(t *testing.T) {
	data, index := replicaVault(t)
	data.Identities = map[string]domain.SSHIdentity{"k1": {ID: "k1"}}
	data.KeyBlobs = map[string]domain.IdentityBlob{"k1": {PEMData: []byte("pem")}}
	data.Connections[0].Users[0].KeyAuth = &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}}

	doc := build(t, data, index)

	if _, carried := doc.Identities["k1"]; !carried {
		t.Error("the identity did not travel")
	}
	if _, carried := doc.KeyBlobs["k1"]; !carried {
		t.Error("the key blob did not travel, so the identity arrives unusable")
	}
}

// A jump hop authenticates too, so what it references is part of the closure. A walk that only
// looked at the connection's own users would send a connection whose first hop cannot be made.
func TestAJumpHopPullsItsOwnSecretsIntoTheClosure(t *testing.T) {
	data, index := replicaVault(t)
	data.Passwords["hop"] = domain.PasswordBlob{Value: []byte("hop")}
	data.Connections[0].JumpChain = domain.JumpChainConfig{Hops: []domain.JumpHop{{
		ID: "j1", Host: "bastion", PassAuth: &domain.PasswordAuthConfig{VaultRef: "hop"},
	}}}

	doc := build(t, data, index)

	if _, carried := doc.Passwords["hop"]; !carried {
		t.Error("the jump hop's password did not travel")
	}
}

// Nothing that is merely present in the vault travels. The document carries the closure of the
// scope, not the vault with the out-of-scope parts removed.
func TestUnreferencedSecretsDoNotTravel(t *testing.T) {
	data, index := replicaVault(t)
	data.Passwords["unused"] = domain.PasswordBlob{Value: []byte("unused")}
	data.Identities = map[string]domain.SSHIdentity{"unused": {ID: "unused"}}

	doc := build(t, data, index)

	if _, carried := doc.Passwords["unused"]; carried {
		t.Error("a password nothing references travelled")
	}
	if _, carried := doc.Identities["unused"]; carried {
		t.Error("an identity nothing references travelled")
	}
}

// An empty scope is the ordinary state of a freshly installed plugin, not an error, and what it
// sends must be empty rather than everything.
func TestAnEmptyScopeProducesAnEmptyDocument(t *testing.T) {
	data := domain.NewVaultData()
	index, err := domain.NewScopeIndex(nil, nil)
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}

	doc, err := domain.BuildReplicaDocument(data, index, "com.example.sync", nil)
	if err != nil {
		t.Fatalf("BuildReplicaDocument: %v", err)
	}

	if len(doc.Connections) != 0 || len(doc.Folders) != 0 || len(doc.Passwords) != 0 {
		t.Fatalf("an empty scope produced %+v", doc)
	}
}

// The document says whose scope it is and how much history it has seen. Without both, the other
// side cannot tell what it is looking at or whether it is newer.
func TestTheDocumentNamesItsScopeAndItsVersion(t *testing.T) {
	data, index := replicaVault(t)

	doc := build(t, data, index)

	if doc.Scope != "com.example.sync" {
		t.Errorf("Scope = %q", doc.Scope)
	}
	if doc.Version["this-device"] != 1 {
		t.Errorf("Version = %v, want the vector it was built with", doc.Version)
	}
}

// Nested folders inside the scope travel with it, because the user's own arrangement inside the
// folder is part of what the other device has to reproduce.
func TestNestedFoldersInsideTheScopeTravel(t *testing.T) {
	data, _ := replicaVault(t)
	data.Folders = append(data.Folders, domain.ConnectionFolder{ID: "eu", Name: "EU", ParentID: "sync"})
	data.Connections = append(data.Connections, domain.Connection{ID: "deep", FolderID: "eu", Host: "c"})
	index, err := domain.NewScopeIndex(data.Folders,
		[]domain.ScopeRoot{{FolderID: "sync", PluginID: "com.example.sync"}})
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}

	doc := build(t, data, index)

	ids := make([]string, 0, len(doc.Folders))
	for _, folder := range doc.Folders {
		ids = append(ids, folder.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"eu", "sync"}) {
		t.Fatalf("folders = %v, want the scope root and the folder nested in it", ids)
	}
	if len(doc.Connections) != 2 {
		t.Fatalf("%d connections travelled, want the nested one too", len(doc.Connections))
	}
}

// A vault that could not be read is an error rather than an empty document: sending "nothing" would
// look to the other device exactly like the user having deleted everything.
func TestANilVaultIsRefusedRatherThanSentAsEmpty(t *testing.T) {
	index, err := domain.NewScopeIndex(nil, nil)
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}

	if _, err := domain.BuildReplicaDocument(nil, index, "com.example.sync", nil); err == nil {
		t.Fatal("a nil vault produced a document instead of an error")
	}
}

// A reference to something that is no longer in the vault is skipped rather than fatal. A dangling
// reference is an ordinary consequence of deleting a key that a connection still names, and refusing
// to build the document over it would stop the whole scope synchronising because of one stale field.
func TestADanglingReferenceIsSkippedRatherThanFatal(t *testing.T) {
	data, index := replicaVault(t)
	data.Connections[0].Users[0].KeyAuth = &domain.KeyAuthConfig{IdentityIDs: []string{"gone"}}
	data.Connections[0].Users[0].PassAuth = &domain.PasswordAuthConfig{VaultRef: "also-gone"}

	doc := build(t, data, index)

	if len(doc.Connections) != 1 {
		t.Fatalf("%d connections travelled, want the one with the stale reference", len(doc.Connections))
	}
	if len(doc.Identities) != 0 || len(doc.Passwords) != 0 {
		t.Fatalf("something travelled for a reference that names nothing: %v %v", doc.Identities, doc.Passwords)
	}
}

// A jump hop can name a non-exportable key just as a user can, and the refusal has to reach it. A
// check that looked only at the connection's own users would send the key one hop earlier.
func TestANonExportableKeyInAJumpHopAlsoRefuses(t *testing.T) {
	data, index := replicaVault(t)
	data.Identities = map[string]domain.SSHIdentity{"k1": {ID: "k1", NonExportable: true}}
	data.Connections[0].JumpChain = domain.JumpChainConfig{Hops: []domain.JumpHop{{
		ID: "j1", Host: "bastion", KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}},
	}}}

	_, err := domain.BuildReplicaDocument(data, index, "com.example.sync", nil)

	if !errors.Is(err, domain.ErrNonExportableInScope) {
		t.Fatalf("err = %v, want ErrNonExportableInScope for a key used by a jump hop", err)
	}
}
