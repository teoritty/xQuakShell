package domain_test

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

func pluginOwned(t *testing.T, conn domain.Connection) domain.Connection {
	t.Helper()
	owner, err := domain.PluginOwner("com.example.sync")
	if err != nil {
		t.Fatalf("PluginOwner: %v", err)
	}
	conn.Owner = owner
	return conn
}

func keyUser() domain.ConnectionUser {
	return domain.ConnectionUser{
		ID: "u1", Username: "root", Auth: domain.AuthMethodKey,
		KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"id1"}},
	}
}

func passwordUser() domain.ConnectionUser {
	return domain.ConnectionUser{
		ID: "u1", Username: "root", Auth: domain.AuthMethodPassword,
		PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"},
	}
}

// The user's own connections are unaffected. They reference the user's own keys and passwords, which
// is the ordinary case and must stay free.
func TestACoreConnectionMayUseTheVault(t *testing.T) {
	conn := domain.Connection{ID: "c1", Host: "h", Users: []domain.ConnectionUser{keyUser()}}

	if err := conn.ValidateProvenance(); err != nil {
		t.Fatalf("a core-owned connection was refused its own key: %v", err)
	}
}

// The escalation this rule exists to stop. A plugin that could provision a connection pointing at a
// key in the vault would not need to read that key: it would ask the core to authenticate with it,
// against a host the plugin chose. The key never leaves, and the plugin gets everything it would
// have got by stealing it.
func TestAPluginConnectionMayNotUseAVaultKey(t *testing.T) {
	conn := pluginOwned(t, domain.Connection{ID: "c1", Host: "h", Users: []domain.ConnectionUser{keyUser()}})

	if err := conn.ValidateProvenance(); !errors.Is(err, domain.ErrProvenanceViolation) {
		t.Fatalf("err = %v, want ErrProvenanceViolation", err)
	}
}

// The same argument for a stored password: the plugin would not read it, it would have the core
// spend it somewhere of the plugin's choosing.
func TestAPluginConnectionMayNotUseAVaultPassword(t *testing.T) {
	conn := pluginOwned(t, domain.Connection{ID: "c1", Host: "h", Users: []domain.ConnectionUser{passwordUser()}})

	if err := conn.ValidateProvenance(); !errors.Is(err, domain.ErrProvenanceViolation) {
		t.Fatalf("err = %v, want ErrProvenanceViolation", err)
	}
}

// A jump hop authenticates too, and a rule that only looked at the connection's own users would let
// the same key be spent one hop earlier - on a host the plugin also chose.
func TestAPluginConnectionMayNotUseTheVaultInAJumpHop(t *testing.T) {
	for _, hop := range []domain.JumpHop{
		{ID: "j1", Host: "bastion", Port: 22, Username: "root", Auth: domain.AuthMethodKey,
			KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"id1"}}},
		{ID: "j1", Host: "bastion", Port: 22, Username: "root", Auth: domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"}},
	} {
		conn := pluginOwned(t, domain.Connection{
			ID: "c1", Host: "h",
			JumpChain: domain.JumpChainConfig{Hops: []domain.JumpHop{hop}},
		})

		if err := conn.ValidateProvenance(); !errors.Is(err, domain.ErrProvenanceViolation) {
			t.Errorf("hop %+v: err = %v, want ErrProvenanceViolation", hop.Auth, err)
		}
	}
}

// What a plugin-provisioned connection is supposed to do instead: authenticate through the plugin.
// The key stays with the plugin and the core sees a signature, which is the shape ADR-022 asks for
// wherever a protocol allows it.
func TestAPluginConnectionMayAuthenticateThroughItsPlugin(t *testing.T) {
	conn := pluginOwned(t, domain.Connection{
		ID: "c1", Host: "h",
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "root", Auth: domain.AuthMethodPlugin,
			PluginAuth: &domain.PluginAuthConfig{PluginID: "com.example.sync", AuthMethodID: "cert"},
		}},
		JumpChain: domain.JumpChainConfig{Hops: []domain.JumpHop{{
			ID: "j1", Host: "bastion", Port: 22, Username: "root",
			Auth:       domain.AuthMethodPlugin,
			PluginAuth: &domain.PluginAuthConfig{PluginID: "com.example.sync", AuthMethodID: "cert"},
		}}},
	})

	if err := conn.ValidateProvenance(); err != nil {
		t.Fatalf("a plugin connection authenticating through its plugin was refused: %v", err)
	}
}

// A plugin connection with no stored credentials at all is the other legitimate shape: the secret
// arrives from the plugin when the session opens and is never written down.
func TestAPluginConnectionWithNoStoredCredentialsIsFine(t *testing.T) {
	conn := pluginOwned(t, domain.Connection{
		ID: "c1", Host: "h",
		Users: []domain.ConnectionUser{{ID: "u1", Username: "root"}},
	})

	if err := conn.ValidateProvenance(); err != nil {
		t.Fatalf("a plugin connection storing nothing was refused: %v", err)
	}
}

// The rule has to run where connections are checked, or it is a function nobody calls. Validate is
// what every save goes through.
func TestValidateEnforcesTheProvenanceRule(t *testing.T) {
	conn := pluginOwned(t, domain.Connection{
		ID: "c1", Host: "h", Port: 22,
		Users: []domain.ConnectionUser{keyUser()},
	})

	if err := conn.Validate(); !errors.Is(err, domain.ErrProvenanceViolation) {
		t.Fatalf("Validate err = %v, want ErrProvenanceViolation", err)
	}
}

// An empty configuration references nothing, so it is not a reference to the vault. Refusing it
// would reject a connection that has merely been half-filled in, which is a draft rather than an
// attack. Both shapes are checked: a rule that only looked at the value for one of them would refuse
// an ordinary editing state for the other.
func TestAnEmptyCredentialReferenceIsNotAVaultReference(t *testing.T) {
	for _, user := range []domain.ConnectionUser{
		{ID: "u1", Username: "root", KeyAuth: &domain.KeyAuthConfig{}},
		{ID: "u1", Username: "root", PassAuth: &domain.PasswordAuthConfig{}},
	} {
		conn := pluginOwned(t, domain.Connection{
			ID: "c1", Host: "h", Users: []domain.ConnectionUser{user},
		})

		if err := conn.ValidateProvenance(); err != nil {
			t.Errorf("an empty reference was treated as a vault reference: %v", err)
		}
	}
}
