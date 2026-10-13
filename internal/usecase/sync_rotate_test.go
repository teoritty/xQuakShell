package usecase_test

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/usecase"
)

// twoDevices is a registered account with a folder, synced to a second device.
func twoDevices(t *testing.T) (*servertest.Harness, *device, *device) {
	h, a := registered(t)
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f1", Name: "Prod"})
		return nil
	})
	a.sync()
	b := newDevice(t, h, "phone")
	s, err := b.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	b.start(s)
	b.sync()
	return h, a, b
}

func (d *device) restart(s usecase.SyncSession) {
	d.session, d.engine = s, d.newEngine(s)
}

func (d *device) folder(id string) string {
	data, _ := d.vault.GetData()
	for _, f := range data.Folders {
		if f.ID == id {
			return f.Name
		}
	}
	return ""
}

func membership(t *testing.T, d *device) []vaultsync.MemberOp {
	t.Helper()
	ops, err := d.session.Server.Membership(syncCtx, d.session.Personal, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestChangePasswordRotatesKeysAndTheVaultEpoch(t *testing.T) {
	h, a, b := twoDevices(t)
	s, err := a.svc.ChangePassword(syncCtx, []byte("correct horse"), []byte("new horse"))
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	a.restart(s)
	a.sync()
	ops := membership(t, a)
	if last := ops[len(ops)-1]; last.Kind != vaultsync.MemberRotateEpoch || last.Epoch != 1 {
		t.Fatalf("last op = %d at epoch %d; the vault must move to a key the old one never opens", last.Kind, last.Epoch)
	}
	if d, _ := a.vault.GetData(); d.Sync.Account.RekeyPending || d.Sync.Account.Pending != nil {
		t.Fatal("the rotation left its pending markers behind")
	}

	if _, err := newDevice(t, h, "x").svc.Login(syncCtx, creds("correct horse")); !errors.Is(err, usecase.ErrSyncLogin) {
		t.Fatalf("the old password still logs in: %v", err)
	}
	c := newDevice(t, h, "tablet")
	sc, err := c.svc.Login(syncCtx, creds("new horse"))
	if err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	c.start(sc)
	c.sync()
	if c.folder("f1") != "Prod" {
		t.Fatal("a device enrolled after the rotation cannot read the vault")
	}

	if _, err := b.engine.Sync(syncCtx, b.session.Personal); !errors.Is(err, synctrust.ErrKeysRotated) {
		t.Fatalf("the other device: %v, want ErrKeysRotated so it asks for the new password", err)
	}
	sb, err := b.svc.Relogin(syncCtx, []byte("new horse"))
	if err != nil {
		t.Fatalf("relogin: %v", err)
	}
	b.restart(sb)
	_ = c.vault.UpdateData(syncCtx, func(d *domain.VaultData) error { d.Folders[0].Name = "Production"; return nil })
	c.sync()
	b.sync()
	if b.folder("f1") != "Production" {
		t.Fatal("the re-logged-in device does not follow the vault at the new epoch")
	}
}

func TestRevokedDeviceIsLockedOut(t *testing.T) {
	h, a, b := twoDevices(t)
	s, err := a.svc.RevokeDevices(syncCtx, []vaultsync.ID{b.session.Keys.Device()}, []byte("correct horse"))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	a.restart(s)
	a.sync()
	ops := membership(t, a)
	last := ops[len(ops)-1]
	if len(last.RevokedDevices) != 1 || last.RevokedDevices[0] != b.session.Keys.Device() {
		t.Fatalf("rotation revokes %v; the vault must stop accepting the device from here on", last.RevokedDevices)
	}
	if _, err := b.engine.Sync(syncCtx, b.session.Personal); err == nil {
		t.Fatal("the revoked device still syncs")
	}
	if _, err := b.svc.Relogin(syncCtx, []byte("correct horse")); !errors.Is(err, usecase.ErrSyncDeviceRevoked) {
		t.Fatalf("relogin of the revoked device: %v, want ErrSyncDeviceRevoked", err)
	}
	c := newDevice(t, h, "tablet")
	sc, err := c.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatalf("a new device after the revocation: %v", err)
	}
	c.start(sc)
	c.sync()
	if c.folder("f1") != "Prod" {
		t.Fatal("the vault is unreadable after the revocation")
	}
}

func TestResetPasswordFromASignedInDevice(t *testing.T) {
	h, a, _ := twoDevices(t)
	s, err := a.svc.ResetPassword(syncCtx, []byte("brand new"))
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	a.restart(s)
	c := newDevice(t, h, "tablet")
	if _, err := c.svc.Login(syncCtx, creds("brand new")); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestChangePasswordNeedsTheOldOne(t *testing.T) {
	_, a, _ := twoDevices(t)
	before, _ := a.session.Server.DeviceLog(syncCtx, a.session.Keys.Account(), 0)
	if _, err := a.svc.ChangePassword(syncCtx, []byte("wrong"), []byte("x")); !errors.Is(err, usecase.ErrSyncLogin) {
		t.Fatalf("err = %v", err)
	}
	after, _ := a.session.Server.DeviceLog(syncCtx, a.session.Keys.Account(), 0)
	if len(after) != len(before) {
		t.Fatal("a refused password change still appended to the device log")
	}
}

func TestRevokeTargetsAreChecked(t *testing.T) {
	_, a, _ := twoDevices(t)
	for name, id := range map[string]vaultsync.ID{"itself": a.session.Keys.Device(), "unknown": {9}} {
		if _, err := a.svc.RevokeDevices(syncCtx, []vaultsync.ID{id}, []byte("correct horse")); !errors.Is(err, usecase.ErrSyncRevokeTarget) {
			t.Errorf("revoking %s: %v", name, err)
		}
	}
}

// A succession that reached the device log but whose new bundle did not reach the server leaves
// no device able to sign in; the device that started it must remember to finish.
func TestInterruptedRekeyIsRememberedAndFinished(t *testing.T) {
	h, a, _ := twoDevices(t)
	a.failRekey = true
	if _, err := a.svc.ChangePassword(syncCtx, []byte("correct horse"), []byte("new horse")); err == nil {
		t.Fatal("a rotation whose bundle never reached the server reported success")
	}
	d, _ := a.vault.GetData()
	if !d.Sync.Account.RekeyPending || d.Sync.Account.Pending != nil {
		t.Fatalf("account = pending %v, rekeyPending %v; the unfinished rekey must be remembered", d.Sync.Account.Pending, d.Sync.Account.RekeyPending)
	}
	if _, err := a.svc.ResetPassword(syncCtx, []byte("new horse")); err != nil {
		t.Fatalf("finishing with a reset: %v", err)
	}
	if d, _ := a.vault.GetData(); d.Sync.Account.RekeyPending {
		t.Fatal("finishing did not clear the marker")
	}
	if _, err := newDevice(t, h, "tablet").svc.Login(syncCtx, creds("new horse")); err != nil {
		t.Fatalf("login after finishing: %v", err)
	}
}
