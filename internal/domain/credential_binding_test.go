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
