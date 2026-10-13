package usecase_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/usecase"
)

// passwordVault checks the master password like the real repository does.
type passwordVault struct{ *syncTestVault }

func (v passwordVault) VerifyMasterPassword(_ context.Context, pw string) error {
	if pw != "master" {
		return domain.ErrVaultDecryptFailed
	}
	return nil
}

type controlled struct {
	*device
	c      *usecase.SyncController
	status chan usecase.SyncStatus
}

func control(t *testing.T, d *device) *controlled {
	cd := &controlled{device: d, status: make(chan usecase.SyncStatus, 64)}
	cd.c = usecase.NewSyncController(usecase.SyncControllerDeps{Accounts: d.svc, Vault: passwordVault{d.vault}, State: d.state,
		NewEngine: d.newEngine, After: time.After, Random: rand.Reader,
		OnStatus: func(s usecase.SyncStatus) { cd.status <- s }})
	t.Cleanup(cd.c.Stop)
	return cd
}

// round waits for the next finished round and returns its status.
func (cd *controlled) round() usecase.SyncStatus {
	cd.t.Helper()
	for {
		select {
		case st := <-cd.status:
			if st.State != usecase.SyncRunning {
				return st
			}
		case <-time.After(10 * time.Second):
			cd.t.Fatal("no sync round finished")
		}
	}
}

func TestControllerStaysOffWithoutAnAccount(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	cd := control(t, newDevice(t, h, "laptop"))
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	if _, on := cd.c.Status(); on {
		t.Fatal("sync runs on a vault that never enabled it")
	}
	if err := cd.c.SyncNow(); !errors.Is(err, usecase.ErrSyncOff) {
		t.Fatalf("sync now: %v", err)
	}
}

func TestControllerResumesAndPushes(t *testing.T) {
	_, a := registered(t)
	cd := control(t, a)
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	if st := cd.round(); st.State != usecase.SyncIdle {
		t.Fatalf("first round: %+v", st)
	}
	a.addFolder("f9", "Ops")
	if err := cd.c.SyncNow(); err != nil {
		t.Fatal(err)
	}
	if st := cd.round(); st.State != usecase.SyncIdle || st.Report.Pushed == 0 {
		t.Fatalf("round after an edit: %+v", st)
	}
	cd.c.Stop()
	if _, on := cd.c.Status(); on {
		t.Fatal("still running after Stop - the vault locking must end sync")
	}
}

func (d *device) addFolder(id, name string) {
	_ = d.vault.UpdateData(syncCtx, func(v *domain.VaultData) error {
		v.Folders = append(v.Folders, domain.ConnectionFolder{ID: id, Name: name})
		return nil
	})
}

// heldOnB has A weaken plugin trust and B's controller hold it.
func heldOnB(t *testing.T) (*device, *controlled) {
	_, a, b := twoDevices(t)
	_ = a.vault.UpdateData(syncCtx, func(v *domain.VaultData) error { v.Settings.Plugins.RequireSignedPlugins = false; return nil })
	a.sync()
	cb := control(t, b)
	if err := cb.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	if st := cb.round(); st.Report.Pull.Held != 1 {
		t.Fatalf("round: %+v", st)
	}
	return a, cb
}

func TestHeldChangeNeedsTheMasterPassword(t *testing.T) {
	_, cb := heldOnB(t)
	held, err := cb.c.HeldChanges(syncCtx)
	if err != nil || len(held) != 1 || !held[0].Pending || held[0].Type != synctrust.SettingsType("plugins") {
		t.Fatalf("held = %+v, %v", held, err)
	}
	for _, pw := range []string{"", "wrong"} {
		if err := cb.c.ConfirmHeld(syncCtx, held[0].Record, pw); !errors.Is(err, usecase.ErrHeldReauth) {
			t.Fatalf("master password %q: %v", pw, err)
		}
	}
	if d, _ := cb.vault.GetData(); !d.Settings.Plugins.RequireSignedPlugins {
		t.Fatal("the change applied without the master password")
	}
	if err := cb.c.ConfirmHeld(syncCtx, held[0].Record, "master"); err != nil {
		t.Fatal(err)
	}
	if d, _ := cb.vault.GetData(); d.Settings.Plugins.RequireSignedPlugins {
		t.Fatal("the confirmed change did not apply")
	}
	if held, _ := cb.c.HeldChanges(syncCtx); len(held) != 0 {
		t.Fatal("the confirmed change is still listed")
	}
}

