package usecase_test

import (
	"context"
	"crypto/rand"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
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

// settle waits until each of vaults has finished a round. round returns at the first vault a pass
// finishes while the pass goes on to the next one, and on a slow runner a test that measured then
// took the lock first and counted the next vault's requests as its own.
func (cd *controlled) settle(vaults ...vaultsync.ID) {
	cd.t.Helper()
	left := map[vaultsync.ID]bool{}
	for _, v := range vaults {
		left[v] = true
	}
	for len(left) > 0 {
		delete(left, cd.round().Vault)
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

func TestHeldChangeStaysInTheStatusUntilDecided(t *testing.T) {
	_, cb := heldOnB(t)
	if err := cb.c.SyncNow(); err != nil {
		t.Fatal(err)
	}
	if st := cb.round(); st.Report.Pull.Held != 1 {
		t.Fatalf("held = %d after a round that pulled nothing, want 1; a change still waiting must not read as up to date", st.Report.Pull.Held)
	}
	held, _ := cb.c.HeldChanges(syncCtx)
	if err := cb.c.ConfirmHeld(syncCtx, held[0].Record, "master"); err != nil {
		t.Fatal(err)
	}
	if st := cb.round(); st.Report.Pull.Held != 0 {
		t.Fatalf("held = %d after the change was confirmed, want 0", st.Report.Pull.Held)
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
	if log, _ := b.state.DeviceLog(syncCtx, b.session.Keys.Account()); len(log) != 0 {
		t.Fatalf("disabling kept %d device log entries; the account this device left stays behind", len(log))
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

// A login that is waiting for the join choice must still be waiting after a restart: resuming
// would sync with merge semantics and skip the backup the join takes first.
func TestAPendingJoinSurvivesARestart(t *testing.T) {
	h, a := registered(t)
	a.addFolder("fa", "account")
	a.sync()
	c := newDevice(t, h, "tablet")
	c.addFolder("fc", "device")
	first := control(t, c)
	if need, err := first.c.Login(syncCtx, creds("correct horse")); err != nil || !need {
		t.Fatalf("login = %v, %v", need, err)
	}
	first.c.Stop()
	restarted := control(t, c)
	if err := restarted.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	if _, on := restarted.c.Status(); on {
		t.Fatal("sync resumed before the user chose how to join")
	}
	if info, _, _ := restarted.c.AccountInfo(); !info.JoinPending {
		t.Fatal("the pending join is not reported, so the page cannot ask for it")
	}
	if _, err := restarted.c.Join(syncCtx, usecase.JoinMerge); err != nil {
		t.Fatalf("join after restart: %v", err)
	}
	if info, _, _ := restarted.c.AccountInfo(); info.JoinPending {
		t.Fatal("the join flag outlived the join")
	}
	if c.backup.taken == nil {
		t.Fatal("no backup before the join")
	}
}

func TestShredNowNeedsTheMasterPassword(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	cd := control(t, a)
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	cd.round()
	for _, pw := range []string{"", "wrong"} {
		if err := cd.c.ShredNow(syncCtx, pw); !errors.Is(err, usecase.ErrShredReauth) {
			t.Fatalf("master password %q: %v", pw, err)
		}
	}
	if items, _ := cd.c.Trash(syncCtx); len(items) != 1 {
		t.Fatal("the trash changed without the master password")
	}
	if err := cd.c.ShredNow(syncCtx, "master"); err != nil {
		t.Fatal(err)
	}
	if items, err := cd.c.Trash(syncCtx); err != nil || len(items) != 0 {
		t.Fatalf("trash after shred now = %+v, %v", items, err)
	}
}

func TestRestoreThroughTheControllerSchedulesARound(t *testing.T) {
	_, a := registered(t)
	addFolders(a, "f1")
	a.sync()
	removeFolder(a, "f1")
	a.sync()
	cd := control(t, a)
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	cd.round()
	items, err := cd.c.Trash(syncCtx)
	if err != nil || len(items) != 1 {
		t.Fatalf("trash = %+v, %v", items, err)
	}
	if err := cd.c.RestoreTrash(syncCtx, items[0].Record); err != nil {
		t.Fatal(err)
	}
	if st := cd.round(); st.Report.Pushed == 0 {
		t.Fatalf("round after a restore: %+v; the restored record must be committed without waiting", st)
	}
}

func TestControllerCreatesAndListsTeams(t *testing.T) {
	_, a := registered(t)
	cd := control(t, a)
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	cd.round()
	team, err := cd.c.CreateTeam(syncCtx, "Ops")
	if err != nil {
		t.Fatal(err)
	}
	teams, err := cd.c.Teams(syncCtx)
	if err != nil || len(teams) != 1 || teams[0].Vault != team || teams[0].Name != "Ops" || teams[0].Role != vaultsync.RoleOwner {
		t.Fatalf("teams = %+v, %v", teams, err)
	}
}

// The Sync page lists teams on every reload; re-verifying each team against the server for it
// tripped the rate limit. A team is verified again only once its stored head or membership moved.
func TestTeamsAreNotReverifiedUntilTheyChange(t *testing.T) {
	_, a := registered(t)
	cd := control(t, a)
	if err := cd.c.Start(syncCtx); err != nil {
		t.Fatal(err)
	}
	cd.round()
	team, err := cd.c.CreateTeam(syncCtx, "Ops")
	if err != nil {
		t.Fatal(err)
	}
	// The pass that creating the team schedules syncs both vaults. Measuring before it reaches the
	// team counted the pass's own fetch of the log as the listing's, and failed on slow runners.
	cd.settle(a.session.Personal, team)
	if teams, err := cd.c.Teams(syncCtx); err != nil || len(teams) != 1 {
		t.Fatalf("teams = %+v, %v", teams, err)
	}
	// Atomic: the scheduler's own passes run on another goroutine and fetch the log too.
	var fetched atomic.Int32
	a.server.svcServer.tamperLog = func(es []vaultsync.DeviceEntry) []vaultsync.DeviceEntry { fetched.Add(1); return es }
	for range 3 {
		if teams, err := cd.c.Teams(syncCtx); err != nil || len(teams) != 1 || teams[0].Name != "Ops" {
			t.Fatalf("teams = %+v, %v", teams, err)
		}
	}
	if n := fetched.Load(); n != 0 {
		t.Fatalf("listing an unchanged team fetched the device log %d times", n)
	}
	if err := cd.c.SetTeamPolicy(syncCtx, team, 45, false); err != nil {
		t.Fatal(err)
	}
	// The op reaches the state store with the next pull, and the listing follows the store.
	_ = cd.c.SyncNow()
	cd.round()
	cd.round()
	before := fetched.Load()
	if _, err := cd.c.Teams(syncCtx); err != nil {
		t.Fatal(err)
	}
	if fetched.Load() == before {
		t.Fatal("a team whose membership moved was listed from the cache; its role may have changed")
	}
}
