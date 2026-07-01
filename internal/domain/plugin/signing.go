package plugin

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	// SignatureScheme identifies Ed25519 manifest signatures (Phase 6).
	SignatureScheme = "ed25519"

	// ChecksumsSHA256Len is the expected length of a hex-encoded SHA-256 checksums digest.
	ChecksumsSHA256Len = sha256.Size * 2
)

// ErrSignatureFormatOutdated is returned when a signature was created with the old
// manifest-only format (without checksums binding).
var ErrSignatureFormatOutdated = errors.New("signature format outdated, plugin must be re-signed with checksums binding")

// ErrMissingChecksumsDigest indicates a signed manifest was verified without a valid checksums digest.
var ErrMissingChecksumsDigest = errors.New("missing checksums digest for signature verification")

type manifestSigningEnvelope struct {
	Manifest        Manifest `json:"manifest"`
	ChecksumsSHA256 string   `json:"checksumsSha256"`
}

// ManifestSigningPayload returns canonical JSON bytes for the signing envelope.
func ManifestSigningPayload(m Manifest, checksumsSHA256 string) ([]byte, error) {
	if len(checksumsSHA256) != ChecksumsSHA256Len {
		return nil, fmt.Errorf("%w: checksumsSha256 must be %d hex characters", ErrInvalidManifest, ChecksumsSHA256Len)
	}
	copy := m
	copy.Signature = ""
	env := manifestSigningEnvelope{
		Manifest:        copy,
		ChecksumsSHA256: checksumsSHA256,
	}
	return canonicalJSON(env)
}

func manifestSigningPayloadOld(m Manifest) ([]byte, error) {
	copy := m
	copy.Signature = ""
	return json.Marshal(copy)
}

// SignManifest attaches a base64 Ed25519 signature for the canonical signing envelope.
func SignManifest(m Manifest, checksumsSHA256 string, privateKey ed25519.PrivateKey) (string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("%w: invalid ed25519 private key", ErrInvalidManifest)
	}
	payload, err := ManifestSigningPayload(m, checksumsSHA256)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(privateKey, payload)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyManifestSignature checks the manifest signature against trusted publisher keys.
func VerifyManifestSignature(m Manifest, checksumsSHA256 string, trustedKeys []ed25519.PublicKey) (bool, error) {
	if m.Signature == "" {
		return false, nil
	}
	if len(checksumsSHA256) != ChecksumsSHA256Len {
		return false, ErrMissingChecksumsDigest
	}
	sig, err := decodeManifestSignature(m.Signature)
	if err != nil {
		return false, err
	}

	payload, err := ManifestSigningPayload(m, checksumsSHA256)
	if err != nil {
		return false, err
	}
	if verifySignatureWithKeys(trustedKeys, payload, sig) {
		return true, nil
	}

	oldPayload, err := manifestSigningPayloadOld(m)
	if err != nil {
		return false, err
	}
	if verifySignatureWithKeys(trustedKeys, oldPayload, sig) {
		return false, ErrSignatureFormatOutdated
	}
	return false, nil
}

func decodeManifestSignature(encoded string) ([]byte, error) {
	sig, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: decode signature: %v", ErrInvalidManifest, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: invalid signature length", ErrInvalidManifest)
	}
	return sig, nil
}

func verifySignatureWithKeys(trustedKeys []ed25519.PublicKey, payload, sig []byte) bool {
	for _, pub := range trustedKeys {
		if len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, payload, sig) {
			return true
		}
	}
	return false
}

// canonicalJSON marshals v to JSON with sorted map keys for deterministic output.
func canonicalJSON(v interface{}) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return json.Marshal(sortJSONValue(decoded))
}

func sortJSONValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(val))
		for _, k := range keys {
			out[k] = sortJSONValue(val[k])
		}
		return out
	case []interface{}:
		for i, item := range val {
			val[i] = sortJSONValue(item)
		}
		return val
	default:
		return v
	}
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
