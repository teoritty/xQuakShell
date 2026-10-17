package syncflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

func folder(d *domain.VaultData, id string) string {
	for _, f := range d.Folders {
		if f.ID == id {
			return f.Name
		}
	}
	return ""
}

// S1: register on A, enrol B by password, an edit on A appears on B, and B fetches only the record
// that changed.
func TestS1DeltaSync(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	for _, id := range []string{"f1", "f2", "f3"} {
		a.edit(func(v *domain.VaultData) { v.Folders = append(v.Folders, domain.ConnectionFolder{ID: id, Name: id}) })
	}
	a.sync()
	b := s.enrolled("phone", "correct horse")
	if folder(b.data(), "f3") != "f3" {
		t.Fatal("B did not receive A's data")
	}
	a.edit(func(v *domain.VaultData) { v.Folders[1].Name = "renamed" })
	a.sync()
	before := s.servedRecords()
	b.sync()
	if got := s.servedRecords() - before; got != 1 {
		t.Fatalf("B fetched %d records for one edit; sync must move deltas, not the vault", got)
	}
	if folder(b.data(), "f2") != "renamed" {
		t.Fatal("the edit did not reach B")
	}
}

// S2: offline edits of different fields of one connection merge; the same credential field
// conflicts on both devices; resolving on B converges both.
func TestS2OfflineMergeAndConflict(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.edit(func(v *domain.VaultData) {
		v.Connections = append(v.Connections, domain.Connection{ID: "c1", Name: "web", Host: "old.example", Port: 22})
	})
	a.sync()
	b := s.enrolled("phone", "correct horse")
	a.edit(func(v *domain.VaultData) { v.Connections[0].Name = "web-a"; v.Connections[0].Host = "a.example" })
	b.edit(func(v *domain.VaultData) { v.Connections[0].Tags = []string{"b"}; v.Connections[0].Host = "b.example" })
	a.sync()
	b.sync()
	a.sync()
	ca, cb := a.data().Connections[0], b.data().Connections[0]
	if ca.Name != "web-a" || len(ca.Tags) != 1 || cb.Name != "web-a" || len(cb.Tags) != 1 {
		t.Fatalf("A %+v, B %+v; different fields must both survive", ca, cb)
	}
	if ca.Host != cb.Host || len(a.data().Sync.Conflicts) != 1 || len(b.data().Sync.Conflicts) != 1 {
		t.Fatalf("hosts %q/%q, conflicts %d/%d; one value in force and the conflict on both",
			ca.Host, cb.Host, len(a.data().Sync.Conflicts), len(b.data().Sync.Conflicts))
	}
	conflicts := usecase.NewSyncConflictService(b.vault)
	list, _ := conflicts.List()
	if err := conflicts.Resolve(ctx, list[0].ID, true); err != nil {
		t.Fatal(err)
	}
	b.sync()
	a.sync()
	if a.data().Connections[0].Host != b.data().Connections[0].Host || len(a.data().Sync.Conflicts) != 0 {
		t.Fatal("the resolution did not converge both devices")
	}
}

