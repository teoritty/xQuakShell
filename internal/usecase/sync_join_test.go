package usecase_test

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// fakeBackup records the vault as it stood when the backup was taken.
type fakeBackup struct {
	vault *syncTestVault
	taken *domain.VaultData
	fail  error
}

func (b *fakeBackup) BackupNow(_ context.Context, reason string) (string, error) {
	if b.fail != nil {
		return "", b.fail
	}
	b.taken, _ = b.vault.GetData()
	return "vault.age." + reason + ".bak", nil
}

// joiner is a device with its own folder that logs in to an account already holding one.
func joiner(t *testing.T) (*device, *device, usecase.SyncSession) {
	h, a := registered(t)
	_ = a.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "fa", Name: "Account's"})
		d.Settings.Terminal.FontSize = 21
		return nil
	})
	a.sync()
	b := newDevice(t, h, "phone")
	_ = b.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "fb", Name: "Device's"})
		return nil
	})
	s, err := b.svc.Login(syncCtx, creds("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	b.session, b.engine = s, b.newEngine(s)
	return a, b, s
}

func TestJoinIsAskedOnlyWhenBothSidesHaveData(t *testing.T) {
	_, b, s := joiner(t)
	if need, err := b.svc.JoinNeeded(syncCtx, s); err != nil || !need {
		t.Fatalf("need = %v, %v; both sides hold data", need, err)
	}
	_, a := registered(t)
	if need, _ := a.svc.JoinNeeded(syncCtx, a.session); need {
		t.Fatal("asked to join an empty account")
	}
}

func TestJoinMergeKeepsBoth(t *testing.T) {
	a, b, s := joiner(t)
	name, _, err := b.svc.Join(syncCtx, s, usecase.JoinMerge)
	if err != nil || name == "" {
		t.Fatalf("join: %q, %v", name, err)
	}
	if b.folder("fa") == "" || b.folder("fb") == "" {
		t.Fatal("merge lost a side")
	}
	a.sync()
	if a.folder("fb") != "Device's" {
		t.Fatal("the joining device's data did not reach the account")
	}
	if b.backup.taken == nil || len(b.backup.taken.Folders) != 1 {
		t.Fatal("the backup was not taken before the merge changed the vault")
	}
}

func TestJoinReplaceTakesTheAccountsData(t *testing.T) {
	a, b, s := joiner(t)
	if _, _, err := b.svc.Join(syncCtx, s, usecase.JoinReplace); err != nil {
		t.Fatal(err)
	}
	if b.folder("fb") != "" || b.folder("fa") != "Account's" {
		t.Fatal("replace kept this device's folder or missed the account's")
	}
	if d, _ := b.vault.GetData(); d.Settings.Terminal.FontSize != 21 {
		t.Fatalf("font size = %d; replace must take the account's settings, not keep local defaults", d.Settings.Terminal.FontSize)
	}
	a.sync()
	if a.folder("fb") != "" {
		t.Fatal("the replaced data leaked to the account")
	}
	if b.backup.taken == nil || b.backup.taken.Folders[0].ID != "fb" {
		t.Fatal("the device's own data was not backed up before it was replaced")
	}
}

func TestJoinChangesNothingWithoutABackup(t *testing.T) {
	_, b, s := joiner(t)
	b.backup.fail = errors.New("disk full")
	if _, _, err := b.svc.Join(syncCtx, s, usecase.JoinReplace); !errors.Is(err, usecase.ErrNoBackup) {
		t.Fatalf("err = %v", err)
	}
	if b.folder("fb") != "Device's" {
		t.Fatal("the vault changed although the backup failed")
	}
	if _, _, err := b.svc.Join(syncCtx, s, usecase.JoinChoice(9)); !errors.Is(err, usecase.ErrJoinChoice) {
		t.Fatalf("unknown choice: %v", err)
	}
}
