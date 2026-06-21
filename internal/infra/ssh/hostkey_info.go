package ssh

import (
	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

// HostKeyInfoFromPublicKey fills KeyType, Fingerprint, KeyBase64 for UI display.
// Mismatch is left false; the caller sets it when resolving ErrHostKeyMismatch.
func HostKeyInfoFromPublicKey(host string, key gossh.PublicKey) domain.HostKeyInfo {
	if key == nil {
		return domain.HostKeyInfo{Host: host}
	}
	return domain.HostKeyInfo{
		Host:        host,
		KeyType:     key.Type(),
		Fingerprint: gossh.FingerprintSHA256(key),
		KeyBase64:   string(gossh.MarshalAuthorizedKey(key)),
	}
}
