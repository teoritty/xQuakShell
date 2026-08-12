package keys

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"xquakshell/internal/domain"
)

// Key generation limits. The RSA floor is 2048 because anything below it is rejected outright by
// current OpenSSH builds, so offering it would only produce keys that cannot log in. The ceiling
// is 4096 because generation time grows steeply past it for no practical gain and the UI would
// appear frozen for tens of seconds.
const (
	minRSABits     = 2048
	maxRSABits     = 4096
	defaultRSABits = 4096
)

// DefaultDataKeyBytes is the length of the random passphrase generated for a KeyPolicyVault key.
// At 32 bytes from crypto/rand the data key is not the weak link: bcrypt_pbkdf's cost exists to
// defend a human-chosen passphrase, and this one is never typed, displayed, or reused.
const DefaultDataKeyBytes = 32

// NewDataKey returns a random passphrase for wrapping a key whose policy is KeyPolicyVault.
func NewDataKey() ([]byte, error) {
	buf := make([]byte, DefaultDataKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate data key: %w", err)
	}
	return buf, nil
}

// Generate creates a new private key already wrapped under passphrase.
func (c *Codec) Generate(spec domain.GeneratedKeySpec, passphrase []byte) (*domain.KeyMaterial, error) {
	raw, err := generateRaw(spec)
	if err != nil {
		return nil, err
	}
	return wrap(raw, passphrase, spec.Comment)
}

func generateRaw(spec domain.GeneratedKeySpec) (crypto.PrivateKey, error) {
	switch spec.Algorithm {
	case domain.AlgorithmEd25519:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ed25519: %w", err)
		}
		return priv, nil
	case domain.AlgorithmRSA:
		return generateRSA(spec.Bits)
	case domain.AlgorithmECDSA:
		return generateECDSA(spec.Bits)
	default:
		return nil, fmt.Errorf("algorithm %q: %w", spec.Algorithm, domain.ErrUnsupportedKeyAlgorithm)
	}
}

func generateRSA(bits int) (crypto.PrivateKey, error) {
	if bits == 0 {
		bits = defaultRSABits
	}
	if bits < minRSABits || bits > maxRSABits {
		return nil, fmt.Errorf("rsa %d bits, want %d-%d: %w", bits, minRSABits, maxRSABits, domain.ErrUnsupportedKeyAlgorithm)
	}
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("generate rsa: %w", err)
	}
	return priv, nil
}

// generateECDSA maps a requested size onto a NIST curve. Only the three curves OpenSSH accepts
// are offered; a size that names no curve is rejected rather than rounded to the nearest one,
// because silently handing back a different key strength than asked for is the wrong answer.
func generateECDSA(bits int) (crypto.PrivateKey, error) {
	var curve elliptic.Curve
	switch bits {
	case 0, 256:
		curve = elliptic.P256()
	case 384:
		curve = elliptic.P384()
	case 521:
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("ecdsa %d bits, want 256, 384 or 521: %w", bits, domain.ErrUnsupportedKeyAlgorithm)
	}
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ecdsa: %w", err)
	}
	return priv, nil
}
