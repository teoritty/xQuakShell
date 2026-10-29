package syncadversary

import (
	"crypto/rand"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/transport"
	srvusecase "xquakshell/internal/syncserver/usecase"
)

// forkedServer is a second real server over a copy of w's database taken now: the other branch a
// hostile operator keeps. It proves itself with w's identity and TLS keys, so a client cannot tell
// which of the two it is talking to.
func forkedServer(t *testing.T, w *world) http.Handler {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fork.db")
	if err := w.h.DB.BackupTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := srvusecase.New(srvusecase.Config{Store: db, Clock: w.h.Clock, AKE: servertest.FakeAKE{}, Random: rand.Reader,
		FakeAccountSecret: []byte("fake-secret")})
	h, err := transport.New(transport.Config{IdentitySeed: w.seed, TLSPublicKey: w.spki, RatePerSecond: 100_000, Burst: 100_000}, svc)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func hasFolder(d *device, id string) bool {
	v, _ := d.vault.GetData()
	for _, f := range v.Folders {
		if f.ID == id {
			return true
		}
	}
	return false
}

// forkedPair splits one account's history in two: A commits folder x on the real branch, then the
// server shows B the other branch, where B commits y and z on top of the head both started from.
// The server then serves A from B's branch, which is one commit longer than A's.
func forkedPair(t *testing.T) (*world, *device) {
	t.Helper()
	w, a, b := pair(t)
	fork := forkedServer(t, w)
	a.addFolder("x", "written on A")
	a.sync()
	w.inner = fork
	b.addFolder("y", "written on B")
	b.sync()
	b.addFolder("z", "written on B again")
	b.sync()
	if !hasFolder(a, "x") || a.head() >= b.head() {
		t.Fatalf("setup: A at %d with x=%v, B at %d; B's branch must be the longer one", a.head(), hasFolder(a, "x"), b.head())
	}
	return w, a
}

func TestAForkIsDetectedWhenTheBranchesMeet(t *testing.T) {
	_, a := forkedPair(t)
	if _, err := a.engine.Sync(ctx, a.session.Personal); !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; a branch that does not extend the verified head is an integrity failure", err)
	}
}

// Claiming the history before the other branch was pruned moves A onto the snapshot instead of the
// commits. The snapshot is newer than A's cursor and verifies on its own; only the heads between the
// two show it is another branch. Without that check A adopted it silently, and folder x - which A
// pushed itself - landed as a purged deletion.
func TestAForkIsNotHiddenBehindPrunedHistory(t *testing.T) {
	w, a := forkedPair(t)
	cursor := a.head()
	w.attack(func(r *http.Request, status int, body []byte) (int, []byte) {
		if strings.HasSuffix(r.URL.Path, "/changes") {
			return http.StatusGone, []byte(`{"error":"` + syncwire.CodeHistoryPruned + `"}`)
		}
		return status, body
	})
	_, err := a.engine.Sync(ctx, a.session.Personal)
	if !errors.Is(err, synctrust.ErrIntegrity) || !errors.Is(err, vaultsync.ErrFork) {
		t.Fatalf("err = %v; a snapshot on another branch must raise the fork alarm", err)
	}
	if a.head() != cursor {
		t.Fatalf("cursor moved from %d to %d; a fork must leave the verified chain where it was", cursor, a.head())
	}
	if !hasFolder(a, "x") || hasFolder(a, "y") {
		t.Fatal("the vault took the other branch's state; A's own folder must survive a refused fork")
	}
}
