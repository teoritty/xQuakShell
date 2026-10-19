package syncflow

import (
	"crypto/rand"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// A password change or a revocation elsewhere is routine, and the other device must say so - not
// report the server as tampering. Every vault the device syncs, the team ones included, has to
// stop with the account-level reason, because the status a person reads is built from all of them.
func TestKeyChangesElsewhereAreNotReportedAsTampering(t *testing.T) {
	s := newServer(t)
	a := s.registered()
	a.sync()
	b := s.enrolled("phone", "correct horse")
	team, err := a.engine.CreateTeam(ctx, "Ops", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a.edit(func(v *domain.VaultData) {
		v.Connections = append(v.Connections, domain.Connection{ID: "c1", Name: "db", Host: "db.example", Port: 22})
	})
	if _, err := a.engine.MoveConnection(ctx, usecase.MoveConnectionRequest{Connection: "c1", To: team.String()}); err != nil {
		t.Fatal(err)
	}
	if err := a.syncVault(team); err != nil {
		t.Fatal(err)
	}
	// B learns its account joined the team from the personal vault, which every round syncs first:
	// A's round pushes the join, B's pulls it.
	a.sync()
	b.sync()
	if err := b.syncVault(team); err != nil {
		t.Fatalf("B reads its own account's team: %v", err)
	}

	sess, err := a.svc.ChangePassword(ctx, []byte("correct horse"), []byte("new horse staple"))
	if err != nil {
		t.Fatal(err)
	}
	a.session, a.engine = sess, a.newEngine(sess)
	for _, v := range []vaultsync.ID{b.session.Personal, team} {
		if err := b.syncVault(v); !errors.Is(err, synctrust.ErrKeysRotated) || errors.Is(err, synctrust.ErrIntegrity) {
			t.Fatalf("vault %s after a password change elsewhere: %v; want ErrKeysRotated and no integrity failure", v, err)
		}
	}
	if _, err := b.engine.Vaults(ctx); errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("listing vaults after a password change elsewhere: %v; a rotation is not tampering", err)
	}

	sb, err := b.svc.Relogin(ctx, []byte("new horse staple"))
	if err != nil {
		t.Fatal(err)
	}
	b.session, b.engine = sb, b.newEngine(sb)
	if _, err := a.svc.RevokeDevices(ctx, []vaultsync.ID{b.session.Keys.Device()}, []byte("new horse staple")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []vaultsync.ID{b.session.Personal, team} {
		if err := b.syncVault(v); !errors.Is(err, usecase.ErrSyncDeviceRevoked) {
			t.Fatalf("vault %s on a revoked device: %v; want ErrSyncDeviceRevoked, so the device is told it was removed", v, err)
		}
	}
}
