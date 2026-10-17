package usecase

import (
	"context"
	"errors"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

// ErrConflictNotFound is a conflict that was already resolved, here or on another device.
var ErrConflictNotFound = errors.New("sync: that conflict is already resolved")

// SyncConflictService lets a person settle the credential conflicts two devices produced.
type SyncConflictService struct {
	vault domain.VaultRepository
}

// NewSyncConflictService works on the vault the rest of the app writes through, so a resolution is
// a local edit like any other and the next sync carries it to every device.
func NewSyncConflictService(vault domain.VaultRepository) *SyncConflictService {
	return &SyncConflictService{vault: vault}
}

// List returns the unresolved conflicts, the ones synced from other devices included.
func (s *SyncConflictService) List() ([]domain.SyncConflict, error) {
	data, err := s.vault.GetData()
	if err != nil || data.Sync == nil {
		return nil, err
	}
	return data.Sync.Conflicts, nil
}

// DescribedConflict is a conflict with the names a person recognises: the item's, and one name
// per path segment for the elements it runs through (a connection user by its login).
type DescribedConflict struct {
	domain.SyncConflict
	ItemName  string
	PathNames []string
}

// Described lists the unresolved conflicts, each named against the vault as it is now.
func (s *SyncConflictService) Described() ([]DescribedConflict, error) {
	data, err := s.vault.GetData()
	if err != nil || data.Sync == nil {
		return nil, err
	}
	out := make([]DescribedConflict, 0, len(data.Sync.Conflicts))
	for _, c := range data.Sync.Conflicts {
		t := synctrust.RecordType(c.RecordType)
		out = append(out, DescribedConflict{SyncConflict: c, ItemName: synctrust.DescribeItem(data, t, c.ItemKey),
			PathNames: synctrust.DescribePath(data, t, c.ItemKey, c.Path)})
	}
	return out, nil
}

// Resolve settles one conflict. useOther writes the value that lost the merge over the one in
// force; otherwise the value in force stays. Either way the conflict is removed in the same vault
// transaction, so an item never changes without its conflict going with it.
func (s *SyncConflictService) Resolve(ctx context.Context, id string, useOther bool) error {
	return s.vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil {
			return ErrConflictNotFound
		}
		i := slices.IndexFunc(d.Sync.Conflicts, func(c domain.SyncConflict) bool { return c.ID == id })
		if i < 0 {
			return ErrConflictNotFound
		}
		c := d.Sync.Conflicts[i]
		if useOther {
			if err := synctrust.SetField(d, synctrust.RecordType(c.RecordType), c.ItemKey, c.Path, c.Local); err != nil {
				return err
			}
		}
		d.Sync.Conflicts = slices.Delete(d.Sync.Conflicts, i, i+1)
		return nil
	})
}
