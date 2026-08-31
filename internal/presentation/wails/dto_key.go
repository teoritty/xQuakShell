package wails

import (
	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// IdentityDTO is what the frontend sees of a stored key.
//
// There is no field for the private key and there is not going to be one. Export is a separate
// call returning bare bytes, so no listing, refresh or accidental log of this struct can carry
// key material — the type system is doing the work a review comment otherwise would.
type IdentityDTO struct {
	ID               string `json:"id"`
	Comment          string `json:"comment"`
	KeyType          string `json:"keyType"`
	Bits             int    `json:"bits,omitempty"`
	PublicKey        string `json:"publicKey,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
	Encrypted        bool   `json:"encrypted"`
	Policy           string `json:"policy,omitempty"`
	CachePolicy      string `json:"cachePolicy,omitempty"`
	CacheTTLSeconds  int    `json:"cacheTtlSeconds,omitempty"`
	AllowPlugins     bool   `json:"allowPlugins"`
	NonExportable    bool   `json:"nonExportable"`
	MigrationPending bool   `json:"migrationPending"`
	CreatedAt        string `json:"createdAt,omitempty"`
	Source           string `json:"source,omitempty"`
}

// KeyUsageDTO names one place a connection refers to a key.
type KeyUsageDTO struct {
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Username       string `json:"username"`
	Hop            string `json:"hop,omitempty"`
}

// KeyOptionsDTO carries the policy flags chosen in the UI.
type KeyOptionsDTO struct {
	CachePolicy     string `json:"cachePolicy"`
	CacheTTLSeconds int    `json:"cacheTtlSeconds"`
	AllowPlugins    bool   `json:"allowPlugins"`
	NonExportable   bool   `json:"nonExportable"`
}

// PendingKeyDTO names a key the schema migration still needs a passphrase for.
type PendingKeyDTO struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
	KeyType string `json:"keyType"`
}

// MigrationPlanDTO says whether an upgrade is due and what it will ask for.
type MigrationPlanDTO struct {
	Required bool            `json:"required"`
	Keys     []PendingKeyDTO `json:"keys"`
}

// MigrationReportDTO tells the user what the upgrade did, including which keys it could not
// finish, so a partial success is never presented as a complete one.
type MigrationReportDTO struct {
	FromVersion int      `json:"fromVersion"`
	ToVersion   int      `json:"toVersion"`
	Converted   []string `json:"converted"`
	Skipped     []string `json:"skipped"`
	BackupPath  string   `json:"backupPath"`
	// RecoveryKey carries a one-time key when this upgrade also gave the vault its first one. Empty
	// otherwise; it is never a key the caller may ask for again.
	RecoveryKey string `json:"recoveryKey,omitempty"`
}

// DeployResultDTO reports whether a key was added or was already authorised.
type DeployResultDTO struct {
	Added          bool   `json:"added"`
	AlreadyPresent bool   `json:"alreadyPresent"`
	Path           string `json:"path"`
}

// IdentityToDTO maps one identity outward.
func IdentityToDTO(in domain.SSHIdentity) IdentityDTO {
	dto := IdentityDTO{
		ID:               in.ID,
		Comment:          in.Comment,
		KeyType:          in.KeyType,
		Bits:             in.Bits,
		PublicKey:        in.PublicKey,
		Fingerprint:      in.Fingerprint,
		Encrypted:        in.Encrypted,
		Policy:           string(in.Policy),
		CachePolicy:      string(in.Cache),
		CacheTTLSeconds:  in.CacheTTLSeconds,
		AllowPlugins:     in.AllowPlugins,
		NonExportable:    in.NonExportable,
		MigrationPending: in.MigrationPending,
		Source:           in.Source,
	}
	if !in.CreatedAt.IsZero() {
		dto.CreatedAt = in.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return dto
}

// IdentitiesToDTO maps a list outward, always returning a non-nil slice so the frontend receives
// an empty array rather than null and does not have to guard every iteration.
func IdentitiesToDTO(in []domain.SSHIdentity) []IdentityDTO {
	out := make([]IdentityDTO, 0, len(in))
	for _, identity := range in {
		out = append(out, IdentityToDTO(identity))
	}
	return out
}

// KeyUsagesToDTO maps usage references outward.
func KeyUsagesToDTO(in []domain.KeyUsage) []KeyUsageDTO {
	out := make([]KeyUsageDTO, 0, len(in))
	for _, usage := range in {
		out = append(out, KeyUsageDTO{
			ConnectionID:   usage.ConnectionID,
			ConnectionName: usage.ConnectionName,
			Username:       usage.Username,
			Hop:            usage.Hop,
		})
	}
	return out
}

// DTOToKeyOptions maps policy flags inward, defaulting an unset cache policy to the behaviour the
// application had before the policy existed rather than to "never cache", which would silently
// start prompting the user on every connection.
func DTOToKeyOptions(in KeyOptionsDTO) usecase.KeyOptions {
	cache := domain.CachePolicy(in.CachePolicy)
	switch cache {
	case domain.CacheNever, domain.CacheDuration, domain.CacheUntilLock:
	default:
		cache = domain.CacheUntilLock
	}
	return usecase.KeyOptions{
		Cache:           cache,
		CacheTTLSeconds: in.CacheTTLSeconds,
		AllowPlugins:    in.AllowPlugins,
		NonExportable:   in.NonExportable,
	}
}
