package synctrust

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// ErrForeignVault is a change that would overwrite or delete an item another synced vault holds: a
// personal record taking over a team connection, or one team vault's record reaching into another.
var ErrForeignVault = errors.New("record names an item of another vault")

// Scope is the synced vault a set of records belongs to. The zero Scope is the personal vault;
// otherwise Vault is the team vault's id as the local items carry it.
type Scope struct {
	Vault string
}

// teamTypes are the record types a team vault may hold (spec §4.2). Host keys, peer trust,
// settings and plugin installs stay personal: a member must not plant them for the others.
// TypePluginSecret stays listed only so that a team vault an older build filled with them still
// verifies: ApplyScope drops those records rather than refusing the commit that carries them.
var teamTypes = []RecordType{TypeFolder, TypeConnection, TypeIdentity, TypePassword, TypePluginSecret, TypeConflict}

// Allows reports whether records of type t may live in this scope's vault.
func (s Scope) Allows(t RecordType) bool {
	if s.Vault == "" {
		return true
	}
	return slices.Contains(teamTypes, t)
}

// SnapshotScope lists the synced items of one vault. Items of other vaults are left out, and the
// vault id is cleared from what is listed: the envelope says which vault a record is in, and data
// that repeated it could disagree with it.
func SnapshotScope(namingKey []byte, d *domain.VaultData, s Scope) (map[vaultsync.ID]Entry, error) {
	return snapshotAll(namingKey, project(d, s))
}

// ApplyScope applies changes to one vault's items and stamps every upserted item with that vault.
// A change of a type the vault may not hold, or one that names an item of another vault, refuses
// the whole set, as Apply refuses any invalid change.
func ApplyScope(d *domain.VaultData, s Scope, changes []Entry) error {
	if s.Vault != "" {
		changes = withoutPluginSecrets(changes)
	}
	for _, c := range changes {
		if !s.Allows(c.Type) {
			return fmt.Errorf("%w: %s in a team vault", ErrRecordType, c.Type)
		}
		if owner, ok := ownerOf(d, c.Type, c.Key); ok && owner != s.Vault {
			return fmt.Errorf("%w: %s %q", ErrForeignVault, c.Type, c.Key)
		}
	}
	if err := apply(d, changes); err != nil {
		return err
	}
	for _, c := range changes {
		if c.Data != nil {
			stamp(d, c.Type, c.Key, s.Vault)
		}
	}
	return nil
}

// withoutPluginSecrets drops the plugin secrets a team commit carries. Only a build that shared a
// moved connection's plugin password with the whole team writes them. Upserting one would plant
// another member's credential in this account; deleting one would remove this account's own value
// filed under the same reference - on the owner's second device, the very secret being moved.
func withoutPluginSecrets(changes []Entry) []Entry {
	return slices.DeleteFunc(slices.Clone(changes), func(c Entry) bool { return c.Type == TypePluginSecret })
}

// project copies the part of d one vault syncs, with vault ids cleared. Personal-only data - host
// keys, peer trust, settings - belongs to the personal scope alone. So does every plugin secret,
// including those of team connections: the host is shared, the credential is not (spec §6.7), and
// each member keeps their own value under the reference the shared connection names.
func project(d *domain.VaultData, s Scope) *domain.VaultData {
	out := &domain.VaultData{Version: d.Version, Identities: map[string]domain.SSHIdentity{}, KeyBlobs: map[string]domain.IdentityBlob{},
		Passwords: map[string]domain.PasswordBlob{}, PluginSecrets: map[string][]byte{}}
	if s.Vault == "" {
		out.KnownHosts, out.PeerTrust, out.Settings = d.KnownHosts, d.PeerTrust, d.Settings
	}
	for _, f := range d.Folders {
		if f.VaultID == s.Vault {
			f.VaultID = ""
			out.Folders = append(out.Folders, f)
		}
	}
	for _, c := range d.Connections {
		if c.VaultID == s.Vault {
			c.VaultID = ""
			out.Connections = append(out.Connections, c)
		}
	}
	projectMaps(d, out, s)
	if d.Sync != nil {
		out.Sync = &domain.VaultSync{}
		if s.Vault == "" {
			out.Sync.Contacts, out.Sync.Bindings, out.Sync.Teams = d.Sync.Contacts, d.Sync.Bindings, d.Sync.Teams
			out.Sync.Plugins, out.Sync.PluginFiles = d.Sync.Plugins, d.Sync.PluginFiles
		}
		for _, c := range d.Sync.Conflicts {
			if c.VaultID == s.Vault {
				c.VaultID = ""
				out.Sync.Conflicts = append(out.Sync.Conflicts, c)
			}
		}
	}
	return out
}

func projectMaps(d, out *domain.VaultData, s Scope) {
	for id, ident := range d.Identities {
		if ident.VaultID == s.Vault {
			ident.VaultID = ""
			out.Identities[id] = ident
			if b, ok := d.KeyBlobs[id]; ok {
				out.KeyBlobs[id] = b
			}
		}
	}
	for id, p := range d.Passwords {
		if p.VaultID == s.Vault {
			p.VaultID = ""
			out.Passwords[id] = p
		}
	}
	if s.Vault == "" {
		maps.Copy(out.PluginSecrets, d.PluginSecrets)
	}
}

// ownerOf reports which vault holds the item a record names, if the local vault holds it at all.
func ownerOf(d *domain.VaultData, t RecordType, key string) (string, bool) {
	switch t {
	case TypeFolder:
		if i := slices.IndexFunc(d.Folders, func(f domain.ConnectionFolder) bool { return f.ID == key }); i >= 0 {
			return d.Folders[i].VaultID, true
		}
	case TypeConnection:
		if i := slices.IndexFunc(d.Connections, func(c domain.Connection) bool { return c.ID == key }); i >= 0 {
			return d.Connections[i].VaultID, true
		}
	case TypeIdentity:
		if v, ok := d.Identities[key]; ok {
			return v.VaultID, true
		}
	case TypePassword:
		if v, ok := d.Passwords[key]; ok {
			return v.VaultID, true
		}
	}
	return "", false
}

// stamp records which vault an upserted item belongs to. A plugin secret carries no vault of its
// own; it is always personal.
func stamp(d *domain.VaultData, t RecordType, key, vault string) {
	switch t {
	case TypeFolder:
		if i := slices.IndexFunc(d.Folders, func(f domain.ConnectionFolder) bool { return f.ID == key }); i >= 0 {
			d.Folders[i].VaultID = vault
		}
	case TypeConnection:
		if i := slices.IndexFunc(d.Connections, func(c domain.Connection) bool { return c.ID == key }); i >= 0 {
			d.Connections[i].VaultID = vault
		}
	case TypeIdentity:
		v := d.Identities[key]
		v.VaultID = vault
		d.Identities[key] = v
	case TypePassword:
		v := d.Passwords[key]
		v.VaultID = vault
		d.Passwords[key] = v
	case TypeConflict:
		if d.Sync != nil {
			if i := slices.IndexFunc(d.Sync.Conflicts, func(c domain.SyncConflict) bool { return c.ID == key }); i >= 0 {
				d.Sync.Conflicts[i].VaultID = vault
			}
		}
	}
}
