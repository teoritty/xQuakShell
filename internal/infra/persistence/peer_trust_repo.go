package persistence

import (
	"context"
	"fmt"
	"time"

	"xquakshell/internal/domain"
)

// PeerTrustRepo stores what plugins trust about the identity of a remote peer.
//
// A type separate from KnownHostsRepo on purpose, and not as cosmetics: one shared list would mean
// a plugin writes where SSH reads its trust from. A different structure, a different vault field
// and a different port - there is no path from a plugin to VaultData.KnownHosts.
type PeerTrustRepo struct {
	vault domain.VaultRepository
}

// NewPeerTrustRepo creates a PeerTrustRepo backed by the given VaultRepository.
func NewPeerTrustRepo(v domain.VaultRepository) *PeerTrustRepo {
	return &PeerTrustRepo{vault: v}
}

var _ domain.PeerTrustRepository = (*PeerTrustRepo)(nil)

// Find returns the entry, or nil.
//
// A missing entry is not an error: it is an ordinary first connection. Turning it into one would
// force every caller to tell "not known" apart from "storage broke", and sooner or later one of
// them would stop.
func (r *PeerTrustRepo) Find(scope, subject string) (*domain.PeerTrustEntry, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("peer trust find: %w", err)
	}
	for i := range data.PeerTrust {
		if data.PeerTrust[i].Scope == scope && data.PeerTrust[i].Subject == subject {
			entry := data.PeerTrust[i]
			return &entry, nil
		}
	}
	return nil, nil
}

// Put creates or replaces the entry for the (scope, subject) pair.
//
// Replaces, precisely. Two entries for one pair would mean either material counts as trusted - and
// a key rotation on the server would stop meaning anything, because the old one would stay
// acceptable.
func (r *PeerTrustRepo) Put(ctx context.Context, entry domain.PeerTrustEntry) error {
	if len(entry.Material) == 0 {
		return domain.ErrPeerMaterialEmpty
	}
	if len(entry.Material) > domain.MaxPeerTrustMaterial {
		return domain.ErrPeerMaterialTooLarge
	}
	if entry.AddedAt.IsZero() {
		entry.AddedAt = time.Now().UTC()
	}
	// A copy of the material: the caller is entitled to reuse its buffer, and the entry
	// outlives this call.
	entry.Material = append([]byte(nil), entry.Material...)

	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		kept := make([]domain.PeerTrustEntry, 0, len(data.PeerTrust)+1)
		for _, e := range data.PeerTrust {
			if e.Scope == entry.Scope && e.Subject == entry.Subject {
				continue
			}
			kept = append(kept, e)
		}
		data.PeerTrust = append(kept, entry)
		return nil
	})
}

// List returns every entry, for the management screen to show.
func (r *PeerTrustRepo) List() ([]domain.PeerTrustEntry, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("peer trust list: %w", err)
	}
	out := make([]domain.PeerTrustEntry, len(data.PeerTrust))
	copy(out, data.PeerTrust)
	return out, nil
}

// Remove is idempotent: deleting an entry that is not there is not an error.
func (r *PeerTrustRepo) Remove(ctx context.Context, scope, subject string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		kept := make([]domain.PeerTrustEntry, 0, len(data.PeerTrust))
		for _, e := range data.PeerTrust {
			if e.Scope == scope && e.Subject == subject {
				continue
			}
			kept = append(kept, e)
		}
		data.PeerTrust = kept
		return nil
	})
}
