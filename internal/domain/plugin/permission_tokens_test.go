package plugin

import "testing"

// The token strings are written into the vault as part of a grant, which makes them an on-disk
// contract rather than an implementation detail.
//
// Renaming one migrates nothing. It simply stops matching what is already recorded, so every plugin
// that held that permission silently loses it - and, because a missing permission reads as "not
// granted" rather than as an error, the first sign would be a plugin quietly failing to do its job.
// This test exists so that renaming one has to be a deliberate act with a migration behind it.
func TestPermissionTokensAreAnOnDiskContract(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{permissionVaultGetSecret, "vault.getSecret"},
		{permissionAuthMethods, "auth.methods"},
		{permissionExecCommands, "channel.execCommands"},
		{PermissionAuthProvider, "auth.provider"},
		{PermissionTunnelProvider, "tunnel.provider"},
		{PermissionMultiSession, "session.allowMultiSession"},
		{PermissionArbitraryOutbound, "network.allowArbitraryOutbound"},
		{PermissionPrivateNetworks, "network.allowPrivateNetworks"},
		{PermissionSecretField("password"), "vault.getSecret:password"},
	} {
		if tc.got != tc.want {
			t.Errorf("token = %q, want %q; changing a stored token revokes it for every plugin that held it", tc.got, tc.want)
		}
	}
}

// The named tokens have to be the ones the mapper actually produces. A constant that drifted from
// the manifest field it stands for would be a permission no enforcement point could ever match,
// which fails closed and silently - the plugin simply stops working with nothing to point at.
func TestTheNamedTokensAreTheOnesAManifestProduces(t *testing.T) {
	produced := PermissionSetFromManifest(&Manifest{
		ID:           "com.example.sync",
		Capabilities: fullyPopulatedCapabilities(),
	})

	for _, token := range []string{
		PermissionAuthProvider,
		PermissionTunnelProvider,
		PermissionMultiSession,
		PermissionArbitraryOutbound,
		PermissionPrivateNetworks,
		PermissionSecretField("password"),
	} {
		if !produced.Has(token) {
			t.Errorf("named token %q is not among the permissions a manifest requesting it produces", token)
		}
	}
}
