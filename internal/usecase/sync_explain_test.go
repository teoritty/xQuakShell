package usecase_test

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// A device revoked while a round is under way reads the device log from before the revocation and
// the vault from after it, re-encrypted to keys it never received. That failed verification, and
// the device told its person the server was tampering - for a removal made on their own other
// device. The round now looks at the log again and says what happened.
func TestARoundThatFailsBecauseTheDeviceWasRemovedSaysSo(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	w.pull()
	w.push(w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	revoke := w.a.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: w.a.Device})
	w.server.tamper = func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
		// From here on the log carries the revocation; this round's own log fetch came before it.
		w.server.tamperLog = func(es []vaultsync.DeviceEntry) []vaultsync.DeviceEntry { return append(es, revoke) }
		for i := range cs {
			cs[i].Commit.Head.Seq += 7
		}
		return cs
	}
	_, err := w.engine.Sync(syncCtx, w.cv.ID)
	if !errors.Is(err, usecase.ErrSyncDeviceRevoked) {
		t.Fatalf("err = %v; a device removed mid-round is told it was removed", err)
	}
	if !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v; the verification failure is kept in the error, not erased", err)
	}
}

// Without an account-level reason a verification failure stays exactly that.
func TestAVerificationFailureWithoutAReasonStaysOne(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	w.pull()
	w.push(w.item(synctrust.TypeFolder, "f1", domain.ConnectionFolder{ID: "f1", Name: "Prod"}))
	w.server.tamper = func(cs []synctrust.RemoteCommit) []synctrust.RemoteCommit {
		for i := range cs {
			cs[i].Commit.Head.Seq += 7
		}
		return cs
	}
	_, err := w.engine.Sync(syncCtx, w.cv.ID)
	if !errors.Is(err, synctrust.ErrIntegrity) || errors.Is(err, usecase.ErrSyncDeviceRevoked) || errors.Is(err, synctrust.ErrKeysRotated) {
		t.Fatalf("err = %v; tampering must read as tampering", err)
	}
}

// A device the log revokes learns so before anything else the log says.
func TestARevokedDeviceIsToldBeforeAnythingElse(t *testing.T) {
	w := newSyncWorld(t)
	w.push(w.meta())
	revoke := w.a.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: w.a.Device})
	w.server.tamperLog = func(es []vaultsync.DeviceEntry) []vaultsync.DeviceEntry { return append(es, revoke) }
	if _, err := w.engine.Pull(syncCtx, w.cv.ID); !errors.Is(err, usecase.ErrSyncDeviceRevoked) {
		t.Fatalf("err = %v; want ErrSyncDeviceRevoked", err)
	}
}
