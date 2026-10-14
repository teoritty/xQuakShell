package wails

import (
	"encoding/hex"
)

// ListSyncTrash lists deleted records of the synced vault and when each will be purged.
func (a *AppAPI) ListSyncTrash() ([]SyncTrashItemDTO, error) {
	if a.sync == nil {
		return nil, errSyncUnavailable
	}
	items, err := a.sync.Trash(a.reqCtx())
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]SyncTrashItemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, SyncTrashItemDTO{Record: hex.EncodeToString(it.Record[:]), Type: string(it.Type), Key: it.Key,
			Name: it.Name, DeletedUnixMilli: it.DeletedUnixMilli, PurgeUnixMilli: it.PurgeUnixMilli, Restorable: it.Restorable})
	}
	return out, nil
}

// RestoreSyncTrash brings a deleted record back on every device.
func (a *AppAPI) RestoreSyncTrash(record string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	id, ok := parseSyncID(record)
	if !ok {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.RestoreTrash(a.reqCtx(), id))
}

// ShredSyncNow destroys the synced vault's trash and history everywhere. The master password is
// checked by the use case; an empty one is refused here so it never travels further.
func (a *AppAPI) ShredSyncNow(masterPassword string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	if masterPassword == "" {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.ShredNow(a.reqCtx(), masterPassword))
}
