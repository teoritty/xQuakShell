package usecase

import (
	"context"
	"errors"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// Register creates an account and starts syncing to it.
func (c *SyncController) Register(ctx context.Context, cred SyncCredentials) error {
	s, err := c.d.Accounts.Register(ctx, cred)
	if err != nil {
		return err
	}
	if err := c.d.NewEngine(s).EnsureVaultMeta(ctx, s.Personal, c.d.Random); err != nil {
		return err
	}
	c.adopt(ctx, s)
	return nil
}

// Login enrols this device in an existing account. When both this device and the account hold
// data it returns true and waits for Join: nothing syncs until the user chose merge or replace.
func (c *SyncController) Login(ctx context.Context, cred SyncCredentials) (bool, error) {
	s, err := c.d.Accounts.Login(ctx, cred)
	if err != nil {
		return false, err
	}
	need, err := c.d.Accounts.JoinNeeded(ctx, s)
	if err != nil {
		return false, err
	}
	if need {
		if err := c.d.Accounts.storeAccount(ctx, func(a *domain.SyncAccount) { a.JoinPending = true }); err != nil {
			return false, err
		}
		c.mu.Lock()
		c.pendingJoin = &s
		c.mu.Unlock()
		return true, nil
	}
	if err := c.d.NewEngine(s).EnsureVaultMeta(ctx, s.Personal, c.d.Random); err != nil {
		return false, err
	}
	c.adopt(ctx, s)
	return false, nil
}

// Join finishes a login that found data on both sides; it returns the backup's file name.
func (c *SyncController) Join(ctx context.Context, choice JoinChoice) (string, error) {
	c.mu.Lock()
	s := c.pendingJoin
	c.mu.Unlock()
	if s == nil {
		return "", ErrNoPendingJoin
	}
	name, _, err := c.d.Accounts.Join(ctx, *s, choice)
	if err != nil {
		return name, err
	}
	if err := c.d.Accounts.storeAccount(ctx, func(a *domain.SyncAccount) { a.JoinPending = false }); err != nil {
		return name, err
	}
	c.mu.Lock()
	c.pendingJoin = nil
	c.mu.Unlock()
	c.adopt(ctx, *s)
	return name, nil
}

// Relogin fetches keys another device rotated and resumes.
func (c *SyncController) Relogin(ctx context.Context, password []byte) error {
	return c.replaceSession(ctx, func() (SyncSession, error) { return c.d.Accounts.Relogin(ctx, password) })
}

// ChangePassword rotates the password and the account keys and resumes on the new keys.
func (c *SyncController) ChangePassword(ctx context.Context, old, next []byte) error {
	return c.replaceSession(ctx, func() (SyncSession, error) { return c.d.Accounts.ChangePassword(ctx, old, next) })
}

// ResetPassword sets a new password from this signed-in device.
func (c *SyncController) ResetPassword(ctx context.Context, next []byte) error {
	return c.replaceSession(ctx, func() (SyncSession, error) { return c.d.Accounts.ResetPassword(ctx, next) })
}

// RevokeDevices removes devices from the account and resumes on the rotated keys.
func (c *SyncController) RevokeDevices(ctx context.Context, devices []vaultsync.ID, password []byte) error {
	return c.replaceSession(ctx, func() (SyncSession, error) { return c.d.Accounts.RevokeDevices(ctx, devices, password) })
}

// replaceSession stops the running engine before an operation that replaces the keys - a round
// on the old keys mid-rotation would race the rotation - and resumes on whatever is stored after,
// including after a failure that still changed the keys.
func (c *SyncController) replaceSession(ctx context.Context, f func() (SyncSession, error)) error {
	c.Stop()
	c.op.Lock()
	s, err := f()
	c.op.Unlock()
	if err == nil {
		c.adopt(ctx, s)
		return nil
	}
	if startErr := c.Start(ctx); startErr != nil {
		return errors.Join(err, startErr)
	}
	return err
}

// Disable stops syncing on this device and forgets its sync account and state. The account and
// the data on the server stay, for the other devices.
//
// The whole state store is emptied, not only the personal vault: team vaults, the device log and
// any pending commit belong to the account this device is leaving. It also runs on a device that
// is not syncing - a removed device, or one whose sign-in never finished - which has no session to
// say which vaults it held.
func (c *SyncController) Disable(ctx context.Context) error {
	c.Stop()
	c.mu.Lock()
	c.pendingJoin = nil
	c.mu.Unlock()
	err := c.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		d.Sync = nil
		return nil
	})
	if err != nil {
		return err
	}
	return c.d.State.ForgetAll(ctx)
}

