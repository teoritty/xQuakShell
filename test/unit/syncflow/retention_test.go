package syncflow

import (
	"bytes"
	"encoding/base64"
	"os"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/synccrypto"
)

const day = int64(24 * 60 * 60 * 1000)

// advance moves the server clock and every device clock together by days.
func (s *server) advance(days int64, ds ...*device) {
	s.h.Clock.Advance(uint64(days * day))
	for _, d := range ds {
		d.now.Add(days * day)
	}
}

func addFolder(d *device, id, name string) {
	d.edit(func(v *domain.VaultData) { v.Folders = append(v.Folders, domain.ConnectionFolder{ID: id, Name: name}) })
}

func dropFolder(d *device, id string) {
	d.edit(func(v *domain.VaultData) {
		for i, f := range v.Folders {
			if f.ID == id {
				v.Folders = append(v.Folders[:i], v.Folders[i+1:]...)
				return
			}
		}
	})
}

// trashed returns the record id of the one item in d's trash.
func trashed(t *testing.T, d *device) vaultsync.ID {
	t.Helper()
	items, err := d.engine.Trash(ctx, d.session.Personal)
	if err != nil || len(items) != 1 || !items[0].Restorable {
		t.Fatalf("trash = %+v, %v; want one restorable item", items, err)
	}
	return items[0].Record
}

// fileHolds reports whether the server's database file or its WAL still carries b.
func (s *server) fileHolds(b []byte) bool {
	s.t.Helper()
	if err := s.h.DB.Checkpoint(ctx); err != nil {
		s.t.Fatal(err)
	}
	needle := []byte(base64.StdEncoding.EncodeToString(b))
	for _, p := range []string{s.h.DBPath, s.h.DBPath + "-wal"} {
		content, err := os.ReadFile(p)
		if err == nil && (bytes.Contains(content, needle) || bytes.Contains(content, b)) {
			return true
		}
	}
	return false
}

func (s *server) epochZeroWrap(d *device) vaultsync.KeyWrap {
	s.t.Helper()
	ops, err := s.h.Svc.Membership(ctx, s.h.Principal(s.tokenOf(d)), d.session.Personal, 0)
	if err != nil {
		s.t.Fatal(err)
	}
	return ops[0].Wraps[0]
}

// S3a: delete versus edit keeps the edit; the trash restores; the window passes on the fake clock
// and the purge removes the record's ciphertext from the server's file; after one and a half
// windows of the deletion the first epoch has retired and its wrap is stripped on the server.
func TestS3aTrashPurgeAndRetirement(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	addFolder(a, "f1", "Prod")
	a.sync()
	b := s.enrolled("phone", "correct horse")

	dropFolder(a, "f1")
	b.edit(func(v *domain.VaultData) { v.Folders[0].Name = "Production" })
	a.sync()
	b.sync()
	a.sync()
	if folder(a.data(), "f1") != "Production" || folder(b.data(), "f1") != "Production" {
		t.Fatal("an edit racing a delete must win on both devices")
	}

	dropFolder(a, "f1")
	a.sync()
	if err := a.engine.Restore(ctx, a.session.Personal, trashed(t, a)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	a.sync()
	b.sync()
	if folder(b.data(), "f1") != "Production" {
		t.Fatal("the restore did not reach the other device")
	}

	dropFolder(a, "f1")
	a.sync()
	b.sync()
	id := trashed(t, a)
	versions, err := a.session.Server.Versions(ctx, a.session.Personal, id)
	if err != nil {
		t.Fatal(err)
	}
	var sealed []byte
	for _, v := range versions {
		if !v.Deleted {
			sealed = v.Body
		}
	}
	if len(sealed) == 0 || !s.fileHolds(sealed) {
		t.Fatal("the fixture is wrong: the live version's ciphertext should be on the server before the purge")
	}

	// The devices sync every couple of weeks, as devices in use do; each round may rotate.
	s.advance(15, a, b)
	a.sync()
	b.sync()
	s.advance(16, a, b)
	a.sync()
	b.sync()
	if left, _ := a.session.Server.Versions(ctx, a.session.Personal, id); len(left) != 0 {
		t.Fatalf("%d versions of the purged record are still served", len(left))
	}
	if s.fileHolds(sealed) {
		t.Fatal("the purged record's ciphertext is still in the server's database file")
	}

	s.advance(15, a, b)
	a.sync()
	b.sync()
	if !s.epochZeroWrap(a).Pruned() {
		t.Fatal("one and a half windows after the purge, the first epoch's key wrap still carries its ciphertext")
	}
}

// S3b: Shred now retires the old epochs at once; changing the sync password afterwards makes even a
// wrap copied from an older database unopenable with the account's current credentials (G5, R1).
func TestS3bShredNowAndThePasswordChange(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	addFolder(a, "f1", "Prod")
	addFolder(a, "f2", "Dev")
	a.sync()
	b := s.enrolled("phone", "correct horse")
	dropFolder(a, "f1")
	a.sync()
	oldCopy := s.epochZeroWrap(a)

	if err := a.engine.ShredNow(ctx, a.session.Personal); err != nil {
		t.Fatalf("shred now: %v", err)
	}
	b.sync()
	if items, _ := a.engine.Trash(ctx, a.session.Personal); len(items) != 0 {
		t.Fatalf("trash after shred now = %+v", items)
	}
	if !s.epochZeroWrap(a).Pruned() {
		t.Fatal("shred now left the old epoch's wrap on the server")
	}
	if folder(b.data(), "f2") != "Dev" {
		t.Fatal("shred now lost a live folder on the other device")
	}

	acct := a.account()
	unwrap := func(seed []byte) error {
		_, err := synccrypto.Account{}.UnwrapVaultKey(seed, a.session.Personal, 0, vaultsync.ID(acct.Account), oldCopy.Ciphertext)
		return err
	}
	if unwrap(acct.EncryptionSeed) != nil {
		t.Fatal("the fixture is wrong: before a password change the old copy opens with current credentials (residual R1)")
	}
	sess, err := a.svc.ChangePassword(ctx, []byte("correct horse"), []byte("new horse"))
	if err != nil {
		t.Fatal(err)
	}
	a.session, a.engine = sess, a.newEngine(sess)
	if unwrap(a.account().EncryptionSeed) == nil {
		t.Fatal("after the password change a wrap from an older database still opens with the current credentials")
	}
}
