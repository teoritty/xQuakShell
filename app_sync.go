package main

import "xquakshell/internal/presentation/wails"

// Wails facade for state sync (ADR-010): one-line delegates, no logic.

// GetSyncStatus reports whether sync is enabled and how its last round went.
func (a *App) GetSyncStatus() (wails.SyncStatusDTO, error) { return a.api.GetSyncStatus() }

// ProbeSyncServer fetches a server identity for the user to confirm by fingerprint.
func (a *App) ProbeSyncServer(serverURL string) (wails.SyncServerProbeDTO, error) {
	return a.api.ProbeSyncServer(serverURL)
}

// EnableSync registers an account or logs in to one with the confirmed server identity.
func (a *App) EnableSync(req wails.EnableSyncRequest) (wails.EnableSyncResultDTO, error) {
	return a.api.EnableSync(req)
}

// JoinSync finishes a login that found data on both sides.
func (a *App) JoinSync(choice string) (string, error) { return a.api.JoinSync(choice) }

// ReloginSync signs in again after another device rotated the account keys.
func (a *App) ReloginSync(password string) error { return a.api.ReloginSync(password) }

// ChangeSyncPassword changes the sync password and rotates the account keys.
func (a *App) ChangeSyncPassword(oldPassword, newPassword string) error {
	return a.api.ChangeSyncPassword(oldPassword, newPassword)
}

// ResetSyncPassword sets a new sync password from this signed-in device.
func (a *App) ResetSyncPassword(newPassword string) error {
	return a.api.ResetSyncPassword(newPassword)
}

// RevokeSyncDevice removes another device from the account.
func (a *App) RevokeSyncDevice(deviceID, password string) error {
	return a.api.RevokeSyncDevice(deviceID, password)
}

// DisableSync stops syncing on this device.
func (a *App) DisableSync() error { return a.api.DisableSync() }

// SyncNow runs a sync round at once.
func (a *App) SyncNow() error { return a.api.SyncNow() }

// ListSyncDevices lists the account's devices for the devices panel, revoked ones included.
func (a *App) ListSyncDevices() ([]wails.SyncDeviceDTO, error) { return a.api.ListSyncDevices() }

// ListSyncHeld lists changes held for the master password or refused.
func (a *App) ListSyncHeld() ([]wails.SyncHeldDTO, error) { return a.api.ListSyncHeld() }

// ConfirmSyncHeld accepts a held change with the master password.
func (a *App) ConfirmSyncHeld(record, masterPassword string) error {
	return a.api.ConfirmSyncHeld(record, masterPassword)
}

// RejectSyncHeld drops a held or refused change.
func (a *App) RejectSyncHeld(record string) error { return a.api.RejectSyncHeld(record) }

// ListSyncConflicts lists unresolved credential conflicts.
func (a *App) ListSyncConflicts() ([]wails.SyncConflictDTO, error) { return a.api.ListSyncConflicts() }

// ResolveSyncConflict settles a conflict in favour of one value.
func (a *App) ResolveSyncConflict(id string, useOther bool) error {
	return a.api.ResolveSyncConflict(id, useOther)
}
