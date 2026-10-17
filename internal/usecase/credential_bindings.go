package usecase

import (
	"context"
	"errors"
	"slices"

	"xquakshell/internal/domain"
)

// ErrBindingTarget is a binding to a credential the personal vault does not hold.
var ErrBindingTarget = errors.New("bind a key or password from your own vault")

// CredentialBindingService manages this member's bindings of team connection users to their own
// credentials (spec §6.7). Bindings live in the personal vault, so they sync to this member's
// devices only, and the sync engine suspends them when another member repoints a connection.
type CredentialBindingService struct {
	vault domain.VaultRepository
}

// NewCredentialBindingService builds the service over the vault the bindings live in.
func NewCredentialBindingService(vault domain.VaultRepository) *CredentialBindingService {
	return &CredentialBindingService{vault: vault}
}

// Bindings lists this member's bindings, suspended ones included.
func (s *CredentialBindingService) Bindings() ([]domain.CredentialBinding, error) {
	data, err := s.vault.GetData()
	if err != nil || data.Sync == nil {
		return nil, err
	}
	return slices.Clone(data.Sync.Bindings), nil
}

// Bind ties one user of a team connection to exactly one credential of the personal vault,
// replacing an earlier binding of the same user; a new binding is in force.
func (s *CredentialBindingService) Bind(ctx context.Context, b domain.CredentialBinding) error {
	if b.VaultID == "" || b.ConnectionID == "" || b.UserID == "" || (b.IdentityID == "") == (b.PasswordID == "") {
		return ErrBindingTarget
	}
	b.SuspendedBy = ""
	return s.vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if !ownCredential(d, b) {
			return ErrBindingTarget
		}
		if d.Sync == nil {
			d.Sync = &domain.VaultSync{}
		}
		d.Sync.Bindings = slices.DeleteFunc(d.Sync.Bindings, func(x domain.CredentialBinding) bool { return x.Key() == b.Key() })
		d.Sync.Bindings = append(d.Sync.Bindings, b)
		return nil
	})
}

func ownCredential(d *domain.VaultData, b domain.CredentialBinding) bool {
	if b.IdentityID != "" {
		ident, ok := d.Identities[b.IdentityID]
		return ok && ident.VaultID == ""
	}
	pw, ok := d.Passwords[b.PasswordID]
	return ok && pw.VaultID == ""
}

// Confirm resumes a suspended binding once the member accepted the connection's new address.
func (s *CredentialBindingService) Confirm(ctx context.Context, vault, connection, user string) error {
	key := domain.CredentialBinding{VaultID: vault, ConnectionID: connection, UserID: user}.Key()
	return s.vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync != nil {
			for i := range d.Sync.Bindings {
				if d.Sync.Bindings[i].Key() == key {
					d.Sync.Bindings[i].SuspendedBy = ""
					return nil
				}
			}
		}
		return domain.ErrBindingMissing
	})
}
