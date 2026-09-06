package domain

import (
	"errors"
	"fmt"
	"slices"
)

var (
	// ErrOutsideScope indicates an arriving document that names a folder outside the plugin's own.
	//
	// This is the attack the scope exists to stop. Everything the user put in the folder is theirs
	// to synchronise; everything beside it is not, and a document is the one place a plugin gets to
	// name a folder it was never given.
	ErrOutsideScope = errors.New("replica names a folder outside the plugin's scope")

	// ErrSecretUsedOutsideScope indicates an arriving secret stored under a reference that
	// something outside the scope also uses.
	//
	// Subtler than ErrOutsideScope and worse: every object in such a document is legitimately in
	// scope, and applying it would still replace the credential of a machine this plugin was never
	// given. Secrets are keyed by reference, and a reference is the one thing a document can choose
	// freely.
	ErrSecretUsedOutsideScope = errors.New("replica writes a secret used outside the plugin's scope")

	// ErrProvisionedOverReplica indicates an arriving object claiming to be owned by a plugin.
	//
	// Replication carries what the user put in the folder, and the user is the core. Objects a
	// plugin owns come from provisioning, which is a different port with a different threat model
	// and its own consent - accepting one here would route around all of it.
	ErrProvisionedOverReplica = errors.New("replica carries a plugin-owned object")
)

// ReplicaApplied reports what an apply changed, so the user can be told rather than surprised.
type ReplicaApplied struct {
	Added   []string
	Updated []string
}

// ApplyReplica writes an arriving document into the vault, touching only what is inside the
// plugin's scope (ADR-022).
//
// It is the last line of the boundary and treats the document as hostile: the transport is a
// plugin, the server behind it is someone else's, and neither had to be honest to get this far.
// Every refusal below is a way a document could otherwise reach something the user never put in the
// folder.
//
// Nothing is written until everything has been checked. A partial apply would leave the vault in a
// state neither device has, and the version vector would then record it as merged.
func ApplyReplica(data *VaultData, index ScopeIndex, pluginID string, doc ReplicaDocument) (ReplicaApplied, error) {
	if data == nil {
		return ReplicaApplied{}, errors.New("apply replica: no vault data")
	}
	// The caller names the plugin it is synchronising; the document names the scope it was built
	// for. They have to agree, because scope is what decides who may read these credentials and a
	// document is free to claim any scope it likes.
	if pluginID == "" || doc.Scope != pluginID {
		return ReplicaApplied{}, fmt.Errorf("apply replica for %s: document is for %q: %w",
			pluginID, doc.Scope, ErrScopeMismatch)
	}
	// The folders the document brings are part of the walk that decides whether they are in scope:
	// one nested inside the scope root does not exist here yet, and against the index as it stands
	// would resolve to nothing and read as out of scope. WithFolders refuses a cycle, so a document
	// naming two folders as each other's parent is rejected rather than walked.
	walk, err := index.WithFolders(doc.Folders)
	if err != nil {
		return ReplicaApplied{}, fmt.Errorf("apply replica for %s: %w", pluginID, err)
	}
	if err := checkArrivingShape(walk, doc); err != nil {
		return ReplicaApplied{}, err
	}
	refs := referencedByDocument(doc)
	if err := checkSecretsStayInScope(data, index, doc, refs); err != nil {
		return ReplicaApplied{}, err
	}
	return writeReplica(data, doc, refs), nil
}

// checkArrivingShape refuses everything that must never be written, before anything is.
func checkArrivingShape(index ScopeIndex, doc ReplicaDocument) error {
	for _, folder := range doc.Folders {
		if !index.Contains(doc.Scope, folder.ID) {
			return fmt.Errorf("apply replica: folder %s: %w", folder.ID, ErrOutsideScope)
		}
	}
	for i := range doc.Connections {
		conn := &doc.Connections[i]
		if !index.Contains(doc.Scope, conn.FolderID) {
			return fmt.Errorf("apply replica: connection %s: %w", conn.ID, ErrOutsideScope)
		}
		if _, byPlugin := conn.Owner.PluginID(); byPlugin {
			return fmt.Errorf("apply replica: connection %s: %w", conn.ID, ErrProvisionedOverReplica)
		}
		if err := conn.Validate(); err != nil {
			return fmt.Errorf("apply replica: connection %s: %w", conn.ID, err)
		}
	}
	return nil
}

// documentRefs is what the arriving connections actually authenticate with. It is recomputed from
// the connections rather than taken from the document's own secret maps, because those maps are
// exactly what a hostile document would pad.
type documentRefs struct {
	passwords  map[string]bool
	identities map[string]bool
}

