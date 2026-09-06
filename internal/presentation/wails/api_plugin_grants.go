package wails

import domainplugin "xquakshell/internal/domain/plugin"

// The plugin runtime installs these hooks after composition, because the grant prompts they run
// live in the UI while the decision to ask belongs to the runtime. They are collected here rather
// than in api.go, which is about building and running AppAPI.

// SetPluginConsentRecorder sets the callback that records what the user agreed to when a plugin is
// installed.
//
// It replaced one hook per elevated capability. Five callbacks recorded five separate facts and
// none of them said what the plugin had actually been allowed, which is the question ADR-022 needs
// answered when the next version of that plugin asks for more.
func (a *AppAPI) SetPluginConsentRecorder(
	fn func(manifest *domainplugin.Manifest, consent domainplugin.ConsentFlags) error,
) {
	a.pluginConsentRecorder = fn
}

// SetPluginUnlockReconciler sets the callback that brings what the vault records about each plugin
// up to date: consent carried forward from the maps that preceded grants, and the scope folder for a
// plugin that declares one (ADR-022).
//
// It runs when the vault opens rather than when plugins are discovered, because discovery happens
// during composition, while the vault is still locked and nothing can be written to it. That is the
// same reason the interface language is broadcast from there and not at startup.
func (a *AppAPI) SetPluginUnlockReconciler(fn func()) {
	a.pluginUnlockReconciler = fn
}