// S4: revoking B rotates the account keys and the vault epoch and locks B out.
func TestS4Revoke(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	b := s.enrolled("phone", "correct horse")
	sess, err := a.svc.RevokeDevices(ctx, []vaultsync.ID{b.session.Keys.Device()}, []byte("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	a.session, a.engine = sess, a.newEngine(sess)
	a.sync()
	if _, err := b.engine.Sync(ctx, b.session.Personal); err == nil {
		t.Fatal("the revoked device still syncs")
	}
	if _, err := b.svc.Relogin(ctx, []byte("correct horse")); !errors.Is(err, usecase.ErrSyncDeviceRevoked) {
		t.Fatalf("relogin: %v", err)
	}
	ops, _ := a.session.Server.Membership(ctx, a.session.Personal, 0)
	if last := ops[len(ops)-1]; last.Kind != vaultsync.MemberRotateEpoch || len(last.RevokedDevices) != 1 {
		t.Fatal("the vault epoch was not rotated against the revoked device")
	}
}

// S5: a password change makes the old password fail and B log in again.
func TestS5PasswordChange(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	b := s.enrolled("phone", "correct horse")
	sess, err := a.svc.ChangePassword(ctx, []byte("correct horse"), []byte("new horse staple"))
	if err != nil {
		t.Fatal(err)
	}
	a.session, a.engine = sess, a.newEngine(sess)
	if _, err := s.device("x").svc.Login(ctx, s.device("y").creds("correct horse")); !errors.Is(err, usecase.ErrSyncLogin) {
		t.Fatalf("the old password: %v", err)
	}
	if _, err := b.engine.Sync(ctx, b.session.Personal); !errors.Is(err, synctrust.ErrKeysRotated) {
		t.Fatalf("B: %v, want ErrKeysRotated", err)
	}
	sb, err := b.svc.Relogin(ctx, []byte("new horse staple"))
	if err != nil {
		t.Fatal(err)
	}
	b.session, b.engine = sb, b.newEngine(sb)
	a.edit(func(v *domain.VaultData) {
		v.Folders = append(v.Folders, domain.ConnectionFolder{ID: "after", Name: "after"})
	})
	a.sync()
	b.sync()
	if folder(b.data(), "after") != "after" {
		t.Fatal("B does not follow the vault after logging in again")
	}
}

// S10: disabling RequireSignedPlugins on A waits for B's master password; enabling it applies at once.
func TestS10AsymmetricSettings(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	b := s.enrolled("phone", "correct horse")
	a.edit(func(v *domain.VaultData) { v.Settings.Plugins.RequireSignedPlugins = false })
	a.sync()
	if r := b.sync(); r.Pull.Held != 1 || !b.data().Settings.Plugins.RequireSignedPlugins {
		t.Fatalf("weakening reached B without its master password: %+v", r.Pull)
	}
	b.edit(func(v *domain.VaultData) { v.Settings.Plugins.RequireSignedPlugins = false })
	a.edit(func(v *domain.VaultData) { v.Settings.Plugins.RequireSignedPlugins = true })
	a.sync()
	b.sync()
	if !b.data().Settings.Plugins.RequireSignedPlugins {
		t.Fatal("strengthening did not apply on arrival")
	}
}

// S11: the server stores a commit and dies before answering; the client resends the same commit id
// and the server keeps one commit.
func TestS11CrashBetweenStoreAndReply(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	before := s.commits(a)
	a.edit(func(v *domain.VaultData) { v.Folders = append(v.Folders, domain.ConnectionFolder{ID: "f", Name: "f"}) })
	s.dropNextCommitReply.Store(true)
	if _, err := a.engine.Sync(ctx, a.session.Personal); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	a.sync()
	if got := s.commits(a) - before; got != 1 {
		t.Fatalf("the server holds %d commits for one edit", got)
	}
}

// S14: locking the vault stops sync: the scheduler stops and no request follows.
func TestS14LockDuringSync(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	timers := make(chan chan time.Time, 64)
	sched := usecase.NewSyncScheduler(usecase.SyncSchedulerDeps{Engine: a.engine, Events: nil,
		Vaults: func(context.Context) ([]vaultsync.ID, error) { return []vaultsync.ID{a.session.Personal}, nil },
		After: func(time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			timers <- ch
			return ch
		},
		OnStatus: func(usecase.SyncStatus) {}, Debounce: time.Second, Interval: time.Minute})
	sched.Start(ctx)
	periodic := <-timers
	sched.Stop()
	a.vault.Lock()
	n := s.requests.Load()
	periodic <- time.Time{}
	sched.Notify()
	if s.requests.Load() != n {
		t.Fatal("a request was sent after the vault locked")
	}
}

// S15: a device with its own data joins the account by merging; a backup of its vault is taken
// first, and nothing is duplicated.
func TestS15JoinWithExistingData(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.edit(func(v *domain.VaultData) {
		v.Folders = append(v.Folders, domain.ConnectionFolder{ID: "fa", Name: "account"})
	})
	a.sync()
	b := s.device("phone")
	b.edit(func(v *domain.VaultData) {
		v.Folders = append(v.Folders, domain.ConnectionFolder{ID: "fb", Name: "device"})
	})
	sess, err := b.svc.Login(ctx, b.creds("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	b.session, b.engine = sess, b.newEngine(sess)
	if need, err := b.svc.JoinNeeded(ctx, sess); err != nil || !need {
		t.Fatalf("join needed = %v, %v", need, err)
	}
	name, _, err := b.svc.Join(ctx, sess, usecase.JoinMerge)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "vault", name)); err != nil || !strings.Contains(name, "sync-join") {
		t.Fatalf("backup %q: %v", name, err)
	}
	a.sync()
	for _, d := range []*device{a, b} {
		if f := d.data().Folders; len(f) != 2 || folder(d.data(), "fa") == "" || folder(d.data(), "fb") == "" {
			t.Fatalf("folders = %+v; keep both means both, once", f)
		}
	}
}
