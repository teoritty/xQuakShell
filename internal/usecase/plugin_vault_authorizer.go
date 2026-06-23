package usecase

// PluginVaultAuthorizer checks plugin ownership of vault resources (IDOR).
type PluginVaultAuthorizer interface {
	PluginOwnsConnection(pluginID, connectionID string) bool
}
