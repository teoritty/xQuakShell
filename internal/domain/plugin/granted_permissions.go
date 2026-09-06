package plugin

import "strings"

// ConsentFlags is what the install dialog asked the user, one field per elevated permission group.
//
// The groups are the ones the dialog already offers separately, so a user who agreed to secret
// access and declined a tunnel provider keeps that distinction in what is recorded. A single
// all-or-nothing grant would lose it, and the first thing lost would be the refusal.
type ConsentFlags struct {
	SecretAccess     bool
	AuthProvider     bool
	TunnelProvider   bool
	MultiSession     bool
	ArbitraryNetwork bool
	ExecChannel      bool
}

// GrantedPermissions returns the subset of a manifest's permissions the user actually agreed to.
//
// Consent bounds what was requested and never adds to it: a box ticked for a permission the manifest
// never asked for grants nothing, because the manifest is the only source of what a plugin may want.
// Everything that is not elevated is conferred by installing the plugin at all - the dialog does not
// offer those separately, so recording them as refused would deny a plugin permissions nobody was
// ever asked about.
func GrantedPermissions(m *Manifest, consent ConsentFlags) PermissionSet {
	requested := PermissionSetFromManifest(m)
	kept := make([]string, 0, len(requested.tokens))
	for _, token := range requested.tokens {
		if consentFor(token, consent) {
			kept = append(kept, token)
		}
	}
	return newPermissionSet(kept)
}

// consentFor reports whether one permission is covered, either because the user ticked the box that
// governs it or because it is not elevated and installing confers it.
//
// Each elevated permission is claimed by exactly one box. Two boxes claiming one permission would
// let a user decline it in one place and be granted it from the other, which is why a test walks the
// boxes and refuses any overlap.
func consentFor(token string, consent ConsentFlags) bool {
	switch {
	case strings.HasPrefix(token, permissionVaultGetSecret+":"):
		return consent.SecretAccess
	case token == PermissionAuthProvider, strings.HasPrefix(token, permissionAuthMethods+":"):
		return consent.AuthProvider
	case token == PermissionTunnelProvider:
		return consent.TunnelProvider
	case token == PermissionMultiSession:
		return consent.MultiSession
	// Private networks ride with arbitrary outbound. The manifest field means nothing on its own -
	// it widens the arbitrary grant rather than standing beside it - so the dialog never offers the
	// two separately and neither may this.
	case token == PermissionArbitraryOutbound, token == PermissionPrivateNetworks:
		return consent.ArbitraryNetwork
	// Only the exec templates are gated. The exec purpose itself is what the manifest declares in
	// order to be asked at all, and the other channel purposes carry no separate consent.
	case strings.HasPrefix(token, permissionExecCommands+":"):
		return consent.ExecChannel
	default:
		return true
	}
}
