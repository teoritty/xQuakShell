package usecase

import "ssh-client/internal/domain"

// PluginAuthGrantReader reports install-time auth provider consent.
type PluginAuthGrantReader interface {
	IsAuthProviderGranted(pluginID string) bool
}

// SSHAuthWiring groups optional plugin-auth dependencies for SSHConnector.
type SSHAuthWiring struct {
	Attempts    *PluginAuthAttemptRegistry
	Provider    domain.PluginAuthProvider
	Builder     domain.PluginAuthMethodBuilder
	Lookup      PluginAuthMethodLookup
	Starter     PluginAuthStarter
	GrantReader PluginAuthGrantReader
}

// Enabled reports whether plugin SSH auth is wired.
func (w *SSHAuthWiring) Enabled() bool {
	return w != nil && w.Provider != nil && w.Builder != nil && w.Attempts != nil
}
