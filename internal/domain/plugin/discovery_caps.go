package plugin

// DiscoveryCaps declares the discovery capability (ADR-014): the plugin may draw resource
// subtrees inside connections whose protocol appears in ParentProtocols.
type DiscoveryCaps struct {
	// ParentProtocols is not decorative: the host addresses discovery.observe only to plugins
	// whose list contains the protocol of the connection being expanded. A plugin that draws
	// docker containers under an SSH connection lists "ssh" here; it is never asked about a
	// connection protocol it didn't declare.
	ParentProtocols []string `json:"parentProtocols,omitempty"`
}

// DiscoveryIconContribution registers one icon asset a discovery plugin may reference by ID
// from a Node.IconID. Asset is validated and loaded once at install time (ValidateViewAssetEntry,
// like every other on-disk view asset) so the hot path (publishing nodes) never touches the
// filesystem — iconId is just a lookup key into an already-loaded set.
type DiscoveryIconContribution struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
}
