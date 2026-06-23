package plugin

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// SignatureScheme identifies Ed25519 manifest signatures (Phase 6).
	SignatureScheme = "ed25519"
)

// ManifestSigningPayload returns canonical JSON bytes used for signature verification.
func ManifestSigningPayload(m Manifest) ([]byte, error) {
	copy := m
	copy.Signature = ""
	return json.Marshal(copy)
}

// SignManifest attaches a base64 Ed25519 signature for the canonical manifest payload.
func SignManifest(m Manifest, privateKey ed25519.PrivateKey) (string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("%w: invalid ed25519 private key", ErrInvalidManifest)
	}
	payload, err := ManifestSigningPayload(m)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(privateKey, payload)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyManifestSignature checks the manifest signature against trusted publisher keys.
func VerifyManifestSignature(m Manifest, trustedKeys []ed25519.PublicKey) (ok bool, err error) {
	if m.Signature == "" {
		return false, nil
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return false, fmt.Errorf("%w: decode signature: %v", ErrInvalidManifest, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("%w: invalid signature length", ErrInvalidManifest)
	}
	payload, err := ManifestSigningPayload(m)
	if err != nil {
		return false, err
	}
	for _, pub := range trustedKeys {
		if len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, payload, sig) {
			return true, nil
		}
	}
	return false, nil
}

// ParseTrustedPublisherKeys decodes base64 Ed25519 public keys from settings.
func ParseTrustedPublisherKeys(raw []string) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey
	for _, s := range raw {
		if s == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid trusted publisher key: %v", ErrInvalidManifest, err)
		}
		if len(data) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: trusted publisher key must be %d bytes", ErrInvalidManifest, ed25519.PublicKeySize)
		}
		keys = append(keys, ed25519.PublicKey(data))
	}
	return keys, nil
}

// EncodePublicKey returns base64 encoding of an Ed25519 public key.
func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

// EncodePrivateKey returns base64 encoding of an Ed25519 private key.
func EncodePrivateKey(priv ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(priv)
}

// ParsePrivateKey decodes a base64 Ed25519 private key.
func ParsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("%w: decode private key: %v", ErrInvalidManifest, err)
	}
	if len(data) != ed25519.PrivateKeySize && len(data) != ed25519.SeedSize {
		return nil, fmt.Errorf("%w: private key must be %d or %d bytes", ErrInvalidManifest, ed25519.PrivateKeySize, ed25519.SeedSize)
	}
	if len(data) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(data), nil
	}
	return ed25519.PrivateKey(data), nil
}
