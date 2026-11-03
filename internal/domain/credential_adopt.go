package domain

import "slices"

// AdoptPersonalCredentials turns a key or password that this save of a team connection names from
// the personal vault into this member's binding, and marks the user as one each member binds.
//
// The editor writes whatever credential the member picks into the user it edits. For a user that is
// already personal-source the editor links it instead, but a user that was moved into the team with
// no credential at all, a user added to a team connection, and every user of a connection created
// straight in a team vault reach here as inline users. Stored as they came, the id of the member's
// personal password lands in the shared record: every other member's device refuses it as a
// credential from outside the team, and the shared user no longer names the credential anyone else
// relied on. One member typing their own password broke the connection for the whole team.
//
// Only a reference this save introduced is adopted - one the stored record's user did not already
// name. A reference already on file may have been written by another member, who can also repoint the
// host; adopting it would bind this member's password to that member's server the next time this
// member saved an unrelated field. Such a reference is left for CheckCredentialScope to refuse at
// connect time, as before. stored is the record on file before this save, or nil for a new one.
//
// A personal connection, or a credential kept in the team's own vault, is not touched: both are
// legitimately named inline.
func AdoptPersonalCredentials(d *VaultData, c, stored *Connection) {
	if d == nil || c == nil || c.VaultID == "" || c.ID == "" {
		return
	}
	for i := range c.Users {
		u := &c.Users[i]
		if u.CredentialSource == CredentialSourcePersonal {
			continue
		}
		b, ok := introducedPersonalCredential(d, *u, storedUser(stored, u.ID))
		if !ok {
			continue
		}
		b.VaultID, b.ConnectionID, b.UserID = c.VaultID, c.ID, u.ID
		putBinding(d, b)
		u.KeyAuth, u.PassAuth, u.CredentialSource = nil, nil, CredentialSourcePersonal
	}
}

// introducedPersonalCredential returns a binding naming the personal-vault credential of u's active
// method that before did not already reference. A key list mixing team and personal keys yields its
// first new personal key: a member who adds a key of their own to a user has made it one each member
// binds, and the team keys are then bound by the members who use them.
func introducedPersonalCredential(d *VaultData, u ConnectionUser, before *ConnectionUser) (CredentialBinding, bool) {
	switch u.Auth {
	case AuthMethodKey:
		if u.KeyAuth == nil {
			return CredentialBinding{}, false
		}
		for _, id := range u.KeyAuth.IdentityIDs {
			ident, ok := d.Identities[id]
			if ok && ident.VaultID == "" && !namesIdentity(before, id) {
				return CredentialBinding{IdentityID: id}, true
			}
		}
	case AuthMethodPassword:
		if u.PassAuth == nil || u.PassAuth.VaultRef == "" {
			return CredentialBinding{}, false
		}
		pw, ok := d.Passwords[u.PassAuth.VaultRef]
		if ok && pw.VaultID == "" && !namesPassword(before, u.PassAuth.VaultRef) {
			return CredentialBinding{PasswordID: u.PassAuth.VaultRef}, true
		}
	}
	return CredentialBinding{}, false
}

func storedUser(stored *Connection, id string) *ConnectionUser {
	if stored == nil {
		return nil
	}
	for i := range stored.Users {
		if stored.Users[i].ID == id {
			return &stored.Users[i]
		}
	}
	return nil
}

func namesIdentity(u *ConnectionUser, id string) bool {
	return u != nil && u.KeyAuth != nil && slices.Contains(u.KeyAuth.IdentityIDs, id)
}

func namesPassword(u *ConnectionUser, id string) bool {
	return u != nil && u.PassAuth != nil && u.PassAuth.VaultRef == id
}

// putBinding stores b in force, replacing an earlier binding of the same user, as a binding the
// member set from the bindings panel would.
func putBinding(d *VaultData, b CredentialBinding) {
	if d.Sync == nil {
		d.Sync = &VaultSync{}
	}
	b.SuspendedBy = ""
	d.Sync.Bindings = slices.DeleteFunc(d.Sync.Bindings, func(x CredentialBinding) bool { return x.Key() == b.Key() })
	d.Sync.Bindings = append(d.Sync.Bindings, b)
}
