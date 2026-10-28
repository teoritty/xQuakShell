package domain

import (
	"errors"
	"testing"
)

func teamConn() (*Connection, ConnectionUser) {
	u := ConnectionUser{ID: "u1", Username: "deploy", Auth: AuthMethodKey, CredentialSource: CredentialSourcePersonal}
	return &Connection{ID: "c1", Name: "db", Host: "db.example", Port: 22, VaultID: "t", Users: []ConnectionUser{u}, DefaultUserID: "u1"}, u
}

func TestResolveCredentialsUsesThePersonalBinding(t *testing.T) {
	conn, user := teamConn()
	d := NewVaultData()
	d.Identities["mine"] = SSHIdentity{ID: "mine"}
	d.Sync = &VaultSync{Bindings: []CredentialBinding{{VaultID: "t", ConnectionID: "c1", UserID: "u1", IdentityID: "mine"}}}
	got, err := ResolveCredentials(d, conn, user)
	if err != nil || got.KeyAuth == nil || len(got.KeyAuth.IdentityIDs) != 1 || got.KeyAuth.IdentityIDs[0] != "mine" {
		t.Fatalf("resolved = %+v, %v; a personal credential source takes the member's own key", got, err)
	}
	user.Auth = AuthMethodPassword
	d.Passwords["pw"] = PasswordBlob{Label: "pw"}
	d.Sync.Bindings[0] = CredentialBinding{VaultID: "t", ConnectionID: "c1", UserID: "u1", PasswordID: "pw"}
	if got, err := ResolveCredentials(d, conn, user); err != nil || got.PassAuth == nil || got.PassAuth.VaultRef != "pw" {
		t.Fatalf("resolved = %+v, %v", got, err)
	}
}

func TestResolveCredentialsFollowsTheBindingNotTheSharedMethod(t *testing.T) {
	conn, user := teamConn()
	user.Auth = AuthMethodPassword
	user.PassAuth = &PasswordAuthConfig{VaultRef: "someone-elses"}
	d := NewVaultData()
	d.Identities["mine"] = SSHIdentity{ID: "mine"}
	d.Sync = &VaultSync{Bindings: []CredentialBinding{{VaultID: "t", ConnectionID: "c1", UserID: "u1", IdentityID: "mine"}}}
	got, err := ResolveCredentials(d, conn, user)
	if err != nil || got.Auth != AuthMethodKey || got.KeyAuth == nil || got.KeyAuth.IdentityIDs[0] != "mine" {
		t.Fatalf("resolved = %+v, %v; a member's key binding authenticates a user the team set up with a password", got, err)
	}
	if got.PassAuth != nil {
		t.Fatalf("passAuth = %+v survived; a reference another member wrote into the shared record must never be used", got.PassAuth)
	}

	_, keyUser := teamConn()
	keyUser.KeyAuth = &KeyAuthConfig{IdentityIDs: []string{"someone-elses"}}
	d.Passwords["pw"] = PasswordBlob{Label: "pw"}
	d.Sync.Bindings[0] = CredentialBinding{VaultID: "t", ConnectionID: "c1", UserID: "u1", PasswordID: "pw"}
	got, err = ResolveCredentials(d, conn, keyUser)
	if err != nil || got.Auth != AuthMethodPassword || got.PassAuth == nil || got.PassAuth.VaultRef != "pw" || got.KeyAuth != nil {
		t.Fatalf("resolved = %+v, %v; a password binding authenticates a key user with the password alone", got, err)
	}
}

