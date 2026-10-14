package wails

import (
	"errors"
	"log/slog"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// ReloginSync signs in again after another device rotated the account keys.
func (a *AppAPI) ReloginSync(password string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	if password == "" {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.Relogin(a.reqCtx(), []byte(password)))
}

// ChangeSyncPassword changes the sync password, which also rotates the account keys.
func (a *AppAPI) ChangeSyncPassword(oldPassword, newPassword string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	if oldPassword == "" || newPassword == "" {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.ChangePassword(a.reqCtx(), []byte(oldPassword), []byte(newPassword)))
}

// ResetSyncPassword sets a new sync password from this signed-in device, without the old one.
func (a *AppAPI) ResetSyncPassword(newPassword string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	if newPassword == "" {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.ResetPassword(a.reqCtx(), []byte(newPassword)))
}

// RevokeSyncDevice removes a device from the account; it needs the sync password.
func (a *AppAPI) RevokeSyncDevice(deviceID, password string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	id, ok := parseSyncID(deviceID)
	if !ok || password == "" {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.RevokeDevices(a.reqCtx(), []vaultsync.ID{id}, []byte(password)))
}

// AcknowledgeSyncDevices clears the notices of devices the person recognised as their own.
func (a *AppAPI) AcknowledgeSyncDevices(deviceIDs []string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	if len(deviceIDs) == 0 || len(deviceIDs) > 256 {
		return errInvalidSyncRequest
	}
	ids := make([]vaultsync.ID, 0, len(deviceIDs))
	for _, s := range deviceIDs {
		id, ok := parseSyncID(s)
		if !ok {
			return errInvalidSyncRequest
		}
		ids = append(ids, id)
	}
	return userSyncError(a.sync.AcknowledgeNewDevices(a.reqCtx(), ids))
}

// DisableSync stops syncing on this device and forgets its sync account here.
func (a *AppAPI) DisableSync() error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	return userSyncError(a.sync.Disable(a.reqCtx()))
}

// SyncNow runs a round at once.
func (a *AppAPI) SyncNow() error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	return userSyncError(a.sync.SyncNow())
}

// ListSyncDevices lists the account's devices for the devices panel, revoked ones included.
func (a *AppAPI) ListSyncDevices() ([]SyncDeviceDTO, error) {
	if a.sync == nil {
		return nil, errSyncUnavailable
	}
	devices, err := a.sync.Devices(a.reqCtx())
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]SyncDeviceDTO, 0, len(devices))
	for _, d := range devices {
		out = append(out, deviceDTO(d))
	}
	return out, nil
}

// ListSyncHeld lists changes held for the master password and changes that were refused.
func (a *AppAPI) ListSyncHeld() ([]SyncHeldDTO, error) {
	if a.sync == nil {
		return nil, errSyncUnavailable
	}
	held, err := a.sync.HeldChanges(a.reqCtx())
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]SyncHeldDTO, 0, len(held))
	for _, h := range held {
		out = append(out, heldDTO(h))
	}
	return out, nil
}

// ConfirmSyncHeld accepts a held change with the master password.
func (a *AppAPI) ConfirmSyncHeld(record, masterPassword string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	id, ok := parseSyncID(record)
	if !ok {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.ConfirmHeld(a.reqCtx(), id, masterPassword))
}

// RejectSyncHeld drops a held or refused change; this device's value is written back everywhere.
func (a *AppAPI) RejectSyncHeld(record string) error {
	if a.sync == nil {
		return errSyncUnavailable
	}
	id, ok := parseSyncID(record)
	if !ok {
		return errInvalidSyncRequest
	}
	return userSyncError(a.sync.RejectHeld(a.reqCtx(), id))
}

// ListSyncConflicts lists unresolved credential conflicts, without the values of secret fields.
func (a *AppAPI) ListSyncConflicts() ([]SyncConflictDTO, error) {
	if a.syncConflicts == nil {
		return nil, errSyncUnavailable
	}
	list, err := a.syncConflicts.Described()
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]SyncConflictDTO, 0, len(list))
	for _, c := range list {
		out = append(out, conflictDTO(c))
	}
	return out, nil
}

