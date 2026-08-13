package domain

import "testing"

func grantedSettings(pluginID string) *PluginSettings {
	return &PluginSettings{
		SecretAccessGranted:           map[string]bool{pluginID: true},
		AuthProviderAccessGranted:     map[string]bool{pluginID: true},
		TunnelProviderAccessGranted:   map[string]bool{pluginID: true},
		MultiSessionAccessGranted:     map[string]bool{pluginID: true},
		ArbitraryNetworkAccessGranted: map[string]bool{pluginID: true},
		Disabled:                      map[string]bool{pluginID: true},
	}
}

// A grant is consent given to a particular plugin. Uninstalling used to leave every one of them
// behind, keyed by an id the plugin author chose and nothing verifies - so the next thing
// installed under that id inherited a decision the user made about something else.
//
// Every map is listed individually on purpose: a grant map added to PluginSettings without a line
// in RevokePluginGrants is exactly the regression this catches.
func TestRevokePluginGrantsClearsEveryMap(t *testing.T) {
	const pluginID = "com.example.tool"
	settings := grantedSettings(pluginID)

	settings.RevokePluginGrants(pluginID)

	checks := map[string]map[string]bool{
		"SecretAccessGranted":           settings.SecretAccessGranted,
		"AuthProviderAccessGranted":     settings.AuthProviderAccessGranted,
		"TunnelProviderAccessGranted":   settings.TunnelProviderAccessGranted,
		"MultiSessionAccessGranted":     settings.MultiSessionAccessGranted,
		"ArbitraryNetworkAccessGranted": settings.ArbitraryNetworkAccessGranted,
		"Disabled":                      settings.Disabled,
	}
	for name, m := range checks {
		if _, present := m[pluginID]; present {
			t.Errorf("%s still carries an entry for %s after revocation", name, pluginID)
		}
	}
}

// Deleting the key matters, not just setting it false: an explicit false and an absent entry read
// the same at every call site, but a lingering key is a record of a plugin that no longer exists.
func TestRevokePluginGrantsDeletesRatherThanFlagging(t *testing.T) {
	const pluginID = "com.example.tool"
	settings := grantedSettings(pluginID)

	settings.RevokePluginGrants(pluginID)

	if len(settings.SecretAccessGranted) != 0 {
		t.Errorf("SecretAccessGranted = %v, want empty", settings.SecretAccessGranted)
	}
}

// Uninstalling one plugin must not touch anyone else's consent.
func TestRevokePluginGrantsLeavesOtherPluginsAlone(t *testing.T) {
	settings := grantedSettings("com.example.tool")
	settings.SecretAccessGranted["com.example.other"] = true
	settings.Disabled["com.example.other"] = true

	settings.RevokePluginGrants("com.example.tool")

	if !settings.SecretAccessGranted["com.example.other"] {
		t.Error("another plugin's secret grant was revoked")
	}
	if !settings.Disabled["com.example.other"] {
		t.Error("another plugin's disabled marker was cleared")
	}
}

// The returned list is what gets logged, so it has to say what was actually lost rather than
// announce a revocation that removed nothing.
func TestRevokePluginGrantsReportsWhatItRevoked(t *testing.T) {
	settings := &PluginSettings{
		SecretAccessGranted:       map[string]bool{"com.example.tool": true},
		AuthProviderAccessGranted: map[string]bool{"com.example.tool": false},
	}

	revoked := settings.RevokePluginGrants("com.example.tool")

	if len(revoked) != 1 || revoked[0] != "secret" {
		t.Errorf("revoked = %v, want [secret]; a grant recorded as false was never held", revoked)
	}
}

// Nothing granted, nothing to report - and no panic on the nil maps a fresh vault carries.
func TestRevokePluginGrantsOnEmptySettings(t *testing.T) {
	settings := &PluginSettings{}

	if revoked := settings.RevokePluginGrants("com.example.tool"); len(revoked) != 0 {
		t.Errorf("revoked = %v, want none", revoked)
	}

	var nilSettings *PluginSettings
	if revoked := nilSettings.RevokePluginGrants("com.example.tool"); revoked != nil {
		t.Errorf("revoked = %v on nil settings, want nil", revoked)
	}
}
