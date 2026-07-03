package domain

// ConnectionProtocolLookup resolves plugin-contributed connection protocol metadata.
type ConnectionProtocolLookup interface {
	DefaultPortForProtocol(protocol string) (int, bool)
}
