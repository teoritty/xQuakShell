package domain_test

import (
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
)

// applyVault is a vault with one scope and one folder outside it, and a secret in each:
//
//	sync/       <- the scope of com.example.sync, holds "in-scope" using password "p1"
//	personal/   <- holds "outside" using password "p2"
func applyVault(t *testing.T) (*domain.VaultData, domain.ScopeIndex) {
	t.Helper()
	data := domain.NewVaultData()
	data.Folders = []domain.ConnectionFolder{
		{ID: "sync", Name: "Sync"},
		{ID: "personal", Name: "Personal"},
	}
	data.Connections = []domain.Connection{
		{ID: "in-scope", FolderID: "sync", Host: "a", Port: 22, Users: []domain.ConnectionUser{{
			ID: "u1", PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"},
		}}},
		{ID: "outside", FolderID: "personal", Host: "b", Port: 22, Users: []domain.ConnectionUser{{
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

func arriving(conns ...domain.Connection) domain.ReplicaDocument {
	return domain.ReplicaDocument{
		Scope:       "com.example.sync",
		Version:     domain.VersionVector{"other": 1},
		Folders:     []domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
		Connections: conns,
	}
}

func apply(t *testing.T, data *domain.VaultData, index domain.ScopeIndex, doc domain.ReplicaDocument) domain.ReplicaApplied {
	t.Helper()
	applied, err := domain.ApplyReplica(data, index, "com.example.sync", doc)
	if err != nil {
		t.Fatalf("ApplyReplica: %v", err)
	}
	return applied
}

func pluginOwner(t *testing.T, pluginID string) domain.Owner {
	t.Helper()
	owner, err := domain.PluginOwner(pluginID)
	if err != nil {
		t.Fatalf("PluginOwner: %v", err)
	}
	return owner
}

func connByID(data *domain.VaultData, id string) *domain.Connection {
	for i := range data.Connections {
		if data.Connections[i].ID == id {
			return &data.Connections[i]
		}
	}
	return nil
}

// The ordinary case: something the user added on their other device shows up here, in the folder
// they put it in, with the password it needs.
func TestAnArrivingConnectionLandsInTheScopeWithItsSecret(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "new", FolderID: "sync", Host: "c", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u3", PassAuth: &domain.PasswordAuthConfig{VaultRef: "p3"}}}})
	doc.Passwords = map[string]domain.PasswordBlob{"p3": {Value: []byte("new secret")}}

	applied := apply(t, data, index, doc)

	arrived := connByID(data, "new")
	if arrived == nil || arrived.Host != "c" {
		t.Fatalf("the connection did not arrive: %+v", data.Connections)
	}
	if string(data.Passwords["p3"].Value) != "new secret" {
		t.Error("the password the connection needs did not arrive with it")
	}
	if !slices.Contains(applied.Added, "new") {
		t.Errorf("Added = %v, want the connection that arrived", applied.Added)
	}
}

// The attack the scope exists to stop. A hostile document names a folder the user never gave this
// plugin, and the connection must not be written there.
func TestAConnectionAimedOutsideTheScopeIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "smuggled", FolderID: "personal", Host: "evil", Port: 22})

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrOutsideScope) {
		t.Fatalf("err = %v, want ErrOutsideScope", err)
	}
	if connByID(data, "smuggled") != nil {
		t.Fatal("a connection was written outside the plugin's folder")
	}
}

// A folder the document invents must be inside the scope too, or the plugin gains a second place in
// the user's tree - one it can name whatever it likes, and drop things into.
func TestAFolderOutsideTheScopeIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving()
	doc.Folders = append(doc.Folders, domain.ConnectionFolder{ID: "toplevel", Name: "Important"})

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrOutsideScope) {
		t.Fatalf("err = %v, want ErrOutsideScope", err)
	}
	if len(data.Folders) != 2 {
		t.Fatalf("the folder tree grew to %d: %+v", len(data.Folders), data.Folders)
	}
}

// A folder nested under the scope root is the user's own arrangement and must be allowed through,
// or replication would flatten every tree they build inside the folder.
func TestAFolderNestedInsideTheScopeIsAccepted(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving()
	doc.Folders = append(doc.Folders, domain.ConnectionFolder{ID: "eu", Name: "EU", ParentID: "sync"})

	apply(t, data, index, doc)

	if !slices.ContainsFunc(data.Folders, func(f domain.ConnectionFolder) bool { return f.ID == "eu" }) {
		t.Fatal("a folder nested inside the scope did not arrive")
	}
}

// The subtle version of the same attack, and the dangerous one. The document is entirely in-scope,
// but it carries a password under a ref that an out-of-scope connection uses - so applying it would
// overwrite the credential of a machine this plugin was never given.
func TestASecretRefUsedOutsideTheScopeIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "bait", FolderID: "sync", Host: "c", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u", PassAuth: &domain.PasswordAuthConfig{VaultRef: "p2"}}}})
	doc.Passwords = map[string]domain.PasswordBlob{"p2": {Value: []byte("attacker")}}

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrSecretUsedOutsideScope) {
		t.Fatalf("err = %v, want ErrSecretUsedOutsideScope", err)
	}
	if string(data.Passwords["p2"].Value) != "out" {
		t.Fatal("an out-of-scope connection's password was overwritten")
	}
}

