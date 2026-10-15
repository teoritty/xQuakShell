package domain

import (
	"errors"
	"fmt"
)

// CredentialSourcePersonal marks a team connection user whose credential each member supplies from
// their own personal vault through a binding (spec §6.7): the host is shared, the credential is not.
const CredentialSourcePersonal = "personal"

var (
	// ErrBindingMissing is a personal-source user this member has not bound to a credential of theirs.
	ErrBindingMissing = errors.New("no credential of yours is bound to this connection")
	// ErrBindingSuspended is a binding another member's change of the connection's address suspended
	// until this member confirms the new address.
	ErrBindingSuspended = errors.New("the connection's address was changed by another member; confirm it before connecting")
)

// CredentialBinding ties one user of a team connection to an identity or password in this member's
// personal vault. It lives in the personal vault and syncs to this member's devices only.
type CredentialBinding struct {
	VaultID      string `json:"vaultId"`
	ConnectionID string `json:"connectionId"`
	UserID       string `json:"userId"`
	IdentityID   string `json:"identityId,omitempty"`
	PasswordID   string `json:"passwordId,omitempty"`
	// SuspendedBy is the account, in hex, whose change of the connection's host, port or jump chain
	// suspended the binding; empty while it is in force.
	SuspendedBy string `json:"suspendedBy,omitempty"`
}

// Key identifies the binding of one user of one connection of one vault.
func (b CredentialBinding) Key() string { return b.VaultID + "/" + b.ConnectionID + "/" + b.UserID }

// ResolveCredentials returns user with the credential it authenticates with: as it is for an inline
// credential, or with this member's bound identity or password for a personal source. A binding
// must name a credential of the personal vault - a team key would defeat the point of binding - and
// a suspended one is refused, so a host another member repointed never receives the credential.
//
// The binding, not the shared user, decides whether that is a key or a password. The shared user's
// method is whatever the member who moved the connection in happened to use; matching against it
// left a member who bound a key to a password user - the only kind of credential the bindings panel
// offered - with ErrBindingMissing for a binding that was plainly there. Any inline reference the
// shared record still carries is replaced, so a value written there by another member is never used.
func ResolveCredentials(d *VaultData, conn *Connection, user ConnectionUser) (ConnectionUser, error) {
	if user.CredentialSource != CredentialSourcePersonal {
		return user, nil
	}
	key := CredentialBinding{VaultID: conn.VaultID, ConnectionID: conn.ID, UserID: user.ID}.Key()
	var b *CredentialBinding
	if d.Sync != nil {
		for i := range d.Sync.Bindings {
			if d.Sync.Bindings[i].Key() == key {
				b = &d.Sync.Bindings[i]
			}
		}
	}
	if b == nil {
		return user, ErrBindingMissing
	}
	if b.SuspendedBy != "" {
		return user, ErrBindingSuspended
	}
	if user.Auth != AuthMethodKey && user.Auth != AuthMethodPassword {
		return user, fmt.Errorf("a personal credential source with %q: %w", user.Auth, ErrInvalidConnectionConfig)
	}
	user.KeyAuth, user.PassAuth = nil, nil
	switch {
	case b.IdentityID != "":
		if ident, ok := d.Identities[b.IdentityID]; !ok || ident.VaultID != "" {
			return user, ErrBindingMissing
		}
		user.Auth, user.KeyAuth = AuthMethodKey, &KeyAuthConfig{IdentityIDs: []string{b.IdentityID}}
	case b.PasswordID != "":
		if pw, ok := d.Passwords[b.PasswordID]; !ok || pw.VaultID != "" {
			return user, ErrBindingMissing
		}
		user.Auth, user.PassAuth = AuthMethodPassword, &PasswordAuthConfig{VaultRef: b.PasswordID}
	default:
		return user, ErrBindingMissing
	}
	return user, nil
}
