package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"xquakshell/internal/domain"
)

// KeyManagerAudit records key-management events. It is an interface rather than the audit
// repository itself so this service stays testable without a database, and so a build with
// auditing unavailable degrades to a nil recorder instead of failing key operations.
type KeyManagerAudit interface {
	RecordKeyEvent(ctx context.Context, event, identityID, fingerprint string)
}

// KeyManagerConfig holds the dependencies of KeyManagerService.
type KeyManagerConfig struct {
	Identities domain.IdentityRepository
	Vault      domain.VaultRepository
	Codec      domain.KeyCodec
	Cache      domain.PassphraseCache
	NewDataKey func() ([]byte, error)
	Audit      KeyManagerAudit
	Now        func() time.Time
}

// KeyManagerService owns every operation that reads or rewrites private key material.
//
// It is the only place allowed to turn a stored blob back into a usable key, so the rules about
// who may do that — the non-exportable flag, the plugin-access flag, re-authentication before an
// export — live in one place and cannot be bypassed by reaching for the repository directly.
type KeyManagerService struct {
	identities domain.IdentityRepository
	vault      domain.VaultRepository
	codec      domain.KeyCodec
	cache      domain.PassphraseCache
	newDataKey func() ([]byte, error)
	audit      KeyManagerAudit
	now        func() time.Time
}

// Key management audit event names, recorded through KeyManagerAudit.
const (
	KeyEventGenerated    = "key.generated"
	KeyEventImported     = "key.imported"
	KeyEventExported     = "key.exported"
	KeyEventDeleted      = "key.deleted"
	KeyEventPassphrase   = "key.passphrase-changed"
	KeyEventPolicy       = "key.policy-changed"
	KeyEventPluginAccess = "key.plugin-access"
	KeyEventDeployed     = "key.deployed"
)