func referencedByDocument(doc ReplicaDocument) documentRefs {
	refs := documentRefs{passwords: map[string]bool{}, identities: map[string]bool{}}
	for i := range doc.Connections {
		collectConnectionRefs(&doc.Connections[i], &refs)
	}
	return refs
}

func collectConnectionRefs(conn *Connection, refs *documentRefs) {
	for _, user := range conn.Users {
		refs.collect(user.KeyAuth, user.PassAuth)
	}
	for i := range conn.JumpChain.Hops {
		hop := &conn.JumpChain.Hops[i]
		refs.collect(hop.KeyAuth, hop.PassAuth)
	}
}

func (r *documentRefs) collect(key *KeyAuthConfig, pass *PasswordAuthConfig) {
	if pass != nil && pass.VaultRef != "" {
		r.passwords[pass.VaultRef] = true
	}
	if key == nil {
		return
	}
	for _, id := range key.IdentityIDs {
		r.identities[id] = true
	}
}

// checkSecretsStayInScope refuses a document that would write a secret something outside the scope
// also authenticates with.
//
// The check is against the vault as it stands, not against the document: what matters is whether
// some connection the user keeps elsewhere depends on this reference today.
func checkSecretsStayInScope(data *VaultData, index ScopeIndex, doc ReplicaDocument, refs documentRefs) error {
	for i := range data.Connections {
		conn := &data.Connections[i]
		if index.Contains(doc.Scope, conn.FolderID) {
			continue
		}
		var outside documentRefs
		outside.passwords, outside.identities = map[string]bool{}, map[string]bool{}
		collectConnectionRefs(conn, &outside)
		for ref := range outside.passwords {
			if refs.passwords[ref] {
				return fmt.Errorf("apply replica: password %s is used by %s: %w",
					ref, conn.ID, ErrSecretUsedOutsideScope)
			}
		}
		for id := range outside.identities {
			if refs.identities[id] {
				return fmt.Errorf("apply replica: identity %s is used by %s: %w",
					id, conn.ID, ErrSecretUsedOutsideScope)
			}
		}
	}
	return nil
}

// writeReplica puts the checked document into the vault. Nothing is deleted here: an object this
// device has and the document does not is reported to the user by the merge, never removed (I9).
func writeReplica(data *VaultData, doc ReplicaDocument, refs documentRefs) ReplicaApplied {
	var applied ReplicaApplied
	for _, folder := range doc.Folders {
		upsertFolder(data, folder)
	}
	for _, conn := range doc.Connections {
		if upsertConnection(data, conn) {
			applied.Updated = append(applied.Updated, conn.ID)
			continue
		}
		applied.Added = append(applied.Added, conn.ID)
	}
	writeSecrets(data, doc, refs)
	slices.Sort(applied.Added)
	slices.Sort(applied.Updated)
	return applied
}

// writeSecrets stores only what the arriving connections actually authenticate with. Anything else
// in the document's secret maps is padding, and a reference is the one thing a document chooses
// freely - so writing an unreferenced one is how a value gets planted under a key something else
// will later use.
func writeSecrets(data *VaultData, doc ReplicaDocument, refs documentRefs) {
	for ref, blob := range doc.Passwords {
		if refs.passwords[ref] {
			ensureMap(&data.Passwords)[ref] = blob
		}
	}
	for id, identity := range doc.Identities {
		if !refs.identities[id] {
			continue
		}
		ensureMap(&data.Identities)[id] = identity
		if blob, carried := doc.KeyBlobs[id]; carried {
			ensureMap(&data.KeyBlobs)[id] = blob
		}
	}
}

func upsertFolder(data *VaultData, folder ConnectionFolder) {
	for i := range data.Folders {
		if data.Folders[i].ID == folder.ID {
			data.Folders[i] = folder
			return
		}
	}
	data.Folders = append(data.Folders, folder)
}

// upsertConnection writes one connection and reports whether it replaced an existing one.
func upsertConnection(data *VaultData, conn Connection) bool {
	for i := range data.Connections {
		if data.Connections[i].ID == conn.ID {
			data.Connections[i] = conn
			return true
		}
	}
	data.Connections = append(data.Connections, conn)
	return false
}

// ensureMap makes a nil vault map writable. A vault read from an older file, or one built by hand
// in a test, can have any of them nil, and writing to a nil map panics.
func ensureMap[K comparable, V any](m *map[K]V) map[K]V {
	if *m == nil {
		*m = make(map[K]V)
	}
	return *m
}
