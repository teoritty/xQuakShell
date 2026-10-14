package usecase

import (
	"context"
	"encoding/hex"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// A device joining the account is announced from the stored device log, not from a round's
// report. The log reaches this device through more than one call - a sync round, but also listing
// the devices or the trash when the settings page opens - and each stores what it fetched; a
// notice raised only in the round's report was lost whenever another call fetched the entry first,
// which is how a device joined unannounced. Every entry in the store was verified before it was
// stored, so reading it back needs no network and changes nothing, and a pull that fails
// verification still leaves the vault exactly as it was.

// NewDeviceNotice is a device that joined the account after this one.
type NewDeviceNotice struct {
	Device           vaultsync.ID
	Name             string
	Platform         string
	CreatedUnixMilli int64
}

// newDeviceNotices lists, in log order, the devices certified after this device's own certificate
// that are neither revoked nor acknowledged. A device that joined before this one is not news here;
// the log's order, which every device verifies, says which is which, where clocks would not.
func newDeviceNotices(log []vaultsync.DeviceEntry, self vaultsync.ID, acknowledged []string) []NewDeviceNotice {
	var out []NewDeviceNotice
	seenSelf := false
	for _, e := range log {
		switch {
		case e.Op == vaultsync.OpAddDevice && e.Cert != nil && e.Cert.Device == self:
			seenSelf = true
		case e.Op == vaultsync.OpAddDevice && e.Cert != nil && seenSelf && !slices.Contains(acknowledged, hex.EncodeToString(e.Cert.Device[:])):
			out = append(out, NewDeviceNotice{Device: e.Cert.Device, Name: e.Cert.Name, Platform: e.Cert.Platform,
				CreatedUnixMilli: int64(min(e.Cert.CreatedUnixMilli, 1<<62))})
		case e.Op == vaultsync.OpRevokeDevice:
			out = slices.DeleteFunc(out, func(n NewDeviceNotice) bool { return n.Device == e.Revoked })
		}
	}
	return out
}

// NewDevices lists the devices that joined after this one and wait for the person to look. It
// reads the stored log only, so it is cheap enough for every status the UI asks for.
func (c *SyncController) NewDevices(ctx context.Context) ([]NewDeviceNotice, error) {
	data, err := c.d.Vault.GetData()
	if err != nil || data.Sync == nil || data.Sync.Account == nil {
		return nil, err
	}
	acct := data.Sync.Account
	log, err := c.d.State.DeviceLog(ctx, vaultsync.ID(acct.Account))
	if err != nil {
		return nil, err
	}
	return newDeviceNotices(log, vaultsync.ID(acct.Device), data.Sync.AcknowledgedDevices), nil
}

// AcknowledgeNewDevices records that the person recognised these devices as their own.
func (c *SyncController) AcknowledgeNewDevices(ctx context.Context, devices []vaultsync.ID) error {
	return c.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil || d.Sync.Account == nil {
			return ErrSyncNotEnabled
		}
		for _, dev := range devices {
			if id := hex.EncodeToString(dev[:]); !slices.Contains(d.Sync.AcknowledgedDevices, id) {
				d.Sync.AcknowledgedDevices = append(d.Sync.AcknowledgedDevices, id)
			}
		}
		return nil
	})
}
