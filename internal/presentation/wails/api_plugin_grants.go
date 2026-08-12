package wails

// The plugin runtime installs these hooks after composition, because the grant prompts they run
// live in the UI while the decision to ask belongs to the runtime. They are collected here rather
// than in api.go, which is about building and running AppAPI.

// SetPluginVaultGrant sets the callback used after install to record secret consent.
func (a *AppAPI) SetPluginVaultGrant(fn func(pluginID string) error) {
	a.pluginVaultGrant = fn
}

// SetPluginAuthGrant sets the callback used after install to record auth provider consent.
func (a *AppAPI) SetPluginAuthGrant(fn func(pluginID string) error) {
	a.pluginAuthGrant = fn
}

// SetPluginTunnelGrant sets the callback used after install to record tunnel provider consent.
func (a *AppAPI) SetPluginTunnelGrant(fn func(pluginID string) error) {
	a.pluginTunnelGrant = fn
}

// SetPluginMultiSessionGrant sets the callback used after install to record multi-session consent.
func (a *AppAPI) SetPluginMultiSessionGrant(fn func(pluginID string) error) {
	a.pluginMultiSessionGrant = fn
}

// SetPluginArbitraryNetworkGrant sets the callback used after install to record arbitrary network consent.
func (a *AppAPI) SetPluginArbitraryNetworkGrant(fn func(pluginID string) error) {
	a.pluginArbitraryNetworkGrant = fn
}
