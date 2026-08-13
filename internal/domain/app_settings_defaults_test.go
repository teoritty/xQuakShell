package domain

import "testing"

// A plugin is arbitrary code on the user's machine, and the default decides this for everyone who
// never opens the settings dialog. RequireSignedPlugins shipped off only because the field is a
// bool with `omitempty` and false is the zero value - nobody chose it.
func TestDefaultPluginSettingsRequireSignedPlugins(t *testing.T) {
	if !DefaultPluginSettings().RequireSignedPlugins {
		t.Error("DefaultPluginSettings().RequireSignedPlugins = false; a fresh vault must not accept unsigned plugins")
	}
}

// The other two defaults in this struct are permissive on purpose and must stay that way: an empty
// key list is the honest starting point, and the sandbox opt-out ships off.
func TestDefaultPluginSettingsHaveNoTrustedKeysAndNoSandboxOptOut(t *testing.T) {
	defaults := DefaultPluginSettings()
	if len(defaults.TrustedPublisherKeys) != 0 {
		t.Errorf("DefaultPluginSettings().TrustedPublisherKeys = %v, want empty", defaults.TrustedPublisherKeys)
	}
	if defaults.AllowUnsandboxedFallback {
		t.Error("DefaultPluginSettings().AllowUnsandboxedFallback = true; a sandbox that ships optional is a sandbox that ships off")
	}
}

// NewVaultData is the fresh-vault path. If it stopped routing through DefaultPluginSettings the
// section would arrive zero-valued and the signature requirement would be off again.
func TestNewVaultDataCarriesThePluginDefaults(t *testing.T) {
	data := NewVaultData()
	if data.Settings == nil {
		t.Fatal("NewVaultData produced no settings")
	}
	if !data.Settings.Plugins.RequireSignedPlugins {
		t.Error("a new vault does not require signed plugins")
	}
}