// ResolveSyncConflict settles a conflict: useOther writes the value that lost the merge.
func (a *AppAPI) ResolveSyncConflict(id string, useOther bool) error {
	if a.syncConflicts == nil {
		return errSyncUnavailable
	}
	if !validSyncField(id) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.syncConflicts.Resolve(a.reqCtx(), id, useOther))
}

// syncMessages maps every sync error a person can act on to a fixed sentence. Anything else is
// logged and reported generically: no server text, path or id ever reaches the UI.
//
// The first match wins, so the order is a decision. A removed device and keys changed elsewhere come
// before the integrity failure: both are routine events on another of the person's devices, and an
// error that carries one of them alongside a verification failure must read as what to do next,
// not as the server tampering with the vault.
var syncMessages = []struct {
	err error
	msg string
}{
	{usecase.ErrSyncLogin, "Wrong username or sync password."},
	{synctrust.ErrServerIdentity, "The server's identity does not match the one you confirmed. Nothing was sent to it."},
	{usecase.ErrSyncDeviceRevoked, "This device was removed from your sync account on another device. Turn sync off here, then connect again if you want this device back."},
	{synctrust.ErrKeysRotated, "Your sync password was changed on another device. Sign in again with the new password."},
	{synctrust.ErrIntegrity, "Data from the sync server failed verification. Sync stopped and nothing from it was applied."},
	{synctrust.ErrRemoteUnsupported, "The sync server does not support this version of xQuakShell."},
	{synctrust.ErrRemoteFrozen, "This sync account was frozen by the server's administrator."},
	{synctrust.ErrRemoteQuota, "The sync server's storage quota is used up."},
	{usecase.ErrSyncAlreadyEnabled, "Sync is already enabled on this vault."},
	{usecase.ErrSyncNotEnabled, "Sync is not enabled on this vault."},
	{usecase.ErrSyncOff, "Sync is not running. Unlock the vault and enable sync first."},
	{usecase.ErrHeldReauth, "The master password is needed to accept this change."},
	{usecase.ErrHeldNotFound, "That change is no longer waiting."},
	{usecase.ErrHeldStale, "A newer version of that item has arrived; review it instead."},
	{usecase.ErrConflictNotFound, "That conflict is already resolved."},
	{usecase.ErrSyncRevokeTarget, "That device cannot be removed from here."},
	{usecase.ErrNoPendingJoin, "No sign-in is waiting to be joined."},
	{usecase.ErrNoBackup, "The vault could not be backed up, so nothing was changed."},
	{usecase.ErrSyncBusy, "Another device keeps changing the vault. Sync will try again shortly."},
	{usecase.ErrTrashNotFound, "That item is not in the trash any more."},
	{usecase.ErrRestoreWeakens, "Restoring that item would weaken this device's security settings, so it was not restored."},
	{usecase.ErrShredReauth, "The master password is needed to shred the trash and history."},
	{usecase.ErrInviteKeyChanged, "That account's key is not the one you compared. Look it up again and compare the new fingerprint."},
	{usecase.ErrContactMismatch, "That account's key changed since you verified it. Compare its new fingerprint before sharing anything with it."},
	{usecase.ErrNotATeam, "The personal vault has no members."},
	{usecase.ErrBindingTarget, "Choose a key or password from your own vault."},
	{domain.ErrConnectionNotFound, "That connection is not in your personal vault."},
	{vaultsync.ErrNotAuthorized, "Your role in this team does not allow that."},
	{domain.ErrBindingMissing, "No credential of yours is bound to that connection."},
	{domain.ErrBindingSuspended, "Another member changed that connection's address. Confirm the new address first."},
	{vaultsync.ErrUnverifiedMember, "This team admits only verified members. Compare the fingerprint and mark it verified."},
	{synctrust.ErrRemoteForbidden, "Your role in this team does not allow that."},
	{synctrust.ErrRemoteNotFound, "That account or team was not found."},
}

func userSyncError(err error) error {
	if err == nil {
		return nil
	}
	var limited *synctrust.RateLimitedError
	if errors.As(err, &limited) {
		return userFacingError{msg: "Too many attempts. Try again later.", cause: err}
	}
	for _, m := range syncMessages {
		if errors.Is(err, m.err) {
			return userFacingError{msg: m.msg, cause: err}
		}
	}
	slog.Warn("sync operation failed", "component", "sync", "err", err)
	return userFacingError{msg: "Sync failed. The debug log has the details.", cause: err}
}