func TestResolveCredentialsRefusals(t *testing.T) {
	conn, user := teamConn()
	d := NewVaultData()
	if _, err := ResolveCredentials(d, conn, user); !errors.Is(err, ErrBindingMissing) {
		t.Fatalf("no binding: %v, want ErrBindingMissing", err)
	}
	d.Identities["mine"] = SSHIdentity{ID: "mine"}
	d.Sync = &VaultSync{Bindings: []CredentialBinding{{VaultID: "t", ConnectionID: "c1", UserID: "u1", IdentityID: "mine", SuspendedBy: "bob"}}}
	if _, err := ResolveCredentials(d, conn, user); !errors.Is(err, ErrBindingSuspended) {
		t.Fatalf("suspended: %v, want ErrBindingSuspended: a repointed host must not receive the member's key", err)
	}
	d.Identities["mine"] = SSHIdentity{ID: "mine", VaultID: "t"}
	d.Sync.Bindings[0].SuspendedBy = ""
	if _, err := ResolveCredentials(d, conn, user); !errors.Is(err, ErrBindingMissing) {
		t.Fatalf("a binding to a team key: %v, want ErrBindingMissing - a binding names the member's own credential", err)
	}
	inline := ConnectionUser{ID: "u1", Username: "x", Auth: AuthMethodKey, KeyAuth: &KeyAuthConfig{IdentityIDs: []string{"k"}}}
	if got, err := ResolveCredentials(NewVaultData(), conn, inline); err != nil || got.KeyAuth.IdentityIDs[0] != "k" {
		t.Fatal("an inline credential is used as it is")
	}
}

// Another member writes a team connection's inline user, so a reference there may name any id it
// likes; looked up in this member's vault, an id of a personal credential would send it to whatever
// host the other member set. Only the connection's own vault may supply an inline credential.
func TestResolveCredentialsKeepsAnInlineCredentialInsideTheTeam(t *testing.T) {
	conn, _ := teamConn()
	d := NewVaultData()
	d.Identities["mine"] = SSHIdentity{ID: "mine"}
	d.Identities["other-team"] = SSHIdentity{ID: "other-team", VaultID: "u"}
	d.Identities["team-key"] = SSHIdentity{ID: "team-key", VaultID: "t"}
	d.Passwords["my-pw"] = PasswordBlob{Label: "mine"}
	d.Passwords["team-pw"] = PasswordBlob{Label: "team", VaultID: "t"}
	pwUser := func(ref string) ConnectionUser {
		return ConnectionUser{ID: "u1", Username: "x", Auth: AuthMethodPassword, PassAuth: &PasswordAuthConfig{VaultRef: ref}}
	}
	keyUser := func(ids ...string) ConnectionUser {
		return ConnectionUser{ID: "u1", Username: "x", Auth: AuthMethodKey, KeyAuth: &KeyAuthConfig{IdentityIDs: ids}}
	}
	for name, u := range map[string]ConnectionUser{
		"personal password":            pwUser("my-pw"),
		"personal key":                 keyUser("mine"),
		"another team's key":           keyUser("other-team"),
		"a personal key after a team's": keyUser("team-key", "mine"),
	} {
		if _, err := ResolveCredentials(d, conn, u); !errors.Is(err, ErrCredentialOutsideVault) {
			t.Errorf("team connection naming %s: %v, want ErrCredentialOutsideVault", name, err)
		}
	}
	for name, u := range map[string]ConnectionUser{"team password": pwUser("team-pw"), "team key": keyUser("team-key")} {
		if _, err := ResolveCredentials(d, conn, u); err != nil {
			t.Errorf("team connection naming its own %s: %v; it must keep working", name, err)
		}
	}
	personal := &Connection{ID: "p1", Users: []ConnectionUser{pwUser("team-pw")}, DefaultUserID: "u1"}
	if _, err := ResolveCredentials(d, personal, pwUser("my-pw")); err != nil {
		t.Errorf("personal connection naming a personal password: %v; only this account writes it", err)
	}
}

func TestAPersonalCredentialSourceNeedsNoInlineReference(t *testing.T) {
	_, user := teamConn()
	if err := user.Validate(); err != nil {
		t.Fatalf("validate: %v; the reference comes from each member's binding", err)
	}
	user.CredentialSource = "shared"
	if err := user.Validate(); !errors.Is(err, ErrInvalidConnectionConfig) {
		t.Fatalf("unknown source: %v", err)
	}
	user.CredentialSource, user.Auth = CredentialSourcePersonal, AuthMethodPlugin
	if err := user.Validate(); !errors.Is(err, ErrInvalidConnectionConfig) {
		t.Fatalf("personal plugin auth: %v; only keys and passwords can be bound", err)
	}
}
