// Package keys converts SSH private keys between the shape the vault stores and the shapes the
// outside world uses.
//
// Everything the vault holds is an OpenSSH private key encrypted with bcrypt_pbkdf — the format
// ssh-keygen itself writes. There is deliberately no second, unprotected shape: an imported
// unprotected key is re-wrapped under a random data key held in the vault rather than stored as
// bare PEM, so a leak of the decrypted vault snapshot still does not hand over usable keys, and
// there is exactly one parse path to get wrong.
//
// The alternative considered was wrapping each key with the vault's own age+scrypt stack. It was
// rejected on cost: that stack is configured at scrypt log2N=18 (~256 MiB transient per call, see
// internal/infra/vault/vault.go), which a connection unwrapping three keys would pay three times.
// bcrypt_pbkdf is the primitive the SSH ecosystem already uses for exactly this job.
package keys

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

// Codec implements domain.KeyCodec.
type Codec struct{}

// NewCodec returns the SSH key codec used by the key manager.
func NewCodec() *Codec { return &Codec{} }

var _ domain.KeyCodec = (*Codec)(nil)

// ErrEmptyWrapPassphrase reports an attempt to store a key with no passphrase at all. It is a
// programming error rather than a user-facing one: the vault policy always supplies either the
// user's passphrase or a random data key, so an empty one means a caller skipped that step.
var ErrEmptyWrapPassphrase = errors.New("refusing to store a private key unprotected")

// Unwrap parses stored bytes into a signer.
//
// It goes through parseRaw rather than ssh.ParsePrivateKey so both entry points share one place
// where library errors are translated; ssh.ParsePrivateKey is itself just ParseRawPrivateKey
// followed by NewSignerFromKey, so nothing is lost by spelling it out.
func (c *Codec) Unwrap(pemData, passphrase []byte) (domain.Signer, error) {
	raw, err := parseRaw(pemData, passphrase)
	if err != nil {
		return nil, err
	}
	signer, err := gossh.NewSignerFromKey(raw)
	if err != nil {
		return nil, fmt.Errorf("build signer: %w", err)
	}
	return signer, nil
}

// Normalize re-wraps an imported key under passphrase, reading it with oldPassphrase.
func (c *Codec) Normalize(pemData, oldPassphrase, passphrase []byte, comment string) (*domain.KeyMaterial, error) {
	raw, err := parseRaw(pemData, oldPassphrase)
	if err != nil {
		return nil, err
	}
	return wrap(raw, passphrase, comment)
}

// Export re-wraps a stored key for writing outside the vault. An empty exportPassphrase produces
// an unprotected key, which is only ever reached through an explicit choice at the export screen.
func (c *Codec) Export(pemData, passphrase, exportPassphrase []byte, comment string) ([]byte, error) {
	raw, err := parseRaw(pemData, passphrase)
	if err != nil {
		return nil, err
	}
	var block *pem.Block
	if len(exportPassphrase) == 0 {
		block, err = gossh.MarshalPrivateKey(raw, comment)
	} else {
		block, err = gossh.MarshalPrivateKeyWithPassphrase(raw, comment, exportPassphrase)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal key for export: %w", err)
	}
	return pem.EncodeToMemory(block), nil
}

// Describe reads what can be learned from stored bytes without the passphrase.
//
// It reports the algorithm from the PEM header rather than from a parsed key, because the whole
// point is to answer for a key nobody has unlocked. An OpenSSH block hides the algorithm inside
// the encrypted section, so it reports the container instead of guessing.
func (c *Codec) Describe(pemData []byte) (keyType string, encrypted bool) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return "unknown", false
	}
	if _, err := gossh.ParsePrivateKey(pemData); err != nil {
		var missing *gossh.PassphraseMissingError
		encrypted = errors.As(err, &missing)
	}
	typeLower := strings.ToLower(block.Type)
	if !encrypted {
		encrypted = strings.Contains(typeLower, "encrypted") ||
			block.Headers["Proc-Type"] == "4,ENCRYPTED"
	}
	switch {
	case strings.Contains(typeLower, "rsa"):
		return domain.AlgorithmRSA, encrypted
	case strings.Contains(typeLower, "ec "), strings.Contains(typeLower, "ec p"), strings.HasPrefix(typeLower, "ec"):
		return domain.AlgorithmECDSA, encrypted
	case strings.Contains(typeLower, "openssh"):
		return describeOpenSSH(pemData, encrypted)
	default:
		return "unknown", encrypted
	}
}

