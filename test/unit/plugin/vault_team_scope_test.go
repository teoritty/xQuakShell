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

// scopeVault serves fixed vault data: the items a team connection's references are checked against.
type scopeVault struct{ data *domain.VaultData }

func (scopeVault) Exists() bool                                       { return true }
func (scopeVault) Create(context.Context, string) error               { return nil }
func (scopeVault) Unlock(context.Context, string) error               { return nil }
func (scopeVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (scopeVault) Lock()                                              {}
func (scopeVault) IsUnlocked() bool                                   { return true }
func (v scopeVault) GetData() (*domain.VaultData, error)              { return v.data, nil }
func (scopeVault) UpdateData(context.Context, func(*domain.VaultData) error) error {
	return nil
}

func teamPasswordInbound(t *testing.T, ref string, vault domain.VaultRepository) *usecase.PluginVaultInbound {
	t.Helper()
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine:       domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}}},
		},
	})
	conn := &domain.Connection{ID: "tc", VaultID: "team1", DefaultUserID: "u1",
		Users: []domain.ConnectionUser{{ID: "u1", Username: "u", Auth: domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{VaultRef: ref}}}}
	inbound := usecase.NewPluginVaultInbound(registry, vaultConnRepo{conn: conn}, vaultPasswordRepo{}, vaultIdentRepo{},
		vaultSettingsReader{granted: true}, nil)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:tc": true}})
	inbound.SetAuditLogger(&recordingVaultAudit{})
	if vault != nil {
		inbound.SetVault(vault)
	}
	return inbound
}

// Another team member writes a team connection's user, so a reference there to this member's
// personal password would hand that password to a plugin this member trusts, for a host the other
// member chose. The plugin gets the team's own password and nothing from outside the team.
func TestVaultGetSecretRefusesAPersonalPasswordForATeamConnection(t *testing.T) {
	data := domain.NewVaultData()
	data.Passwords["mine"] = domain.PasswordBlob{Value: []byte("personal")}
	data.Passwords["shared"] = domain.PasswordBlob{Value: []byte("team"), VaultID: "team1"}
	params, _ := json.Marshal(map[string]string{"connectionId": "tc", "field": "password"})

	_, err := teamPasswordInbound(t, "mine", scopeVault{data}).GetSecret(context.Background(), "com.test.vault", params)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("team connection naming a personal password: %v, want ErrCapabilityDenied", err)
	}
	if _, err := teamPasswordInbound(t, "shared", scopeVault{data}).GetSecret(context.Background(), "com.test.vault", params); err != nil {
		t.Fatalf("team connection naming the team's own password: %v; it must keep working", err)
	}
	_, err = teamPasswordInbound(t, "shared", nil).GetSecret(context.Background(), "com.test.vault", params)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("no vault to check against: %v, want ErrCapabilityDenied; an unchecked team secret is refused", err)
	}
}