// The same attack through an identity rather than a password. A walk that only guarded passwords
// would let the key a jump host authenticates with be replaced.
func TestAnIdentityUsedOutsideTheScopeIsRefused(t *testing.T) {
	data, index := applyVault(t)
	data.Identities = map[string]domain.SSHIdentity{"k1": {ID: "k1", Comment: "mine"}}
	data.KeyBlobs = map[string]domain.IdentityBlob{"k1": {PEMData: []byte("real")}}
	data.Connections[1].Users[0].KeyAuth = &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}}

	doc := arriving(domain.Connection{ID: "bait", FolderID: "sync", Host: "c", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u", KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}}}}})
	doc.Identities = map[string]domain.SSHIdentity{"k1": {ID: "k1", Comment: "swapped"}}
	doc.KeyBlobs = map[string]domain.IdentityBlob{"k1": {PEMData: []byte("attacker")}}

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrSecretUsedOutsideScope) {
		t.Fatalf("err = %v, want ErrSecretUsedOutsideScope", err)
	}
	if string(data.KeyBlobs["k1"].PEMData) != "real" {
		t.Fatal("an out-of-scope connection's private key was overwritten")
	}
}

// A secret nothing in the arriving document references has no business being written. It is either
// noise or an attempt to plant a value under a ref something else will later use.
func TestASecretNothingReferencesIsNotWritten(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "new", FolderID: "sync", Host: "c", Port: 22})
	doc.Passwords = map[string]domain.PasswordBlob{"orphan": {Value: []byte("planted")}}

	apply(t, data, index, doc)

	if _, written := data.Passwords["orphan"]; written {
		t.Fatal("a secret nothing references was written into the vault")
	}
}

// A document that claims to be for a different plugin must never be applied here, whatever it
// contains: scope is what decides who may read these credentials.
func TestADocumentForAnotherScopeIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving()
	doc.Scope = "com.example.other"

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrScopeMismatch) {
		t.Fatalf("err = %v, want ErrScopeMismatch", err)
	}
}

// Nothing outside the scope may move, and the check that says so has to cover the objects the
// document never mentions - not just the ones it does.
func TestNothingOutsideTheScopeIsTouched(t *testing.T) {
	data, index := applyVault(t)
	before := *connByID(data, "outside")
	doc := arriving(domain.Connection{ID: "new", FolderID: "sync", Host: "c", Port: 22})

	apply(t, data, index, doc)

	after := connByID(data, "outside")
	if after == nil || after.Host != before.Host || after.FolderID != before.FolderID {
		t.Fatalf("the out-of-scope connection changed: %+v", after)
	}
	if string(data.Passwords["p2"].Value) != "out" {
		t.Fatal("the out-of-scope password changed")
	}
}

// A connection already here is updated in place rather than duplicated. Two rows with the same id
// would leave the user with a tree that cannot be reasoned about and a vault that cannot be saved.
func TestAnExistingConnectionIsUpdatedNotDuplicated(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "in-scope", FolderID: "sync", Host: "renamed", Port: 22})

	applied := apply(t, data, index, doc)

	count := 0
	for _, conn := range data.Connections {
		if conn.ID == "in-scope" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d connections share the id after applying", count)
	}
	if connByID(data, "in-scope").Host != "renamed" {
		t.Error("the update did not land")
	}
	if !slices.Contains(applied.Updated, "in-scope") {
		t.Errorf("Updated = %v, want the connection that changed", applied.Updated)
	}
}

// Deletion never travels (I9). A document that simply omits what this device has must leave it
// alone - a wiped server must not be able to empty a folder by serving nothing.
func TestApplyingNeverDeletes(t *testing.T) {
	data, index := applyVault(t)

	apply(t, data, index, arriving())

	if connByID(data, "in-scope") == nil {
		t.Fatal("a document that omitted the connection deleted it")
	}
	if _, held := data.Passwords["p1"]; !held {
		t.Fatal("a document that omitted the password deleted it")
	}
}

// A plugin-owned object has no business arriving over a replica: provisioning is a different port
// with a different threat model, and accepting one here would let a document create objects the
// core did not make.
func TestAPluginOwnedConnectionIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{
		ID: "provisioned", FolderID: "sync", Host: "c", Port: 22,
		Owner: pluginOwner(t, "com.example.sync"),
	})

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrProvisionedOverReplica) {
		t.Fatalf("err = %v, want ErrProvisionedOverReplica", err)
	}
	if connByID(data, "provisioned") != nil {
		t.Fatal("a plugin-owned connection was written into the vault")
	}
}

