package plugin_test

import (
	"context"
	"encoding/json"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// keyFlagIdentRepo serves one identity whose AllowPlugins flag the test controls, and records
// whether the key bytes were ever handed out.
type keyFlagIdentRepo struct {
	identity domain.SSHIdentity
	released *bool
}

func (r keyFlagIdentRepo) GetAll(context.Context) ([]domain.SSHIdentity, error) {
	return []domain.SSHIdentity{r.identity}, nil
}
func (r keyFlagIdentRepo) Get(context.Context, string) (*domain.SSHIdentity, error) {
	clone := r.identity
	return &clone, nil
}
func (r keyFlagIdentRepo) GetKeyBlob(context.Context, string) ([]byte, error) {
	*r.released = true
	return []byte("private-key-bytes"), nil
}
func (r keyFlagIdentRepo) GetBlob(context.Context, string) (*domain.IdentityBlob, error) {
	*r.released = true
	return &domain.IdentityBlob{PEMData: []byte("private-key-bytes")}, nil
}
func (keyFlagIdentRepo) Import(context.Context, []byte, string) (*domain.SSHIdentity, error) {
	return nil, nil
}
func (keyFlagIdentRepo) Save(context.Context, domain.SSHIdentity, domain.IdentityBlob) error {
	return nil
}
func (keyFlagIdentRepo) Update(context.Context, string, func(*domain.SSHIdentity) error) error {
	return nil
}
func (keyFlagIdentRepo) Delete(context.Context, string) error { return nil }

func keyConnection() *domain.Connection {
	return &domain.Connection{
		ID: "c1", Host: "h", DefaultUserID: "u1",
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "root", Auth: domain.AuthMethodKey,
			KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}},
		}},
	}
}

// askForSecret drives one vault.getSecret call for a plugin granted that field, with a key whose
// AllowPlugins flag is under the test's control.
func askForSecret(t *testing.T, field string, allowPlugins bool, cache domain.PassphraseCache) (released bool, err error) {
	t.Helper()
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{field}},
			},
		},
	})

	identity := domain.SSHIdentity{
		ID: "k1", Comment: "prod", Encrypted: true,
		Policy: domain.KeyPolicyPassphrase, AllowPlugins: allowPlugins,
	}
	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: keyConnection()},
		vaultPasswordRepo{},
		keyFlagIdentRepo{identity: identity, released: &released},
		vaultSettingsReader{granted: true},
		cache,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})
	inbound.SetAuditLogger(&recordingVaultAudit{})

	params, _ := json.Marshal(map[string]string{"connectionId": "c1", "field": field})
	_, err = inbound.GetSecret(context.Background(), "com.test.vault", params)
	return released, err
}

// The default is refusal. Before the key manager, any plugin holding the vault capability could
// read any private key belonging to a connection it was invoked for; the capability grant is
// about the vault as a whole and is far too coarse for "this third-party binary may read this
// particular private key".
func TestAPluginCannotReadAPrivateKeyByDefault(t *testing.T) {
	released, err := askForSecret(t, "privateKey", false, nil)
	if err == nil {
		t.Fatal("a plugin read a private key from a connection whose key was never opened up to plugins")
	}
	if released {
		t.Error("the key bytes were fetched before the flag was checked; a denial after the read is not a denial")
	}
}

func TestAPluginReadsAPrivateKeyOnlyWhenTheKeySaysSo(t *testing.T) {
	released, err := askForSecret(t, "privateKey", true, nil)
	if err != nil {
		t.Fatalf("a plugin was refused a key it was explicitly allowed: %v", err)
	}
	if !released {
		t.Error("the call succeeded without ever reading the key")
	}
}

// The passphrase is as much a secret as the key it opens. Gating only the key would leave the
// plugin able to ask for both in either order.
func TestAPluginCannotReadAKeyPassphraseByDefault(t *testing.T) {
	cache := &memoryPassphraseCache{values: map[string]string{"k1": "hunter2"}}
	if _, err := askForSecret(t, "passphrase", false, cache); err == nil {
		t.Fatal("a plugin read a key passphrase for a key that was never opened up to plugins")
	}
}

func TestAPluginReadsAPassphraseOnlyWhenTheKeySaysSo(t *testing.T) {
	cache := &memoryPassphraseCache{values: map[string]string{"k1": "hunter2"}}
	if _, err := askForSecret(t, "passphrase", true, cache); err != nil {
		t.Fatalf("a plugin was refused a passphrase it was explicitly allowed: %v", err)
	}
}
