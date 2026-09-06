package domain

import (
	"errors"
	"fmt"
)

// ErrNonExportableInScope indicates the scope holds a connection that uses a key the user was
// promised could not leave the vault.
var ErrNonExportableInScope = errors.New("scope contains a non-exportable key")

// ReplicaDocument is everything that leaves the machine for one plugin's scope (ADR-022, A).
//
// It is the closure of the scope rather than the vault with the out-of-scope parts removed: a
// connection arrives useless without the password or key it references, so what it references comes
// with it, and nothing that is merely present in the vault does.
//
// The transport never sees this. It is serialized and sealed before a plugin is handed anything, so
// what a plugin moves is opaque bytes and what a hostile server can read is nothing (T1).
type ReplicaDocument struct {
	// Scope names the plugin whose folder this is, so a document cannot be applied to the wrong one.
	Scope string `json:"scope"`
	// Version is how much of each device's history this document has seen, and is what decides
	// whether a payload is newer, older or divergent (T2).
	Version VersionVector `json:"version"`

	Folders     []ConnectionFolder      `json:"folders,omitempty"`
	Connections []Connection            `json:"connections,omitempty"`
	Passwords   map[string]PasswordBlob `json:"passwords,omitempty"`
	Identities  map[string]SSHIdentity  `json:"identities,omitempty"`
	KeyBlobs    map[string]IdentityBlob `json:"keyBlobs,omitempty"`
}

// BuildReplicaDocument gathers the closure of one plugin's scope.
//
// A non-exportable key anywhere in that closure refuses the whole document rather than being
// dropped from it. Dropping would be worse than refusing: the connection would arrive on the other
// device referencing a key that is not there, and the user would be told nothing.
func BuildReplicaDocument(
	data *VaultData,
	index ScopeIndex,
	pluginID string,
	version VersionVector,
) (ReplicaDocument, error) {
	if data == nil {
		// Sending an empty document would look to the other device exactly like the user having
		// deleted everything, which is the one thing an unreadable vault must not say.
		return ReplicaDocument{}, fmt.Errorf("build replica for %s: no vault data", pluginID)
	}
	doc := ReplicaDocument{
		Scope:      pluginID,
		Version:    version,
		Passwords:  map[string]PasswordBlob{},
		Identities: map[string]SSHIdentity{},
		KeyBlobs:   map[string]IdentityBlob{},
	}
	for _, folder := range data.Folders {
		if index.Contains(pluginID, folder.ID) {
			doc.Folders = append(doc.Folders, folder)
		}
	}
	for _, conn := range data.Connections {
		if !index.Contains(pluginID, conn.FolderID) {
			continue
		}
		doc.Connections = append(doc.Connections, conn)
		if err := doc.gatherClosure(data, conn); err != nil {
			return ReplicaDocument{}, err
		}
	}
	return doc, nil
}

// gatherClosure pulls in what one connection needs to be usable on the other side: the secrets its
// users reference, and the ones its jump hops do. A walk that looked only at the users would send a
// connection whose first hop cannot be made.
func (d *ReplicaDocument) gatherClosure(data *VaultData, conn Connection) error {
	for _, user := range conn.Users {
		if err := d.gatherAuth(data, user.KeyAuth, user.PassAuth); err != nil {
			return err
		}
	}
	for i := range conn.JumpChain.Hops {
		hop := &conn.JumpChain.Hops[i]
		if err := d.gatherAuth(data, hop.KeyAuth, hop.PassAuth); err != nil {
			return err
		}
	}
	return nil
}

func (d *ReplicaDocument) gatherAuth(data *VaultData, key *KeyAuthConfig, pass *PasswordAuthConfig) error {
	if pass != nil && pass.VaultRef != "" {
		if blob, stored := data.Passwords[pass.VaultRef]; stored {
			d.Passwords[pass.VaultRef] = blob
		}
	}
	if key == nil {
		return nil
	}
	for _, id := range key.IdentityIDs {
		identity, stored := data.Identities[id]
		if !stored {
			continue
		}
		if identity.NonExportable {
			return fmt.Errorf("%w: key %s", ErrNonExportableInScope, id)
		}
		d.Identities[id] = identity
		if blob, held := data.KeyBlobs[id]; held {
			d.KeyBlobs[id] = blob
		}
	}
	return nil
}
