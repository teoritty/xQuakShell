package domain

// SSHIdentity holds metadata for a private key stored in the vault.
// The actual key bytes are stored separately in VaultData.Identities.
type SSHIdentity struct {
	// ID is the unique identifier within the vault.
	ID string `json:"id"`
	// Comment is an optional human-readable label (e.g., original filename).
	Comment string `json:"comment"`
	// KeyType describes the algorithm (e.g., "rsa", "ecdsa", "ed25519").
	KeyType string `json:"keyType"`
	// Encrypted indicates whether the stored PEM requires a passphrase.
	Encrypted bool `json:"encrypted"`
}
