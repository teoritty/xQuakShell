package usecase

import (
	"context"
	"errors"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrHeldNotFound is a held change that was already confirmed, rejected or superseded.
	ErrHeldNotFound = errors.New("sync: that change is no longer waiting")
	// ErrHeldStale is a held change a newer version replaced; the newer one is what to review.
	ErrHeldStale = errors.New("sync: a newer version of that item has arrived")
)

// HeldChange is a record waiting in quarantine, as the user sees it. Type and Key are empty when
// the record does not open on this device.
type HeldChange struct {
	// Vault is the vault the change arrived in: the personal vault or a team's.
	Vault  vaultsync.ID
	Record vaultsync.ID
	Type   synctrust.RecordType
	Key    string
	// Name is what the person called the item, empty for a settings section or an unreadable item.
	Name        string
	Reason      string
	Pending     bool
	AtUnixMilli int64
}

// DeviceInfo is one device of the account, from the verified device log.
type DeviceInfo struct {
	Cert    vaultsync.DeviceCert
	Revoked bool
	Current bool
}

// HeldChanges lists the records a pull held for the master password or refused. The quarantine is
// read first, from the state store: a vault with nothing waiting costs no request to the server.
func (e *SyncEngine) HeldChanges(ctx context.Context, vault vaultsync.ID) ([]HeldChange, error) {
	items, err := e.d.State.Quarantined(ctx, vault)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return nil, err
	}
	out := make([]HeldChange, 0, len(items))
	for _, it := range items {
		h := HeldChange{Vault: vault, Record: it.Envelope.Record, Reason: it.Reason, Pending: it.Pending, AtUnixMilli: it.AtUnixMilli}
		if body, err := p.open(&it.Envelope); err == nil {
			h.Type = body.Type
			h.Key, _ = synctrust.NaturalKey(body.Type, body.Data)
			h.Name = synctrust.Describe(body.Type, body.Data)
		}
		out = append(out, h)
	}
	return out, nil
}

// ConfirmHeld applies a change that was held for the master password; the caller has verified it.
// The version applied is the one that was held and verified, and only while it is still the
// latest: a newer version arriving since supersedes the decision the user was asked to make.
func (e *SyncEngine) ConfirmHeld(ctx context.Context, vault, record vaultsync.ID) error {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return err
	}
	item, err := e.heldItem(ctx, vault, record)
	if err != nil {
		return err
	}
	if !item.Pending {
		return ErrHeldNotFound
	}
	if base, ok := p.base[record]; !ok || base.Version != item.Envelope.Version {
		return errors.Join(ErrHeldStale, e.d.State.Release(ctx, vault, record))
	}
	entry, reason := p.decode(&item.Envelope)
	if reason != nil {
		return reason
	}
	if err := e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { return synctrust.ApplyScope(d, p.scope, []synctrust.Entry{entry}) }); err != nil {
		return err
	}
	return e.d.State.Release(ctx, vault, record)
}

// RejectHeld drops a held or refused change. The vault keeps its own value, which no longer
// matches the version the server holds, so the next push writes it back for every device: the
// held change is reverted everywhere, not hidden here.
func (e *SyncEngine) RejectHeld(ctx context.Context, vault, record vaultsync.ID) error {
	if _, err := e.heldItem(ctx, vault, record); err != nil {
		return err
	}
	return e.d.State.Release(ctx, vault, record)
}

// waiting counts what sits in the vault's quarantine: changes held for the master password, and
// changes refused. A round reports these totals, not what its own pull added: the status is built
// from the last round, and a change held two rounds ago read as "up to date" while it still waited.
func (e *SyncEngine) waiting(ctx context.Context, vault vaultsync.ID) (held, refused int, err error) {
	items, err := e.d.State.Quarantined(ctx, vault)
	if err != nil {
		return 0, 0, err
	}
	for _, it := range items {
		if it.Pending {
			held++
		} else {
			refused++
		}
	}
	return held, refused, nil
}

func (e *SyncEngine) heldItem(ctx context.Context, vault, record vaultsync.ID) (synctrust.QuarantineItem, error) {
	items, err := e.d.State.Quarantined(ctx, vault)
	if err != nil {
		return synctrust.QuarantineItem{}, err
	}
	for _, it := range items {
		if it.Envelope.Record == record {
			return it, nil
		}
	}
	return synctrust.QuarantineItem{}, ErrHeldNotFound
}

// Devices lists the account's devices from the device log this device verified.
func (e *SyncEngine) Devices(ctx context.Context) ([]DeviceInfo, error) {
	dl, _, err := e.verifiedDeviceLog(ctx)
	if err != nil {
		return nil, err
	}
	self := e.d.Keys.Device()
	var out []DeviceInfo
	for _, id := range dl.Devices() {
		cert, _ := dl.Certificate(id)
		out = append(out, DeviceInfo{Cert: cert, Revoked: dl.IsRevoked(id), Current: id == self})
	}
	return out, nil
}
