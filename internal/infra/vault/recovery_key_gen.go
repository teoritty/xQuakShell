package vault

import (
	"crypto/rand"
	"fmt"

	"xquakshell/internal/domain"
)

// NewRecoveryKey mints one recovery key from the system entropy source.
//
// There is deliberately no fallback path. Elsewhere in this codebase a random-id helper degrades to
// a fixed string when the entropy source fails, which is correct for a label nobody attacks; doing
// the same here would hand every installation on a broken machine the same key, and nothing about
// the vault would look wrong afterwards. An entropy failure is an error the user must see.
func NewRecoveryKey() (string, error) {
	entropy := make([]byte, domain.RecoveryKeyBytes)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("recovery key entropy: %w", err)
	}

	key := domain.EncodeRecoveryKey(entropy)
	if key == "" {
		return "", fmt.Errorf("recovery key encode produced nothing for %d bytes", len(entropy))
	}
	return key, nil
}
