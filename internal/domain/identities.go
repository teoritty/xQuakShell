package domain

import "time"

// KeyPolicy decides where the passphrase that unwraps a stored private key comes from.
//
// Both policies store the key in exactly one on-disk shape: an OpenSSH private key encrypted
// with bcrypt_pbkdf, the same format ssh-keygen writes. Only the source of the passphrase
// differs. Keeping the shape uniform means there is a single parse path, a single export path
// and no branch where a key sits in the vault as unprotected PEM.
type KeyPolicy string

const (
	// KeyPolicyVault wraps the key under a random data key held in the vault, so unlocking the
	// vault is enough to use it. This is what an imported unprotected key becomes.
	KeyPolicyVault KeyPolicy = "vault"
	// KeyPolicyPassphrase wraps the key under a passphrase the user supplies. Nothing derived
	// from it is stored, so an unlocked vault alone does not reveal the key.
	KeyPolicyPassphrase KeyPolicy = "passphrase"
)

// CachePolicy decides how long a passphrase entered for a KeyPolicyPassphrase key stays in memory.
type CachePolicy string

const (
	// CacheNever discards the passphrase as soon as the signer is built, so every connection asks.
	CacheNever CachePolicy = "never"
	// CacheDuration keeps the passphrase for SSHIdentity.CacheTTLSeconds after it was last used.
	CacheDuration CachePolicy = "duration"
	// CacheUntilLock keeps the passphrase until the vault locks. This is what the application did
	// before caching became configurable, and it stays the default so behaviour does not regress.
	CacheUntilLock CachePolicy = "until-lock"
)

// DefaultCacheTTLSeconds is the passphrase lifetime applied when CacheDuration is chosen without
// an explicit TTL. Fifteen minutes is long enough to cover a burst of reconnects to the same host
// and short enough that an unattended machine stops being able to authenticate on its own.
const DefaultCacheTTLSeconds = 900

// SSHIdentity holds metadata for a private key stored in the vault. The wrapped key bytes live
// separately in VaultData.KeyBlobs, so listing, searching and usage lookups never touch them.
//
// PublicKey and Fingerprint are cached here for the same reason: the UI shows them constantly and
// deriving them on demand would mean unwrapping the private key to render a list.
type SSHIdentity struct {
	// ID is the unique identifier within the vault.
	ID string `json:"id"`
	// Comment is an optional human-readable label (e.g., original filename).
	Comment string `json:"comment"`
	// KeyType describes the algorithm (e.g., "rsa", "ecdsa", "ed25519").
	KeyType string `json:"keyType"`
	// Encrypted reports whether using the key needs a passphrase from the user. It is derived
	// from Policy and kept because the field predates it and the frontend already reads it.
	Encrypted bool `json:"encrypted"`

	// Bits is the key size for algorithms where it varies (RSA); zero for ed25519.
	Bits int `json:"bits,omitempty"`
	// PublicKey is the authorized_keys line for this identity, without a trailing comment.
	PublicKey string `json:"publicKey,omitempty"`
	// Fingerprint is the SHA256 fingerprint the user compares against the server.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Policy decides where the unwrapping passphrase comes from.
	Policy KeyPolicy `json:"policy,omitempty"`
	// Cache decides how long an entered passphrase is retained.
	Cache CachePolicy `json:"cachePolicy,omitempty"`
	// CacheTTLSeconds applies when Cache is CacheDuration.
	CacheTTLSeconds int `json:"cacheTtlSeconds,omitempty"`
	// AllowPlugins lets a plugin holding the vault capability read this private key. It is off
	// by default: before this flag existed every plugin could read every key.
	AllowPlugins bool `json:"allowPlugins,omitempty"`
	// NonExportable blocks export permanently. It can be set but never cleared — a key the user
	// was promised could not leave the vault must not become exportable by editing a checkbox.
	NonExportable bool `json:"nonExportable,omitempty"`
	// MigrationPending marks a key the v3 migration could not rewrite because its passphrase was
	// not supplied. Such a key still authenticates from its original bytes and is the only
	// reason the legacy blob path exists.
	MigrationPending bool `json:"migrationPending,omitempty"`
	// CreatedAt is when the identity entered the vault, generated or imported.
	CreatedAt time.Time `json:"createdAt,omitzero"`
	// Source records how it got here: "generated", "imported", "putty", "ssh-config".
	Source string `json:"source,omitempty"`
}

// Identity sources recorded in SSHIdentity.Source.
const (
	SourceGenerated = "generated"
	SourceImported  = "imported"
	SourcePuTTY     = "putty"
	SourceSSHConfig = "ssh-config"
)

// NeedsUserPassphrase reports whether using this key requires asking the user for a passphrase.
func (i *SSHIdentity) NeedsUserPassphrase() bool {
	return i.Policy == KeyPolicyPassphrase
}

// CacheTTL returns the passphrase lifetime for this identity, and whether one applies at all.
// A zero TTL under CacheDuration means the identity predates an explicit choice, so the default
// applies rather than "expire immediately" — the latter would silently turn into CacheNever.
func (i *SSHIdentity) CacheTTL() (ttl time.Duration, bounded bool) {
	switch i.Cache {
	case CacheNever:
		return 0, true
	case CacheDuration:
		secs := i.CacheTTLSeconds
		if secs <= 0 {
			secs = DefaultCacheTTLSeconds
		}
		return time.Duration(secs) * time.Second, true
	default:
		return 0, false
	}
}

// KeyUsage names one place a connection refers to an identity, so the UI can explain why a key
// cannot be deleted. Hop is empty for a connection user and set for a jump-chain hop.
type KeyUsage struct {
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Username       string `json:"username"`
	Hop            string `json:"hop,omitempty"`
}

// GeneratedKeySpec describes a key to create. Bits is ignored for algorithms with a fixed size.
type GeneratedKeySpec struct {
	Algorithm string
	Bits      int
	Comment   string
}

// The three algorithms this application will create. They double as the stored KeyType values,
// so the set is closed by what a v3 vault already holds and not only by what OpenSSH accepts:
// adding one here means every older build reads an identity whose type it cannot name.
const (
	AlgorithmEd25519 = "ed25519"
	AlgorithmRSA     = "rsa"
	AlgorithmECDSA   = "ecdsa"
)
