package domain

import (
	"errors"
	"testing"
)

const adoptTeam = "team-vault"

func adoptVault() *VaultData {
	return &VaultData{
		Identities: map[string]SSHIdentity{
			"my-key":   {ID: "my-key"},
			"team-key": {ID: "team-key", VaultID: adoptTeam},
		},
		Passwords: map[string]PasswordBlob{
			"my-pw":   {},
			"team-pw": {VaultID: adoptTeam},
		},
	}
}

func passwordUser(ref string) ConnectionUser {
	return ConnectionUser{ID: "u1", Username: "deploy", Auth: AuthMethodPassword, PassAuth: &PasswordAuthConfig{VaultRef: ref}}
}

func adoptConn(users ...ConnectionUser) *Connection {
	return &Connection{ID: "c1", VaultID: adoptTeam, Host: "db.example", Port: 22, Users: users}
}

// The reported failure: a member types their own password into a team user that was moved in with
// no credential. The shared record must not carry the id, and the member must still connect with it.
func TestAdoptBindsAPasswordTheMemberJustTyped(t *testing.T) {
	d := adoptVault()
	stored := adoptConn(ConnectionUser{ID: "u1", Username: "deploy", Auth: AuthMethodKey})
	c := adoptConn(passwordUser("my-pw"))

	AdoptPersonalCredentials(d, c, stored)

	u := c.Users[0]
	if u.CredentialSource != CredentialSourcePersonal || u.PassAuth != nil || u.KeyAuth != nil {
		t.Fatalf("shared user = %+v, want personal-source with no reference; a personal id must not reach the team", u)
	}
	got, err := ResolveCredentials(d, c, u)
	if err != nil || got.PassAuth == nil || got.PassAuth.VaultRef != "my-pw" {
		t.Fatalf("ResolveCredentials = %+v, %v; want this member's own password through the binding", got, err)
	}
}

// A reference already on file may have been planted by another member who also repointed the host.
// Saving an unrelated field must not turn it into this member's binding.
func TestAdoptLeavesAReferenceAlreadyOnFile(t *testing.T) {
	d := adoptVault()
	stored := adoptConn(passwordUser("my-pw"))
	c := adoptConn(passwordUser("my-pw"))
	c.Name = "renamed"

	AdoptPersonalCredentials(d, c, stored)

	if c.Users[0].CredentialSource != "" || d.Sync != nil {
		t.Fatalf("user = %+v, bindings = %+v; a reference this save did not introduce must not be adopted", c.Users[0], d.Sync)
	}
	if _, err := ResolveCredentials(d, c, c.Users[0]); !errors.Is(err, ErrCredentialOutsideVault) {
		t.Errorf("ResolveCredentials error = %v, want ErrCredentialOutsideVault; the planted id stays refused", err)
	}
}

func TestAdoptKeepsTheTeamsOwnCredential(t *testing.T) {
	d := adoptVault()
	c := adoptConn(passwordUser("team-pw"))

	AdoptPersonalCredentials(d, c, adoptConn(ConnectionUser{ID: "u1", Username: "deploy", Auth: AuthMethodKey}))

	if c.Users[0].CredentialSource != "" || c.Users[0].PassAuth == nil || c.Users[0].PassAuth.VaultRef != "team-pw" {
		t.Errorf("user = %+v; a password kept in the team vault is named inline, as the team shares it", c.Users[0])
	}
}

func TestAdoptLeavesAPersonalConnectionAlone(t *testing.T) {
	d := adoptVault()
	c := adoptConn(passwordUser("my-pw"))
	c.VaultID = ""

	AdoptPersonalCredentials(d, c, nil)

	if c.Users[0].CredentialSource != "" || c.Users[0].PassAuth == nil || d.Sync != nil {
		t.Errorf("user = %+v; only this account writes a personal connection, so it keeps its reference", c.Users[0])
	}
}

// A connection created straight in a team vault has no stored record: every reference is new.
func TestAdoptBindsTheFirstNewPersonalKeyOfANewConnection(t *testing.T) {
	d := adoptVault()
	c := adoptConn(ConnectionUser{ID: "u1", Username: "deploy", Auth: AuthMethodKey,
		KeyAuth: &KeyAuthConfig{IdentityIDs: []string{"team-key", "my-key"}}})

	AdoptPersonalCredentials(d, c, nil)

	if c.Users[0].CredentialSource != CredentialSourcePersonal {
		t.Fatalf("user = %+v, want personal-source", c.Users[0])
	}
	if d.Sync == nil || len(d.Sync.Bindings) != 1 || d.Sync.Bindings[0].IdentityID != "my-key" {
		t.Errorf("bindings = %+v, want one binding of my-key; the team key is not this member's to bind", d.Sync)
	}
}

func TestAdoptReplacesAnEarlierBindingOfTheUser(t *testing.T) {
	d := adoptVault()
	old := CredentialBinding{VaultID: adoptTeam, ConnectionID: "c1", UserID: "u1", IdentityID: "my-key", SuspendedBy: "ab"}
	d.Sync = &VaultSync{Bindings: []CredentialBinding{old}}
	c := adoptConn(passwordUser("my-pw"))

	AdoptPersonalCredentials(d, c, adoptConn(ConnectionUser{ID: "u1", Username: "deploy", Auth: AuthMethodKey}))

	if len(d.Sync.Bindings) != 1 {
		t.Fatalf("bindings = %+v, want exactly one per user", d.Sync.Bindings)
	}
	if b := d.Sync.Bindings[0]; b.PasswordID != "my-pw" || b.IdentityID != "" || b.SuspendedBy != "" {
		t.Errorf("binding = %+v, want the new password in force; the member chose it after seeing the host", b)
	}
}

func TestAdoptSkipsAPersonalSourceUser(t *testing.T) {
	d := adoptVault()
	u := passwordUser("my-pw")
	u.CredentialSource = CredentialSourcePersonal
	c := adoptConn(u)

	AdoptPersonalCredentials(d, c, nil)

	if d.Sync != nil {
		t.Errorf("bindings = %+v; a personal-source user is linked by the bindings panel, not here", d.Sync)
	}
}