// Nothing at all is applied when any part of the document is refused. A partial apply would leave
// the vault in a state neither device has, and the version vector would then claim it was merged.
func TestARefusedDocumentAppliesNothingAtAll(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(
		domain.Connection{ID: "good", FolderID: "sync", Host: "c", Port: 22},
		domain.Connection{ID: "smuggled", FolderID: "personal", Host: "evil", Port: 22},
	)

	if _, err := domain.ApplyReplica(data, index, "com.example.sync", doc); err == nil {
		t.Fatal("a document with an out-of-scope connection was applied")
	}

	if connByID(data, "good") != nil {
		t.Fatal("the acceptable half of a refused document was applied")
	}
}

// A connection the document sends must still be a valid one. Applying a malformed connection would
// put the vault into a state the user could not have created through the UI, and the document is
// the one place such a connection can come from.
func TestAnInvalidConnectionIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "broken", FolderID: "sync", Host: "c", Port: 99999})

	if _, err := domain.ApplyReplica(data, index, "com.example.sync", doc); err == nil {
		t.Fatal("a connection with an impossible port was applied")
	}
	if connByID(data, "broken") != nil {
		t.Fatal("the invalid connection was written")
	}
}

// A document whose folders name each other as parents has no root at all. A walk that trusted the
// tree it was given would follow it forever; this is the one input where that tree is an attacker's
// to choose.
func TestAFolderCycleInTheDocumentIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving()
	doc.Folders = append(doc.Folders,
		domain.ConnectionFolder{ID: "a", Name: "A", ParentID: "b"},
		domain.ConnectionFolder{ID: "b", Name: "B", ParentID: "a"})

	if _, err := domain.ApplyReplica(data, index, "com.example.sync", doc); !errors.Is(err, domain.ErrInvalidScope) {
		t.Fatalf("err = %v, want ErrInvalidScope", err)
	}
	if len(data.Folders) != 2 {
		t.Fatalf("the folder tree grew to %d", len(data.Folders))
	}
}

// A vault that could not be read is an error rather than a silent no-op, so a failed sync is never
// reported as a successful one.
func TestApplyingToNoVaultIsRefused(t *testing.T) {
	_, index := applyVault(t)

	if _, err := domain.ApplyReplica(nil, index, "com.example.sync", arriving()); err == nil {
		t.Fatal("applying to a nil vault succeeded")
	}
}

// The ordinary re-sync: a connection already in the scope comes back with the password it already
// had. The out-of-scope check must not fire on the scope's own secrets, or every second sync of an
// unchanged connection would be refused as an attack.
func TestASecretSharedInsideTheScopeIsAccepted(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "in-scope", FolderID: "sync", Host: "a", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u1", PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"}}}})
	doc.Passwords = map[string]domain.PasswordBlob{"p1": {Value: []byte("rotated")}}

	apply(t, data, index, doc)

	if string(data.Passwords["p1"].Value) != "rotated" {
		t.Fatal("the scope's own password was not updated")
	}
}

// A jump hop authenticates too, so a document that reaches an out-of-scope credential through one
// must be refused exactly as if it had used it directly. A guard that walked only the users would
// let the connection arrive pointing at a password from outside the folder.
func TestAJumpHopReachingOutsideTheScopeIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "bait", FolderID: "sync", Host: "c", Port: 22,
		JumpChain: domain.JumpChainConfig{Hops: []domain.JumpHop{{
			ID: "j1", Host: "bastion", Port: 22,
			PassAuth: &domain.PasswordAuthConfig{VaultRef: "p2"},
		}}}})

	_, err := domain.ApplyReplica(data, index, "com.example.sync", doc)

	if !errors.Is(err, domain.ErrSecretUsedOutsideScope) {
		t.Fatalf("err = %v, want ErrSecretUsedOutsideScope for a jump hop's credential", err)
	}
}

// Applying twice is the normal case - every unlock re-reads the same remote - and must not grow the
// folder tree. Duplicate folder ids would leave a tree the user cannot reason about.
func TestApplyingTheSameDocumentTwiceChangesNothingTheSecondTime(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving(domain.Connection{ID: "new", FolderID: "sync", Host: "c", Port: 22})

	apply(t, data, index, doc)
	folders, conns := len(data.Folders), len(data.Connections)
	apply(t, data, index, doc)

	if len(data.Folders) != folders {
		t.Errorf("folders grew from %d to %d on a second apply", folders, len(data.Folders))
	}
	if len(data.Connections) != conns {
		t.Errorf("connections grew from %d to %d on a second apply", conns, len(data.Connections))
	}
}

// A caller that does not name a plugin has no scope to check against, so there is nothing that
// makes the document safe to apply. Composition can leave the id empty, and the failure has to be a
// refusal rather than an apply against an empty scope.
func TestApplyingWithNoPluginNamedIsRefused(t *testing.T) {
	data, index := applyVault(t)
	doc := arriving()
	doc.Scope = ""

	_, err := domain.ApplyReplica(data, index, "", doc)

	if !errors.Is(err, domain.ErrScopeMismatch) {
		t.Fatalf("err = %v, want ErrScopeMismatch", err)
	}
}