// Devices lists the account's devices from the verified log, revoked ones included.
func (c *SyncController) Devices(ctx context.Context) ([]DeviceInfo, error) {
	var out []DeviceInfo
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error {
		var err error
		out, err = e.Devices(ctx)
		return err
	})
	return out, err
}

// HeldChanges lists what a pull held or refused.
func (c *SyncController) HeldChanges(ctx context.Context) ([]HeldChange, error) {
	var out []HeldChange
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error {
		var err error
		out, err = e.HeldChanges(ctx, s.Personal)
		return err
	})
	return out, err
}

// ConfirmHeld applies a held change once the master password proves the person at this machine
// agrees - not another device, and not whoever holds the sync password.
func (c *SyncController) ConfirmHeld(ctx context.Context, record vaultsync.ID, masterPassword string) error {
	if masterPassword == "" || c.d.Vault.VerifyMasterPassword(ctx, masterPassword) != nil {
		return ErrHeldReauth
	}
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error { return e.ConfirmHeld(ctx, s.Personal, record) })
	if err == nil {
		// A round reports the quarantine as it now stands, so the status stops counting this change.
		c.Notify()
	}
	return err
}

// RejectHeld drops a held or refused change; the next round writes this device's value back.
func (c *SyncController) RejectHeld(ctx context.Context, record vaultsync.ID) error {
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error { return e.RejectHeld(ctx, s.Personal, record) })
	if err == nil {
		c.Notify()
	}
	return err
}

// Trash lists the deleted records of the personal vault.
func (c *SyncController) Trash(ctx context.Context) ([]TrashItem, error) {
	var out []TrashItem
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error {
		var err error
		out, err = e.Trash(ctx, s.Personal)
		return err
	})
	return out, err
}

// RestoreTrash brings a deleted record back; the round it schedules commits it.
func (c *SyncController) RestoreTrash(ctx context.Context, record vaultsync.ID) error {
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error { return e.Restore(ctx, s.Personal, record) })
	if err == nil {
		c.Notify()
	}
	return err
}

// ShredNow destroys the trash and history of the personal vault on every device and the server.
// It cannot be undone, so it takes the master password like any other irreversible decision.
func (c *SyncController) ShredNow(ctx context.Context, masterPassword string) error {
	if masterPassword == "" || c.d.Vault.VerifyMasterPassword(ctx, masterPassword) != nil {
		return ErrShredReauth
	}
	return c.withEngine(func(e *SyncEngine, s SyncSession) error { return e.ShredNow(ctx, s.Personal) })
}

// SyncAccountInfo is what the settings page shows about the account. It carries no secret: the
// identity key is public, and the seeds never leave the vault.
type SyncAccountInfo struct {
	ServerURL      string
	Username       string
	ServerIdentity []byte
	// AccountIdentity is the account's genesis identity key: the one a team admin compares by
	// fingerprint before inviting this account, which a password change does not move.
	AccountIdentity []byte
	RekeyPending    bool
	JoinPending     bool
}

// AccountInfo describes the stored account; ok is false when sync is not enabled.
func (c *SyncController) AccountInfo() (SyncAccountInfo, bool, error) {
	data, err := c.d.Vault.GetData()
	if err != nil || data.Sync == nil || data.Sync.Account == nil {
		return SyncAccountInfo{}, false, err
	}
	a := data.Sync.Account
	return SyncAccountInfo{ServerURL: a.ServerURL, Username: a.Username, ServerIdentity: a.ServerIdentity,
		AccountIdentity: a.GenesisIdentityKey, RekeyPending: a.RekeyPending, JoinPending: a.JoinPending}, true, nil
}

// Probe fetches a server's identity for the user to confirm by fingerprint.
func (c *SyncController) Probe(ctx context.Context, url string) (ServerProbe, error) {
	return c.d.Accounts.Probe(ctx, url)
}
