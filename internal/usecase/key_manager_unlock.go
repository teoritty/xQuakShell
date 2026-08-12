package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"xquakshell/internal/domain"
)

// Signer turns a stored key into something that can authenticate, asking for a passphrase only
// when the key's policy needs one.
//
// This is the single door to usable key material. Every caller that wants to sign goes through
// here, which is what makes the per-key policy real: reaching the repository directly would get
// wrapped bytes and a data key the caller still has to know what to do with.
func (s *KeyManagerService) Signer(ctx context.Context, id string) (domain.Signer, error) {
	identity, blob, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !identity.NeedsUserPassphrase() {
		signer, err := s.codec.Unwrap(blob.PEMData, blob.DataKey)
		if err != nil {
			return nil, fmt.Errorf("unwrap key %s: %w", id, err)
		}
		return signer, nil
	}
	cached, ok := s.cachedPassphrase(id)
	if !ok {
		return nil, domain.ErrPassphraseRequired
	}
	return s.unwrapWithPassphrase(blob, cached)
}

// SignerWithPassphrase authenticates with a passphrase the user has just supplied and caches it
// according to the key's own policy.
func (s *KeyManagerService) SignerWithPassphrase(ctx context.Context, id, passphrase string) (domain.Signer, error) {
	identity, blob, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	signer, err := s.unwrapWithPassphrase(blob, passphrase)
	if err != nil {
		return nil, err
	}
	s.cachePassphrase(identity, passphrase)
	return signer, nil
}

func (s *KeyManagerService) unwrapWithPassphrase(blob *domain.IdentityBlob, passphrase string) (domain.Signer, error) {
	return s.codec.Unwrap(blob.PEMData, []byte(passphrase))
}

// cachePassphrase applies the identity's own retention policy. A key set to never cache stores
// nothing, so the next connection asks again.
func (s *KeyManagerService) cachePassphrase(identity *domain.SSHIdentity, passphrase string) {
	if s.cache == nil {
		return
	}
	ttl, bounded := identity.CacheTTL()
	if !bounded {
		s.cache.Set(identity.ID, passphrase)
		return
	}
	s.cache.SetWithTTL(identity.ID, passphrase, ttl)
}

func (s *KeyManagerService) cachedPassphrase(id string) (string, bool) {
	if s.cache == nil {
		return "", false
	}
	return s.cache.Get(id)
}

// PublicKey returns the authorized_keys line for a key, without unwrapping anything.
func (s *KeyManagerService) PublicKey(ctx context.Context, id string) (string, error) {
	identity, err := s.identities.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if identity.PublicKey == "" {
		// A key skipped during migration never had its public half derived, because deriving it
		// needs the passphrase that was not supplied.
		return "", fmt.Errorf("identity %s: %w", id, domain.ErrMigrationPending)
	}
	return identity.PublicKey, nil
}

// Export re-wraps a key for writing outside the vault.
//
// The caller must have re-authenticated: reAuthenticated is passed by the handler that verified
// the master password, and a false value is a refusal rather than a prompt, because deciding to
// prompt is the presentation layer's job and doing it here would let a caller skip it by not
// asking. exportPassphrase may be empty, which produces an unprotected key — the user's explicit
// choice at the export screen.
func (s *KeyManagerService) Export(ctx context.Context, id, passphrase, exportPassphrase string, reAuthenticated bool) ([]byte, error) {
	if !reAuthenticated {
		return nil, domain.ErrVaultLocked
	}
	identity, blob, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if identity.NonExportable {
		return nil, fmt.Errorf("identity %s: %w", id, domain.ErrKeyNotExportable)
	}
	if blob.Legacy {
		return nil, fmt.Errorf("identity %s: %w", id, domain.ErrMigrationPending)
	}

	out, err := s.codec.Export(blob.PEMData, s.openWith(identity, blob, passphrase), []byte(exportPassphrase), identity.Comment)
	if err != nil {
		return nil, err
	}
	s.record(ctx, KeyEventExported, id, identity.Fingerprint)
	return out, nil
}