// describeOpenSSH names the algorithm of an OpenSSH block, which is only readable when the key is
// unencrypted; an encrypted one keeps it inside the sealed section.
func describeOpenSSH(pemData []byte, encrypted bool) (string, bool) {
	if encrypted {
		return "openssh", true
	}
	signer, err := gossh.ParsePrivateKey(pemData)
	if err != nil {
		return "openssh", false
	}
	return shortKeyType(signer.PublicKey().Type()), false
}

// parseRaw reads a private key with or without a passphrase, mapping library errors onto the
// domain's two distinct cases so a caller can tell "you gave nothing" from "you gave the wrong
// thing" — only the second deserves to count against a retry limit.
func parseRaw(pemData, passphrase []byte) (crypto.PrivateKey, error) {
	if len(passphrase) == 0 {
		raw, err := gossh.ParseRawPrivateKey(pemData)
		if err != nil {
			return nil, translateParseError(err)
		}
		return raw, nil
	}
	raw, err := gossh.ParseRawPrivateKeyWithPassphrase(pemData, passphrase)
	if err != nil {
		return nil, translateParseError(err)
	}
	return raw, nil
}

func translateParseError(err error) error {
	var missing *gossh.PassphraseMissingError
	switch {
	case errors.As(err, &missing):
		return domain.ErrPassphraseRequired
	case errors.Is(err, x509.IncorrectPasswordError):
		// x/crypto returns this same sentinel for a bad bcrypt_pbkdf passphrase on an OpenSSH
		// key, not only for the PKCS#8 case its name suggests, so this one branch covers every
		// wrong-passphrase path the vault can produce.
		return domain.ErrKeyPassphraseWrong
	default:
		return fmt.Errorf("parse private key: %w", err)
	}
}

// wrap seals a parsed key under passphrase and derives the metadata the vault caches alongside it.
func wrap(raw crypto.PrivateKey, passphrase []byte, comment string) (*domain.KeyMaterial, error) {
	if len(passphrase) == 0 {
		return nil, ErrEmptyWrapPassphrase
	}
	block, err := gossh.MarshalPrivateKeyWithPassphrase(raw, comment, passphrase)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	signer, err := gossh.NewSignerFromKey(raw)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}
	pub := signer.PublicKey()
	return &domain.KeyMaterial{
		PEM:         pem.EncodeToMemory(block),
		PublicKey:   strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub))),
		Fingerprint: gossh.FingerprintSHA256(pub),
		KeyType:     shortKeyType(pub.Type()),
		Bits:        keyBits(raw),
	}, nil
}

// shortKeyType maps an SSH wire type ("ssh-ed25519", "ecdsa-sha2-nistp256") onto the algorithm
// names this vault has always stored, so schema 4 does not reword data schema 3 already holds.
func shortKeyType(wireType string) string {
	switch {
	case strings.Contains(wireType, "ed25519"):
		return domain.AlgorithmEd25519
	case strings.Contains(wireType, "ecdsa"):
		return domain.AlgorithmECDSA
	case strings.Contains(wireType, "rsa"):
		return domain.AlgorithmRSA
	default:
		return wireType
	}
}

// keyBits reports the key size where it varies. ed25519 has exactly one size, so it reports zero
// rather than 256: a UI showing "ed25519 256 bits" invites a comparison with RSA 256 that is
// meaningless.
func keyBits(raw crypto.PrivateKey) int {
	switch k := raw.(type) {
	case *rsa.PrivateKey:
		return k.N.BitLen()
	case *ecdsa.PrivateKey:
		return k.Curve.Params().BitSize
	case ed25519.PrivateKey, *ed25519.PrivateKey:
		return 0
	default:
		return 0
	}
}
