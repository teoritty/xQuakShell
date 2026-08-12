package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"xquakshell/internal/domain"
)

// IdentityRepo implements domain.IdentityRepository backed by the vault.
type IdentityRepo struct {
	vault   domain.VaultRepository
	codec   domain.KeyCodec
	newKey  func() ([]byte, error)
	nowFunc func() time.Time
}

// NewIdentityRepo creates an IdentityRepo backed by the given VaultRepository.
//
// The codec and data-key source are injected rather than imported so this package keeps its one
// dependency on the domain, and so a test can store a key without paying bcrypt_pbkdf's cost.
func NewIdentityRepo(v domain.VaultRepository, codec domain.KeyCodec, newDataKey func() ([]byte, error)) *IdentityRepo {
	return &IdentityRepo{vault: v, codec: codec, newKey: newDataKey, nowFunc: time.Now}
}

// GetAll returns metadata for every SSH identity in the vault.
func (r *IdentityRepo) GetAll(_ context.Context) ([]domain.SSHIdentity, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get all identities: %w", err)
	}

	result := make([]domain.SSHIdentity, 0, len(data.Identities))
	for _, id := range data.Identities {
		result = append(result, id)
	}
	return result, nil
}

// Get returns one identity's metadata, and never its key bytes — callers that only need to show
// or check a key's properties must not have to hold its private material to do so.
func (r *IdentityRepo) Get(_ context.Context, id string) (*domain.SSHIdentity, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get identity: %w", err)
	}
	identity, ok := data.Identities[id]
	if !ok {
		return nil, fmt.Errorf("identity %s: %w", id, domain.ErrIdentityNotFound)
	}
	return &identity, nil
}

// GetKeyBlob returns the stored key bytes for the identity with the given ID.
//
// The bytes alone do not open the key: under either policy they are an encrypted OpenSSH key,
// and the caller still needs the data key from GetBlob or the user's passphrase.
func (r *IdentityRepo) GetKeyBlob(ctx context.Context, id string) ([]byte, error) {
	blob, err := r.GetBlob(ctx, id)
	if err != nil {
		return nil, err
	}
	return blob.PEMData, nil
}

// GetBlob returns the stored key together with the data key that unwraps it, when the identity's
// policy keeps one.
func (r *IdentityRepo) GetBlob(_ context.Context, id string) (*domain.IdentityBlob, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get key blob: %w", err)
	}

	blob, ok := data.KeyBlobs[id]
	if !ok {
		return nil, fmt.Errorf("identity %s: %w", id, domain.ErrIdentityNotFound)
	}
	return &domain.IdentityBlob{
		PEMData: append([]byte(nil), blob.PEMData...),
		DataKey: append([]byte(nil), blob.DataKey...),
		Legacy:  blob.Legacy,
	}, nil
}

// Import stores a new SSH identity in the vault and returns its metadata.
//
// An unprotected key is re-wrapped under a fresh random data key, so importing a bare ~/.ssh/id_*
// does not leave a usable private key sitting in the vault snapshot. A key that already carries
// its own passphrase is stored as it arrived: re-wrapping it would need that passphrase, which an
// import does not ask for, and the key is already protected by it. Such a blob is marked Legacy
// and is normalised the first time the user unlocks it.
func (r *IdentityRepo) Import(ctx context.Context, pemData []byte, comment string) (*domain.SSHIdentity, error) {
	keyType, encrypted := r.codec.Describe(pemData)
	identity := domain.SSHIdentity{
		ID:        uuid.New().String(),
		Comment:   comment,
		KeyType:   keyType,
		Encrypted: encrypted,
		Cache:     domain.CacheUntilLock,
		CreatedAt: r.nowFunc(),
		Source:    domain.SourceImported,
	}

	blob, err := r.wrapImport(pemData, comment, encrypted, &identity)
	if err != nil {
		return nil, err
	}
	if err := r.Save(ctx, identity, blob); err != nil {
		return nil, err
	}
	return &identity, nil
}

// wrapImport decides the stored shape of an imported key and fills in the metadata that depends
// on it. It never fails an import that the application could otherwise complete: a key it cannot
// re-wrap is kept verbatim rather than rejected.
func (r *IdentityRepo) wrapImport(pemData []byte, comment string, encrypted bool, identity *domain.SSHIdentity) (domain.IdentityBlob, error) {
	if encrypted {
		identity.Policy = domain.KeyPolicyPassphrase
		return domain.IdentityBlob{PEMData: append([]byte(nil), pemData...), Legacy: true}, nil
	}

	dataKey, err := r.newKey()
	if err != nil {
		return domain.IdentityBlob{}, fmt.Errorf("import identity: %w", err)
	}
	material, err := r.codec.Normalize(pemData, nil, dataKey, comment)
	if err != nil {
		if errors.Is(err, domain.ErrPassphraseRequired) {
			// Describe read the container as unprotected but the key turned out to need a
			// passphrase after all — a shape it could not see into. Keeping the original bytes
			// is right: the key still works, and the alternative is refusing a valid import.
			identity.Policy = domain.KeyPolicyPassphrase
			identity.Encrypted = true
			return domain.IdentityBlob{PEMData: append([]byte(nil), pemData...), Legacy: true}, nil
		}
		return domain.IdentityBlob{}, fmt.Errorf("import identity: %w", err)
	}

	identity.Policy = domain.KeyPolicyVault
	identity.Encrypted = false
	identity.KeyType = material.KeyType
	identity.Bits = material.Bits
	identity.PublicKey = material.PublicKey
	identity.Fingerprint = material.Fingerprint
	return domain.IdentityBlob{PEMData: material.PEM, DataKey: dataKey}, nil
}

// Save writes an identity and its key bytes, replacing any entry with the same ID.
func (r *IdentityRepo) Save(ctx context.Context, identity domain.SSHIdentity, blob domain.IdentityBlob) error {
	if identity.ID == "" {
		return fmt.Errorf("save identity: %w", domain.ErrIdentityNotFound)
	}
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		data.Identities[identity.ID] = identity
		data.KeyBlobs[identity.ID] = blob
		return nil
	})
}

// Update applies mutate to one identity's metadata inside the vault's own transaction, so a
// concurrent write cannot land between reading the identity and storing the changed copy.
func (r *IdentityRepo) Update(ctx context.Context, id string, mutate func(*domain.SSHIdentity) error) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		identity, ok := data.Identities[id]
		if !ok {
			return fmt.Errorf("identity %s: %w", id, domain.ErrIdentityNotFound)
		}
		if err := mutate(&identity); err != nil {
			return err
		}
		data.Identities[id] = identity
		return nil
	})
}

// Delete removes an identity by ID from the vault.
func (r *IdentityRepo) Delete(ctx context.Context, id string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		delete(data.Identities, id)
		delete(data.KeyBlobs, id)
		return nil
	})
}