// ChangePassphrase re-wraps a key under a new passphrase, or under a fresh vault data key when
// the new passphrase is empty.
//
// The old passphrase is forgotten before the new one is cached: leaving it would mean the next
// connection tries a passphrase that no longer opens the blob and reports it to the user as
// wrong, for a key they just successfully changed.
func (s *KeyManagerService) ChangePassphrase(ctx context.Context, id, oldPassphrase, newPassphrase string) error {
	identity, blob, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	wrapWith, dataKey, err := s.wrappingKey(newPassphrase)
	if err != nil {
		return err
	}
	material, err := s.codec.Normalize(blob.PEMData, s.openWith(identity, blob, oldPassphrase), wrapWith, identity.Comment)
	if err != nil {
		return err
	}

	updated := *identity
	updated.Policy = policyFor(newPassphrase)
	updated.Encrypted = newPassphrase != ""
	updated.MigrationPending = false
	updated.KeyType = material.KeyType
	updated.Bits = material.Bits
	updated.PublicKey = material.PublicKey
	updated.Fingerprint = material.Fingerprint

	if err := s.identities.Save(ctx, updated, domain.IdentityBlob{PEMData: material.PEM, DataKey: dataKey}); err != nil {
		return fmt.Errorf("save re-wrapped key: %w", err)
	}
	s.forget(id)
	if newPassphrase != "" {
		s.cachePassphrase(&updated, newPassphrase)
	}
	s.record(ctx, KeyEventPassphrase, id, updated.Fingerprint)
	return nil
}

// SetPolicy updates a key's caching, plugin-access and export flags.
//
// NonExportable is one-way. A key the user was told could never leave the vault must not become
// exportable by unticking a checkbox, or the promise was never worth anything; clearing it needs
// deleting the key and creating another.
func (s *KeyManagerService) SetPolicy(ctx context.Context, id string, opts KeyOptions) error {
	var fingerprint string
	err := s.identities.Update(ctx, id, func(identity *domain.SSHIdentity) error {
		if identity.NonExportable && !opts.NonExportable {
			return fmt.Errorf("identity %s: %w", id, domain.ErrKeyNotExportable)
		}
		identity.Cache = opts.Cache
		identity.CacheTTLSeconds = opts.CacheTTLSeconds
		identity.AllowPlugins = opts.AllowPlugins
		identity.NonExportable = opts.NonExportable
		fingerprint = identity.Fingerprint
		return nil
	})
	if err != nil {
		return err
	}
	if opts.Cache == domain.CacheNever {
		s.forget(id)
	}
	s.record(ctx, KeyEventPolicy, id, fingerprint)
	return nil
}

// Usages lists every connection that refers to a key, covering both a connection user's own key
// list and the hops of its jump chain. Missing the jump chain would let a key vanish out from
// under a bastion hop, which fails at connect time far from the action that caused it.
func (s *KeyManagerService) Usages(_ context.Context, id string) ([]domain.KeyUsage, error) {
	if s.vault == nil {
		return nil, nil
	}
	data, err := s.vault.GetData()
	if err != nil {
		return nil, err
	}
	usages := make([]domain.KeyUsage, 0)
	for i := range data.Connections {
		usages = append(usages, connectionUsages(&data.Connections[i], id)...)
	}
	return usages, nil
}

func connectionUsages(conn *domain.Connection, id string) []domain.KeyUsage {
	var out []domain.KeyUsage
	for _, user := range conn.Users {
		if user.KeyAuth != nil && slices.Contains(user.KeyAuth.IdentityIDs, id) {
			out = append(out, domain.KeyUsage{
				ConnectionID: conn.ID, ConnectionName: conn.Name, Username: user.Username,
			})
		}
	}
	for _, hop := range conn.JumpChain.Hops {
		if hop.KeyAuth != nil && slices.Contains(hop.KeyAuth.IdentityIDs, id) {
			out = append(out, domain.KeyUsage{
				ConnectionID: conn.ID, ConnectionName: conn.Name, Username: hop.Username, Hop: hop.Host,
			})
		}
	}
	return out
}

// openWith picks the secret that opens a stored blob: the vault's data key for a vault-policy
// key, the supplied passphrase otherwise.
func (s *KeyManagerService) openWith(identity *domain.SSHIdentity, blob *domain.IdentityBlob, passphrase string) []byte {
	if identity.NeedsUserPassphrase() || len(blob.DataKey) == 0 {
		return []byte(passphrase)
	}
	return blob.DataKey
}

func (s *KeyManagerService) load(ctx context.Context, id string) (*domain.SSHIdentity, *domain.IdentityBlob, error) {
	identity, err := s.identities.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	blob, err := s.identities.GetBlob(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			return nil, nil, fmt.Errorf("identity %s has metadata but no key bytes: %w", id, err)
		}
		return nil, nil, err
	}
	return identity, blob, nil
}
