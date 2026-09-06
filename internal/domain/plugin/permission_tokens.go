package plugin

// The permission vocabulary. These strings are written into the vault as part of a grant, so they
// are an on-disk contract: renaming one does not migrate anything, it silently stops matching the
// consent already recorded and revokes it for every plugin that had it. A test pins them.
//
// The names are derived from the manifest's own JSON tags, so a reader can move between plugin.json
// and a recorded grant without a translation table.
const (
	// permissionVaultGetSecret is a prefix; the field name follows. Consent is per field, so a
	// plugin allowed a connection password has not thereby been allowed the private key.
	permissionVaultGetSecret = "vault.getSecret"
	permissionAuthMethods    = "auth.methods"
	permissionExecCommands   = "channel.execCommands"
	permissionConfigSlots    = "config.slots"

	// PermissionScope is the folder itself, not what ends up in it: an empty scope grants nothing,
	// and what it exposes is decided later, by the user, one object at a time.
	PermissionScope = "scope"

	// PermissionAuthProvider allows a plugin to answer SSH authentication for connections bound to
	// it.
	PermissionAuthProvider = "auth.provider"
	// PermissionTunnelProvider allows a plugin to decide routing for dynamic forward rules.
	PermissionTunnelProvider = "tunnel.provider"
	// PermissionMultiSession allows one plugin process to hold several sessions at once.
	PermissionMultiSession = "session.allowMultiSession"
	// PermissionArbitraryOutbound allows a plugin to dial hosts its manifest did not name.
	PermissionArbitraryOutbound = "network.allowArbitraryOutbound"
	// PermissionPrivateNetworks widens PermissionArbitraryOutbound to private, loopback and
	// link-local addresses. It means nothing on its own, which is why the two are consented to
	// together.
	PermissionPrivateNetworks = "network.allowPrivateNetworks"
)

// PermissionSecretField names the permission to read one connection secret field, so an enforcement
// point asks the same question the manifest recorded rather than building the string itself.
func PermissionSecretField(field string) string {
	return permissionVaultGetSecret + ":" + field
}
