package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// grantSettings answers with a recorded grant rather than the boolean maps that preceded it.
type grantSettings struct {
	permissions []string
	legacyBool  bool
}

func (s grantSettings) PluginSettings() (domain.PluginSettings, error) {
	var settings domain.PluginSettings
	if s.legacyBool {
		settings.SecretAccessGranted = map[string]bool{"com.test.vault": true}
	}
	if s.permissions != nil {
		settings.RecordGrant(domain.PluginGrant{
			PluginID: "com.test.vault",
			Granted:  s.permissions,
		})
	}
	return settings, nil
}

func grantTestInbound(t *testing.T, declaredFields []string, settings usecase.PluginSettingsReader) *usecase.PluginVaultInbound {
	t.Helper()
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: declaredFields},
			},
		},
	})
	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: &domain.Connection{
			ID: "c1",
			Users: []domain.ConnectionUser{{
				ID: "u1", Username: "u", Auth: domain.AuthMethodPassword,
				PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"},
			}},
			DefaultUserID: "u1",
		}},
		vaultPasswordRepo{},
		vaultIdentRepo{},
		settings,
		nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})
	inbound.SetAuditLogger(&recordingVaultAudit{})
	return inbound
}

func getSecret(inbound *usecase.PluginVaultInbound, field string) error {
	params, _ := json.Marshal(map[string]string{"connectionId": "c1", "field": field})
	_, err := inbound.GetSecret(context.Background(), "com.test.vault", params)
	return err
}

// The permission the user agreed to opens the field it names, and nothing here should be harder
// than before for a plugin doing what it was installed to do.
func TestVaultGetSecretAllowedForAGrantedField(t *testing.T) {
	inbound := grantTestInbound(t, []string{"password"}, grantSettings{
		permissions: []string{domainplugin.PermissionSecretField("password")},
	})

	if err := getSecret(inbound, "password"); err != nil {
		t.Fatalf("a granted field was refused: %v", err)
	}
}

// The point of recording consent per permission rather than as one boolean.
//
// The manifest declares two secret fields and the user agreed to one. Under the boolean this was
// indistinguishable from agreeing to both, so a plugin that shipped an update adding a field to its
// manifest could read it on the strength of a consent given for another. Here the second field is
// refused until it is consented to by name.
//
// The refused field is "password" rather than "privateKey" on purpose. This connection has a
// password and no key, so a request for the key is refused by the resolver anyway, with the same
// error - a test written that way passes whether or not consent is checked at all, which is how an
// earlier version of it did. Refusing the field that WOULD resolve is what makes the assertion mean
// something: without the consent check this call succeeds.
func TestVaultGetSecretDeniedForAFieldTheGrantDoesNotCover(t *testing.T) {
	inbound := grantTestInbound(t, []string{"password", "passphrase"}, grantSettings{
		permissions: []string{domainplugin.PermissionSecretField("passphrase")},
	})

	err := getSecret(inbound, "password")

	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied for a field the grant does not name", err)
	}
}

// The guard that matters when composition is incomplete: with no settings to read there is no
// consent, and no consent means no secret.
func TestVaultGetSecretDeniedWhenSettingsAreUnwired(t *testing.T) {
	inbound := grantTestInbound(t, []string{"password"}, nil)

	if err := getSecret(inbound, "password"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied when there are no settings to read", err)
	}
}

// A plugin with a grant that permits nothing is a plugin the user refused. It must not fall back
// to being allowed whatever its manifest happens to declare.
func TestVaultGetSecretDeniedWhenTheGrantPermitsNothing(t *testing.T) {
	inbound := grantTestInbound(t, []string{"password"}, grantSettings{permissions: []string{}})

	if err := getSecret(inbound, "password"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}

// The grant is now the authority, and the maps it replaced are not consulted. A vault still holding
// the old boolean - because migration has not run, or failed - must not open anything on its own,
// or the switch to grants would be cosmetic.
func TestVaultGetSecretIgnoresTheOldBooleanWithoutAGrant(t *testing.T) {
	inbound := grantTestInbound(t, []string{"password"}, grantSettings{legacyBool: true})

	if err := getSecret(inbound, "password"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied - the old boolean must not grant on its own", err)
	}
}

// A field outside the manifest is refused whatever the grant says. Consent bounds what the manifest
// asked for; it cannot reach past it, and a grant naming a field the manifest dropped - because a
// later version stopped asking for it - must not resurrect it.
//
// As above, the field is one that WOULD resolve. Asking for a key this connection does not have
// would be refused by the resolver with the same error, and the test would pass with the manifest
// bound removed entirely.
func TestVaultGetSecretDeniedForAFieldTheManifestNoLongerDeclares(t *testing.T) {
	inbound := grantTestInbound(t, []string{"passphrase"}, grantSettings{
		permissions: []string{
			domainplugin.PermissionSecretField("password"),
			domainplugin.PermissionSecretField("passphrase"),
		},
	})

	if err := getSecret(inbound, "password"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied for a field the manifest does not declare", err)
	}
}

type failingSettings struct{}

// A grant is returned ALONGSIDE the error, which is what makes this test worth anything. A reader
// that zeroed its result on failure would deny by accident, and the error check could be deleted
// without any test noticing - nothing states that contract, so nothing may rely on it.
func (failingSettings) PluginSettings() (domain.PluginSettings, error) {
	var settings domain.PluginSettings
	settings.RecordGrant(domain.PluginGrant{
		PluginID: "com.test.vault",
		Granted:  []string{domainplugin.PermissionSecretField("password")},
	})
	return settings, domain.ErrVaultLocked
}

// Settings that cannot be read must deny. A read failure that fell through to "allowed" would turn
// a locked or damaged vault into secret access for whatever asked at that moment, and the field
// requested here is one that would otherwise resolve, so the denial is the consent check and not the
// resolver.
func TestVaultGetSecretDeniedWhenSettingsCannotBeRead(t *testing.T) {
	inbound := grantTestInbound(t, []string{"password"}, failingSettings{})

	if err := getSecret(inbound, "password"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied when settings cannot be read", err)
	}
}
