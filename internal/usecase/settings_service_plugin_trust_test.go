package usecase

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

// trustTestVault records what SavePluginSettings asked of the vault. correctPassword is the only
// string VerifyMasterPassword accepts, so a test can tell "the gate ran and passed" apart from
// "the gate never ran".
type trustTestVault struct {
	data            *domain.VaultData
	correctPassword string
	verifyCalls     []string
	written         *domain.PluginSettings
}

func (v *trustTestVault) Exists() bool                         { return true }
func (v *trustTestVault) Create(context.Context, string) error { return nil }
func (v *trustTestVault) Unlock(context.Context, string) error { return nil }
func (v *trustTestVault) Lock()                                {}
func (v *trustTestVault) IsUnlocked() bool                     { return true }
func (v *trustTestVault) GetData() (*domain.VaultData, error)  { return v.data, nil }

func (v *trustTestVault) VerifyMasterPassword(_ context.Context, password string) error {
	v.verifyCalls = append(v.verifyCalls, password)
	if password != v.correctPassword {
		return domain.ErrVaultDecryptFailed
	}
	return nil
}

func (v *trustTestVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	if err := mutate(v.data); err != nil {
		return err
	}
	saved := v.data.Settings.Plugins
	v.written = &saved
	return nil
}

func newTrustVault(stored domain.PluginSettings) *trustTestVault {
	settings := domain.AppSettings{Plugins: stored}
	return &trustTestVault{
		data:            &domain.VaultData{Settings: &settings},
		correctPassword: "correct horse battery",
	}
}

// The attack this closes: anything holding window.go adds its own publisher key, then presents
// its own plugin as signed by a trusted publisher. The master password never crosses the bridge,
// so requiring it here is what separates the user from code running in the WebView.
func TestSavePluginSettings_WeakeningWithoutPasswordIsRefused(t *testing.T) {
	vault := newTrustVault(domain.PluginSettings{RequireSignedPlugins: true})
	svc := NewSettingsService(vault, nil, nil)

	attacker := domain.PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"attacker-key"},
	}
	err := svc.SavePluginSettings(context.Background(), attacker, "")

	if !errors.Is(err, domain.ErrPluginTrustReauthRequired) {
		t.Fatalf("SavePluginSettings err = %v, want ErrPluginTrustReauthRequired", err)
	}
	if vault.written != nil {
		t.Errorf("trust anchor was written despite the refusal: %+v", *vault.written)
	}
}

func TestSavePluginSettings_WeakeningWithWrongPasswordIsRefused(t *testing.T) {
	vault := newTrustVault(domain.PluginSettings{RequireSignedPlugins: true})
	svc := NewSettingsService(vault, nil, nil)

	weakened := domain.PluginSettings{RequireSignedPlugins: false}
	err := svc.SavePluginSettings(context.Background(), weakened, "guess")

	if !errors.Is(err, domain.ErrPluginTrustReauthRequired) {
		t.Fatalf("SavePluginSettings err = %v, want ErrPluginTrustReauthRequired", err)
	}
	if vault.written != nil {
		t.Error("signature requirement was turned off with a wrong password")
	}
}

func TestSavePluginSettings_WeakeningWithCorrectPasswordIsAllowed(t *testing.T) {
	vault := newTrustVault(domain.PluginSettings{RequireSignedPlugins: true})
	svc := NewSettingsService(vault, nil, nil)

	next := domain.PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"deliberate-key"},
	}
	if err := svc.SavePluginSettings(context.Background(), next, vault.correctPassword); err != nil {
		t.Fatalf("SavePluginSettings err = %v, want nil", err)
	}
	if vault.written == nil {
		t.Fatal("trust anchor was not written after a verified password")
	}
	if len(vault.written.TrustedPublisherKeys) != 1 || vault.written.TrustedPublisherKeys[0] != "deliberate-key" {
		t.Errorf("written keys = %v, want [deliberate-key]", vault.written.TrustedPublisherKeys)
	}
}

// Locking the installation down must not prompt. If it did, the password would be the price of
// enabling a security control, and that price is why controls stay off.
func TestSavePluginSettings_StrengtheningNeedsNoPassword(t *testing.T) {
	vault := newTrustVault(domain.PluginSettings{
		RequireSignedPlugins: false,
		TrustedPublisherKeys: []string{"old-key"},
	})
	svc := NewSettingsService(vault, nil, nil)

	hardened := domain.PluginSettings{RequireSignedPlugins: true}
	if err := svc.SavePluginSettings(context.Background(), hardened, ""); err != nil {
		t.Fatalf("SavePluginSettings err = %v, want nil", err)
	}
	if len(vault.verifyCalls) != 0 {
		t.Errorf("VerifyMasterPassword called %d times for a strengthening change, want 0", len(vault.verifyCalls))
	}
	if vault.written == nil || !vault.written.RequireSignedPlugins {
		t.Error("hardening change was not persisted")
	}
}

// A save that changes nothing about trust - the sandbox opt-out left alone, a plugin disabled -
// must go through untouched, or every unrelated settings write starts asking for a password.
func TestSavePluginSettings_UnrelatedChangeNeedsNoPassword(t *testing.T) {
	vault := newTrustVault(domain.PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"key"},
	})
	svc := NewSettingsService(vault, nil, nil)

	next := domain.PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"key"},
		Disabled:             map[string]bool{"some-plugin": true},
	}
	if err := svc.SavePluginSettings(context.Background(), next, ""); err != nil {
		t.Fatalf("SavePluginSettings err = %v, want nil", err)
	}
	if len(vault.verifyCalls) != 0 {
		t.Errorf("VerifyMasterPassword called %d times, want 0", len(vault.verifyCalls))
	}
}
