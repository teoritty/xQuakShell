package domain

// PluginTrustWeakened reports whether moving the plugin settings from old to next
// lowers the bar a plugin has to clear before it runs.
//
// The plugin trust anchor lives in PluginSettings and nowhere else: TrustedPublisherKeys
// IS the root of trust for manifest signatures, RequireSignedPlugins decides whether an
// unsigned plugin installs at all, AllowUnsandboxedFallback decides whether a plugin may
// start unconfined, and the grant maps decide what an installed plugin may reach. Every
// one of those reaches the vault through a Wails binding, so anything that gets hold of
// window.go - a cross-site script in the UI, a plugin WebView - can rewrite the anchor and
// then present its own plugin as signed by a trusted publisher.
//
// The asymmetry is deliberate and is the whole point: revoking a key, turning the signature
// requirement on, disabling a plugin, taking a grant away all return false. You never need
// to prove who you are in order to become safer, and demanding a password to lock something
// down is how a security control gets switched off and left off. Only the direction that
// buys an attacker something is gated.
//
// Callers use this to decide whether a save needs the master password re-entered; the
// master password is the one secret that never crosses the Wails bridge, so it is what
// separates the user sitting at the machine from code running in the WebView.
func PluginTrustWeakened(old, next PluginSettings) bool {
	if old.RequireSignedPlugins && !next.RequireSignedPlugins {
		return true
	}
	if !old.AllowUnsandboxedFallback && next.AllowUnsandboxedFallback {
		return true
	}
	if hasNewTrustedKey(old.TrustedPublisherKeys, next.TrustedPublisherKeys) {
		return true
	}
	return grantsWidened(old, next)
}

// hasNewTrustedKey reports whether next introduces a publisher key old did not carry.
// Reordering the list is not a change, and dropping a key is a revocation.
func hasNewTrustedKey(old, next []string) bool {
	known := make(map[string]struct{}, len(old))
	for _, key := range old {
		known[key] = struct{}{}
	}
	for _, key := range next {
		if _, ok := known[key]; !ok {
			return true
		}
	}
	return false
}

// grantsWidened reports whether any per-plugin capability grant turned on.
//
// Absent and false are the same thing here - a plugin with no entry has no grant - so a map
// that gains an explicit false is not a widening, and one that loses a true entry is a
// revocation.
func grantsWidened(old, next PluginSettings) bool {
	pairs := []struct {
		old  map[string]bool
		next map[string]bool
	}{
		{old.SecretAccessGranted, next.SecretAccessGranted},
		{old.AuthProviderAccessGranted, next.AuthProviderAccessGranted},
		{old.TunnelProviderAccessGranted, next.TunnelProviderAccessGranted},
		{old.MultiSessionAccessGranted, next.MultiSessionAccessGranted},
		{old.ArbitraryNetworkAccessGranted, next.ArbitraryNetworkAccessGranted},
	}
	for _, pair := range pairs {
		for pluginID, granted := range pair.next {
			if granted && !pair.old[pluginID] {
				return true
			}
		}
	}
	return false
}