func TestRejectingAHeldChangeRevertsItEverywhere(t *testing.T) {
	a, cb := heldOnB(t)
	held, _ := cb.c.HeldChanges(syncCtx)
	if err := cb.c.RejectHeld(syncCtx, held[0].Record); err != nil {
		t.Fatal(err)
	}
	if err := cb.c.SyncNow(); err != nil {
		t.Fatal(err)
	}
	cb.round()
	a.sync()
	if d, _ := a.vault.GetData(); !d.Settings.Plugins.RequireSignedPlugins {
		t.Fatal("rejecting on B did not restore the stricter setting on A")
	}
}

func TestControllerListsDevicesAndDisables(t *testing.T) {
	_, _, b := twoDevices(t)
	cb := control(t, b)
	if err := cb.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	cb.round()
	devices, err := cb.c.Devices(syncCtx)
	if err != nil || len(devices) != 2 {
		t.Fatalf("devices = %+v, %v", devices, err)
	}
	current := 0
	for _, d := range devices {
		if d.Current {
			current++
			if d.Cert.Name != "phone" {
				t.Fatalf("the current device is %q", d.Cert.Name)
			}
		}
	}
	if current != 1 {
		t.Fatalf("%d devices marked current", current)
	}
	if err := cb.c.Disable(syncCtx); err != nil {
		t.Fatal(err)
	}
	if d, _ := b.vault.GetData(); d.Sync != nil {
		t.Fatal("disabling kept the sync account in the vault")
	}
	if sv, _ := b.state.LoadVault(syncCtx, b.session.Personal); sv.Head != nil {
		t.Fatal("disabling kept the sync state")
	}
	if err := cb.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	if _, on := cb.c.Status(); on {
		t.Fatal("a disabled device started syncing again")
	}
}

func TestLoginWaitsForTheJoinChoice(t *testing.T) {
	h, a := registered(t)
	a.addFolder("fa", "account")
	a.sync()
	c := newDevice(t, h, "tablet")
	c.addFolder("fc", "device")
	cc := control(t, c)
	if _, err := cc.c.Join(syncCtx, usecase.JoinMerge); !errors.Is(err, usecase.ErrNoPendingJoin) {
		t.Fatalf("join without a login: %v", err)
	}
	need, err := cc.c.Login(syncCtx, creds("correct horse"))
	if err != nil || !need {
		t.Fatalf("login = %v, %v; both sides hold data", need, err)
	}
	if _, on := cc.c.Status(); on {
		t.Fatal("sync started before the user chose how to join")
	}
	name, err := cc.c.Join(syncCtx, usecase.JoinMerge)
	if err != nil || name == "" {
		t.Fatalf("join = %q, %v", name, err)
	}
	if _, on := cc.c.Status(); !on {
		t.Fatal("sync did not start after the join")
	}
	if c.backup.taken == nil {
		t.Fatal("no backup before the join")
	}
}

func TestNothingRunsAfterTheVaultLocks(t *testing.T) {
	_, a := registered(t)
	cd := control(t, a)
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	cd.round()
	cd.c.Stop()
	if _, err := cd.c.Devices(syncCtx); !errors.Is(err, usecase.ErrSyncOff) {
		t.Fatalf("devices after lock: %v; a locked vault's keys must not be used", err)
	}
	if _, err := cd.c.HeldChanges(syncCtx); !errors.Is(err, usecase.ErrSyncOff) {
		t.Fatalf("held changes after lock: %v", err)
	}
}

func TestAccountInfoCarriesNoSecret(t *testing.T) {
	_, a := registered(t)
	cd := control(t, a)
	info, ok, err := cd.c.AccountInfo()
	if err != nil || !ok || info.Username != "alice" || len(info.ServerIdentity) != 32 {
		t.Fatalf("info = %+v, %v, %v", info, ok, err)
	}
	h := servertest.NewHarness(t, 0)
	if _, ok, _ := control(t, newDevice(t, h, "x")).c.AccountInfo(); ok {
		t.Fatal("account info on a vault without sync")
	}
}
