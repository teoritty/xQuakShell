package domain

import "testing"

// Every field of PluginSettings that can lower the bar for a plugin is listed here with the
// direction that lowers it. A field added to the struct without an entry here is the exact
// regression this table exists to catch: the gate in SettingsService.SavePluginSettings only
// covers what PluginTrustWeakened knows to look at.
func TestPluginTrustWeakened_WeakeningDirections(t *testing.T) {
	tests := []struct {
		name string
		old  PluginSettings
		next PluginSettings
	}{
		{
			name: "signature requirement turned off",
			old:  PluginSettings{RequireSignedPlugins: true},
			next: PluginSettings{RequireSignedPlugins: false},
		},
		{
			name: "sandbox fallback turned on",
			old:  PluginSettings{AllowUnsandboxedFallback: false},
			next: PluginSettings{AllowUnsandboxedFallback: true},
		},
		{
			name: "publisher key added to an empty anchor",
			old:  PluginSettings{},
			next: PluginSettings{TrustedPublisherKeys: []string{"AAAA"}},
		},
		{
			name: "publisher key added alongside an existing one",
			old:  PluginSettings{TrustedPublisherKeys: []string{"AAAA"}},
			next: PluginSettings{TrustedPublisherKeys: []string{"AAAA", "BBBB"}},
		},
		{
			name: "publisher key swapped for a different one",
			old:  PluginSettings{TrustedPublisherKeys: []string{"AAAA"}},
			next: PluginSettings{TrustedPublisherKeys: []string{"BBBB"}},
		},
		{
			name: "secret access granted",
			old:  PluginSettings{},
			next: PluginSettings{SecretAccessGranted: map[string]bool{"p": true}},
		},
		{
			name: "auth provider access granted",
			old:  PluginSettings{AuthProviderAccessGranted: map[string]bool{"p": false}},
			next: PluginSettings{AuthProviderAccessGranted: map[string]bool{"p": true}},
		},
		{
			name: "tunnel provider access granted",
			old:  PluginSettings{},
			next: PluginSettings{TunnelProviderAccessGranted: map[string]bool{"p": true}},
		},
		{
			name: "multi session access granted",
			old:  PluginSettings{},
			next: PluginSettings{MultiSessionAccessGranted: map[string]bool{"p": true}},
		},
		{
			name: "arbitrary network access granted",
			old:  PluginSettings{},
			next: PluginSettings{ArbitraryNetworkAccessGranted: map[string]bool{"p": true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !PluginTrustWeakened(tt.old, tt.next) {
				t.Errorf("PluginTrustWeakened = false, want true; %s lowers the bar and must cost the master password", tt.name)
			}
		})
	}
}

// The asymmetry is load-bearing: if these returned true, locking the installation down would
// prompt for a password, and a control that costs something to enable is one people leave off.
func TestPluginTrustWeakened_StrengtheningIsFree(t *testing.T) {
	tests := []struct {
		name string
		old  PluginSettings
		next PluginSettings
	}{
		{
			name: "signature requirement turned on",
			old:  PluginSettings{RequireSignedPlugins: false},
			next: PluginSettings{RequireSignedPlugins: true},
		},
		{
			name: "sandbox fallback turned off",
			old:  PluginSettings{AllowUnsandboxedFallback: true},
			next: PluginSettings{AllowUnsandboxedFallback: false},
		},
		{
			name: "publisher key revoked",
			old:  PluginSettings{TrustedPublisherKeys: []string{"AAAA", "BBBB"}},
			next: PluginSettings{TrustedPublisherKeys: []string{"AAAA"}},
		},
		{
			name: "every publisher key revoked",
			old:  PluginSettings{TrustedPublisherKeys: []string{"AAAA"}},
			next: PluginSettings{},
		},
		{
			name: "publisher keys reordered",
			old:  PluginSettings{TrustedPublisherKeys: []string{"AAAA", "BBBB"}},
			next: PluginSettings{TrustedPublisherKeys: []string{"BBBB", "AAAA"}},
		},
		{
			name: "grant revoked by flipping to false",
			old:  PluginSettings{SecretAccessGranted: map[string]bool{"p": true}},
			next: PluginSettings{SecretAccessGranted: map[string]bool{"p": false}},
		},
		{
			name: "grant revoked by dropping the entry",
			old:  PluginSettings{SecretAccessGranted: map[string]bool{"p": true}},
			next: PluginSettings{},
		},
		{
			name: "explicit false added where there was no entry",
			old:  PluginSettings{},
			next: PluginSettings{SecretAccessGranted: map[string]bool{"p": false}},
		},
		{
			name: "nothing changed",
			old:  PluginSettings{RequireSignedPlugins: true, TrustedPublisherKeys: []string{"AAAA"}},
			next: PluginSettings{RequireSignedPlugins: true, TrustedPublisherKeys: []string{"AAAA"}},
		},
		{
			name: "an unrelated field changed",
			old:  PluginSettings{Disabled: map[string]bool{"p": false}},
			next: PluginSettings{Disabled: map[string]bool{"p": true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if PluginTrustWeakened(tt.old, tt.next) {
				t.Errorf("PluginTrustWeakened = true, want false; %s does not lower the bar and must not need a password", tt.name)
			}
		})
	}
}

// One save can carry several edits at once. A weakening buried among strengthenings still costs
// the password - taking the whole change as safe because most of it is would be the bypass.
func TestPluginTrustWeakened_MixedChangeIsWeakening(t *testing.T) {
	old := PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"AAAA", "BBBB"},
	}
	next := PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"AAAA", "CCCC"},
	}
	if !PluginTrustWeakened(old, next) {
		t.Error("PluginTrustWeakened = false, want true; revoking BBBB does not pay for trusting CCCC")
	}
}
