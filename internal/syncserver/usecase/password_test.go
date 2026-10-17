package usecase_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/usecase"
)

func rekeySig(t *testing.T, key ed25519.PrivateKey, account vaultsync.ID, record, bundle []byte) []byte {
	t.Helper()
	msg, err := vaultsync.RekeyMessage(account, record, bundle)
	if err != nil {
		t.Fatal(err)
	}
	return ed25519.Sign(key, msg)
}

func TestPasswordRekeyReplacesRecordAndBundle(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	_, dsk2, _ := ed25519.GenerateKey(rand.Reader)
	second := servertest.RandomID(t)
	if err := h.Svc.AppendDeviceLog(ctx, h.Principal(alice.Access), []vaultsync.DeviceEntry{alice.AddDeviceEntry(t, second, dsk2)}); err != nil {
		t.Fatal(err)
	}
	token2 := h.DeviceLogin(alice, second, dsk2)
	lid, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	pwSession, err := h.Svc.LoginFinish(ctx, lid, "ip", servertest.CorrectKE3([]byte("k"), alice.Record))
	if err != nil {
		t.Fatal(err)
	}
	p := h.Principal(alice.Access)
	resp, err := h.Svc.PasswordStart(ctx, p, []byte("blinded-new"))
	if err != nil || !bytes.HasSuffix(resp, []byte("blinded-new")) {
		t.Fatalf("password start = %q, %v", resp, err)
	}
	record, bundle := []byte("new-record"), []byte("new-bundle")
	if err := h.Svc.PasswordFinish(ctx, p, record, bundle, rekeySig(t, alice.AIK, alice.Account, record, bundle)); err != nil {
		t.Fatal(err)
	}
	id, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	res, err := h.Svc.LoginFinish(ctx, id, "ip", servertest.CorrectKE3([]byte("k"), record))
	if err != nil || !bytes.Equal(res.Bundle, bundle) {
		t.Fatalf("login after rekey = %+v, %v; the new record and bundle must be in force", res, err)
	}
	if _, err := h.Svc.Authenticate(ctx, token2, usecase.TokenAccess); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("another device's session survived a password change")
	}
	if _, err := h.Svc.Authenticate(ctx, pwSession.AccountToken, usecase.TokenAccount); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a password session from before the change survived it")
	}
	if _, err := h.Svc.Authenticate(ctx, alice.Access, usecase.TokenAccess); err != nil {
		t.Fatalf("the changing device lost its own session: %v", err)
	}
}

func TestPasswordRekeyNeedsTheCurrentAccountKey(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	bob := h.Register("bob")
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	record, bundle := []byte("evil-record"), []byte("evil-bundle")
	for name, c := range map[string]struct {
		p   usecase.Principal
		sig []byte
	}{
		"a session without the account key": {h.Principal(alice.Access), rekeySig(t, stranger, alice.Account, record, bundle)},
		"another account's key":             {h.Principal(alice.Access), rekeySig(t, bob.AIK, alice.Account, record, bundle)},
		"a consent for another account":     {h.Principal(alice.Access), rekeySig(t, alice.AIK, bob.Account, record, bundle)},
		"a consent for another bundle":      {h.Principal(alice.Access), rekeySig(t, alice.AIK, alice.Account, record, []byte("other"))},
		"bob's session with alice's key":    {h.Principal(bob.Access), rekeySig(t, alice.AIK, alice.Account, record, bundle)},
	} {
		if err := h.Svc.PasswordFinish(ctx, c.p, record, bundle, c.sig); !isErr(err, usecase.ErrUnauthorized) {
			t.Errorf("%s: err = %v, want ErrUnauthorized", name, err)
		}
	}
	id, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	if _, err := h.Svc.LoginFinish(ctx, id, "ip", servertest.CorrectKE3([]byte("k"), alice.Record)); err != nil {
		t.Fatalf("a refused rekey changed alice's record: %v", err)
	}
}

// After a succession the old identity key is superseded; whoever still holds it - the revoked
// device - can no longer replace the bundle.
func TestPasswordRekeyRefusesASupersededKey(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	p := h.Principal(alice.Access)
	second := servertest.RandomID(t)
	_, dsk2, _ := ed25519.GenerateKey(rand.Reader)
	add := alice.AddDeviceEntry(t, second, dsk2)
	revoke := alice.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: second})
	_, newAIK, _ := ed25519.GenerateKey(rand.Reader)
	succ := vaultsync.DeviceEntry{Account: alice.Account, Seq: revoke.Seq + 1, Prev: alice.Log.Hash(), Op: vaultsync.OpSuccession,
		NewSuite: vaultsync.Suite1, NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: bytes.Repeat([]byte{4}, 1216),
		SigAlg: vaultsync.SigEd25519}
	msg, _ := succ.SigningBytes()
	succ.Sig, succ.SuccessorSig = ed25519.Sign(alice.AIK, msg), ed25519.Sign(newAIK, msg)
	if err := h.Svc.AppendDeviceLog(ctx, p, []vaultsync.DeviceEntry{add, revoke, succ}); err != nil {
		t.Fatal(err)
	}
	record, bundle := []byte("r"), []byte("b")
	if err := h.Svc.PasswordFinish(ctx, p, record, bundle, rekeySig(t, alice.AIK, alice.Account, record, bundle)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatalf("superseded key: %v, want ErrUnauthorized", err)
	}
	if err := h.Svc.PasswordFinish(ctx, p, record, bundle, rekeySig(t, newAIK, alice.Account, record, bundle)); err != nil {
		t.Fatalf("the successor key was refused: %v", err)
	}
}

func TestPasswordRekeyRefusesMalformedAndFrozen(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	p := h.Principal(alice.Access)
	if _, err := h.Svc.PasswordStart(ctx, p, nil); !isErr(err, usecase.ErrMalformed) {
		t.Errorf("empty request: %v", err)
	}
	big := make([]byte, 4097)
	if err := h.Svc.PasswordFinish(ctx, p, []byte("r"), big, rekeySig(t, alice.AIK, alice.Account, []byte("r"), big)); !isErr(err, usecase.ErrMalformed) {
		t.Errorf("oversize bundle: %v", err)
	}
	if err := h.Svc.PasswordFinish(ctx, p, nil, []byte("b"), rekeySig(t, alice.AIK, alice.Account, nil, []byte("b"))); !isErr(err, usecase.ErrMalformed) {
		t.Errorf("empty record: %v", err)
	}
	if err := h.Svc.SetFrozen(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Svc.PasswordStart(ctx, p, []byte("x")); err == nil {
		t.Error("a frozen account started a password change")
	}
}