// NewKeyManagerService wires the service. It panics on a missing required dependency for the same
// reason NewVaultService does: a nil repository here is a composition-root bug that would
// otherwise surface as a nil dereference during a key operation.
func NewKeyManagerService(cfg KeyManagerConfig) *KeyManagerService {
	if cfg.Identities == nil {
		panic("usecase: KeyManagerService requires IdentityRepository")
	}
	if cfg.Codec == nil {
		panic("usecase: KeyManagerService requires KeyCodec")
	}
	if cfg.NewDataKey == nil {
		panic("usecase: KeyManagerService requires a data key source")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &KeyManagerService{
		identities: cfg.Identities,
		vault:      cfg.Vault,
		codec:      cfg.Codec,
		cache:      cfg.Cache,
		newDataKey: cfg.NewDataKey,
		audit:      cfg.Audit,
		now:        now,
	}
}

// List returns metadata for every stored key, never the key material.
func (s *KeyManagerService) List(ctx context.Context) ([]domain.SSHIdentity, error) {
	return s.identities.GetAll(ctx)
}

// Generate creates a key inside the vault. A non-empty passphrase puts it under the user's
// control; an empty one wraps it under a random data key the vault holds.
func (s *KeyManagerService) Generate(ctx context.Context, spec domain.GeneratedKeySpec, passphrase string, opts KeyOptions) (*domain.SSHIdentity, error) {
	if strings.TrimSpace(spec.Comment) == "" {
		return nil, domain.ErrIdentityNameRequired
	}
	wrapWith, dataKey, err := s.wrappingKey(passphrase)
	if err != nil {
		return nil, err
	}
	material, err := s.codec.Generate(spec, wrapWith)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	identity := s.newIdentity(spec.Comment, domain.SourceGenerated, passphrase, material, opts)
	if err := s.identities.Save(ctx, identity, domain.IdentityBlob{PEMData: material.PEM, DataKey: dataKey}); err != nil {
		return nil, fmt.Errorf("save generated key: %w", err)
	}
	s.record(ctx, KeyEventGenerated, identity.ID, identity.Fingerprint)
	return &identity, nil
}

// Import stores a key the user supplied, normalising it into the vault's storage shape.
func (s *KeyManagerService) Import(ctx context.Context, pemData []byte, passphrase, comment string, opts KeyOptions) (*domain.SSHIdentity, error) {
	if strings.TrimSpace(comment) == "" {
		return nil, domain.ErrIdentityNameRequired
	}
	wrapWith, dataKey, err := s.wrappingKey(passphrase)
	if err != nil {
		return nil, err
	}
	material, err := s.codec.Normalize(pemData, []byte(passphrase), wrapWith, comment)
	if err != nil {
		return nil, err
	}

	identity := s.newIdentity(comment, domain.SourceImported, passphrase, material, opts)
	if err := s.identities.Save(ctx, identity, domain.IdentityBlob{PEMData: material.PEM, DataKey: dataKey}); err != nil {
		return nil, fmt.Errorf("save imported key: %w", err)
	}
	s.record(ctx, KeyEventImported, identity.ID, identity.Fingerprint)
	return &identity, nil
}

// Rename changes a key's label without touching its material.
func (s *KeyManagerService) Rename(ctx context.Context, id, comment string) error {
	if strings.TrimSpace(comment) == "" {
		return domain.ErrIdentityNameRequired
	}
	return s.identities.Update(ctx, id, func(identity *domain.SSHIdentity) error {
		identity.Comment = comment
		return nil
	})
}

// Delete removes a key, refusing while any connection still refers to it.
//
// The check and the delete are not one transaction, so a connection saved in the same instant can
// still slip through. That race is accepted: the window is a single user's own two actions in the
// same UI, and the cost of losing it is a connection pointing at a missing key, which already
// fails cleanly at connect time with ErrIdentityNotFound.
func (s *KeyManagerService) Delete(ctx context.Context, id string) error {
	usages, err := s.Usages(ctx, id)
	if err != nil {
		return err
	}
	if len(usages) > 0 {
		return fmt.Errorf("identity %s used by %d connection(s): %w", id, len(usages), domain.ErrIdentityInUse)
	}
	identity, err := s.identities.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.identities.Delete(ctx, id); err != nil {
		return err
	}
	s.forget(id)
	s.record(ctx, KeyEventDeleted, id, identity.Fingerprint)
	return nil
}

// KeyOptions carries the policy flags chosen when a key is created or imported.
type KeyOptions struct {
	Cache           domain.CachePolicy
	CacheTTLSeconds int
	AllowPlugins    bool
	NonExportable   bool
}

// newIdentity assembles the metadata for a freshly wrapped key.
func (s *KeyManagerService) newIdentity(comment, source, passphrase string, material *domain.KeyMaterial, opts KeyOptions) domain.SSHIdentity {
	cache := opts.Cache
	if cache == "" {
		cache = domain.CacheUntilLock
	}
	return domain.SSHIdentity{
		ID:              newIdentityID(),
		Comment:         comment,
		KeyType:         material.KeyType,
		Bits:            material.Bits,
		PublicKey:       material.PublicKey,
		Fingerprint:     material.Fingerprint,
		Encrypted:       passphrase != "",
		Policy:          policyFor(passphrase),
		Cache:           cache,
		CacheTTLSeconds: opts.CacheTTLSeconds,
		AllowPlugins:    opts.AllowPlugins,
		NonExportable:   opts.NonExportable,
		CreatedAt:       s.now(),
		Source:          source,
	}
}

// wrappingKey returns what a new key should be wrapped under, and the data key to store beside it.
// A user passphrase is never stored, so the second result is nil in that case.
func (s *KeyManagerService) wrappingKey(passphrase string) (wrapWith, storedDataKey []byte, err error) {
	if passphrase != "" {
		return []byte(passphrase), nil, nil
	}
	dataKey, err := s.newDataKey()
	if err != nil {
		return nil, nil, fmt.Errorf("new data key: %w", err)
	}
	return dataKey, dataKey, nil
}

func policyFor(passphrase string) domain.KeyPolicy {
	if passphrase != "" {
		return domain.KeyPolicyPassphrase
	}
	return domain.KeyPolicyVault
}

func (s *KeyManagerService) record(ctx context.Context, event, id, fingerprint string) {
	if s.audit == nil {
		return
	}
	s.audit.RecordKeyEvent(ctx, event, id, fingerprint)
}

func (s *KeyManagerService) forget(id string) {
	if s.cache != nil {
		s.cache.Forget(id)
	}
}

// newIdentityID mints a vault-unique key id.
//
// It uses crypto/rand directly rather than the uuid package the persistence layer uses, because
// the usecase layer may not import third-party code. 128 random bits is the same collision
// resistance a v4 UUID offers, which is what the existing ids are.
func newIdentityID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand.Read never returns an error on any supported platform; if it somehow did,
		// continuing with a predictable id would be worse than stopping.
		panic("usecase: no entropy for a key id: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
