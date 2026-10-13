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

func TestRegisterLoginAndDeviceProof(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	p := h.Principal(u.Access)
	if p.Account != u.Account || p.Device != u.Device || p.Kind != usecase.TokenAccess {
		t.Fatalf("principal = %+v", p)
	}
	id, account, ke2, err := h.Svc.LoginStart(ctx, "alice", "10.0.0.1", []byte("ke1"))
	if err != nil || account != u.Account || len(ke2) != servertest.KE2Size {
		t.Fatalf("login start = %v, %v, %d bytes", account, err, len(ke2))
	}
	res, err := h.Svc.LoginFinish(ctx, id, "10.0.0.1", servertest.CorrectKE3([]byte("ke1"), u.Record))
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Bundle) != "sealed-bundle" || !bytes.Equal(res.IdentityKey, u.AIK.Public().(ed25519.PublicKey)) || res.AccountToken == "" {
		t.Fatalf("login result = %+v", res)
	}
	if _, err := h.Svc.Authenticate(ctx, res.AccountToken, usecase.TokenAccount); err != nil {
		t.Fatalf("account token does not authenticate: %v", err)
	}
	if _, err := h.Svc.Authenticate(ctx, res.AccountToken, usecase.TokenAccess); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatalf("an account token passed as an access token: %v", err)
	}
}

func TestRegistrationRefusals(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	h.Register("alice")
	cases := map[string]func(r *usecase.Registration){
		"bad username":         func(r *usecase.Registration) { r.Username = "A!" },
		"taken username":       func(r *usecase.Registration) { r.Username = "alice" },
		"empty bundle":         func(r *usecase.Registration) { r.Bundle = nil },
		"huge bundle":          func(r *usecase.Registration) { r.Bundle = make([]byte, 5000) },
		"empty record":         func(r *usecase.Registration) { r.Record = nil },
		"device not by AIK":    func(r *usecase.Registration) { r.Device.Sig[0] ^= 1 },
		"vault op not by DSK":  func(r *usecase.Registration) { r.Vault.Sig[0] ^= 1 },
		"vault for another":    func(r *usecase.Registration) { r.Vault.Author = vaultsync.ID{0x77} },
		"short identity key":   func(r *usecase.Registration) { r.IdentityKey = r.IdentityKey[:31] },
		"wrong first log op":   func(r *usecase.Registration) { r.Device.Op = vaultsync.OpRevokeDevice },
		"unused invite reused": nil,
	}
	for name, mutate := range cases {
		u := servertest.NewUser(t, "bob")
		inv := h.Invite()
		r := h.Registration(u, inv)
		if mutate == nil {
			if err := h.Svc.RegisterFinish(ctx, r); err != nil {
				t.Fatalf("baseline registration failed: %v", err)
			}
			again := servertest.NewUser(t, "carol")
			r2 := h.Registration(again, inv)
			if err := h.Svc.RegisterFinish(ctx, r2); !isErr(err, usecase.ErrForbidden) {
				t.Errorf("%s: err = %v, want ErrForbidden", name, err)
			}
			continue
		}
		mutate(&r)
		if err := h.Svc.RegisterFinish(ctx, r); err == nil {
			t.Errorf("%s: registration accepted", name)
		}
		if _, _, _, err := h.Svc.LoginStart(ctx, r.Username, "1.1.1.1", []byte("k")); err != nil && !isErr(err, usecase.ErrRateLimited) {
			t.Errorf("%s: login start errored: %v", name, err)
		}
	}
}

func TestRefusedRegistrationLeavesNoAccount(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := servertest.NewUser(t, "bob")
	inv := h.Invite()
	r := h.Registration(u, inv)
	r.Vault.Sig[0] ^= 1
	if err := h.Svc.RegisterFinish(ctx, r); err == nil {
		t.Fatal("tampered registration accepted")
	}
	accounts, _ := h.Svc.ListAccounts(ctx)
	if len(accounts) != 0 {
		t.Fatalf("a refused registration left %d account rows", len(accounts))
	}
	fresh := servertest.NewUser(t, "bob")
	if err := h.Svc.RegisterFinish(ctx, h.Registration(fresh, inv)); err != nil {
		t.Fatalf("the invite was spent by a refused registration: %v", err)
	}
}

func TestRegisterStartChecks(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := servertest.NewUser(t, "alice")
	if _, err := h.Svc.RegisterStart(ctx, "not-an-invite", u.Name, u.Account, []byte("b")); !isErr(err, usecase.ErrForbidden) {
		t.Errorf("unknown invite: err = %v", err)
	}
	inv := h.Invite()
	h.Clock.Advance(3_600_001)
	if _, err := h.Svc.RegisterStart(ctx, inv, u.Name, u.Account, []byte("b")); !isErr(err, usecase.ErrForbidden) {
		t.Errorf("expired invite: err = %v", err)
	}
	inv = h.Invite()
	if _, err := h.Svc.RegisterStart(ctx, inv, u.Name, vaultsync.ID{}, []byte("b")); !isErr(err, usecase.ErrMalformed) {
		t.Errorf("zero account id: err = %v", err)
	}
}

func TestUnknownAndFrozenUsersLookLikeWrongPasswords(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	_, real, ke2Real, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	_, fake1, ke2Fake, err := h.Svc.LoginStart(ctx, "nobody", "ip", []byte("k"))
	if err != nil || len(ke2Fake) != len(ke2Real) || fake1 == real || fake1.IsZero() {
		t.Fatalf("unknown user reply: %v, %v, %d bytes", fake1, err, len(ke2Fake))
	}
	_, fake2, _, _ := h.Svc.LoginStart(ctx, "nobody", "ip2", []byte("k"))
	if fake1 != fake2 {
		t.Fatal("the fake account id changes between probes; a stable real id would stand out against it")
	}
	if err := h.Svc.SetFrozen(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	id, frozenAcct, _, _ := h.Svc.LoginStart(ctx, "alice", "ip3", []byte("k"))
	if frozenAcct == u.Account {
		t.Fatal("a frozen account answers with its real id; freezing must not be observable")
	}
	if _, err := h.Svc.LoginFinish(ctx, id, "ip3", servertest.CorrectKE3([]byte("k"), u.Record)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatalf("frozen login finished: %v", err)
	}
}

func TestWrongPasswordAndReplayedLogin(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	id, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	if _, err := h.Svc.LoginFinish(ctx, id, "ip", []byte("wrong")); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := h.Svc.LoginFinish(ctx, id, "ip", servertest.CorrectKE3([]byte("k"), u.Record)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a login id was usable twice")
	}
	id2, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	h.Clock.Advance(120_001)
	if _, err := h.Svc.LoginFinish(ctx, id2, "ip", servertest.CorrectKE3([]byte("k"), u.Record)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("an expired pending login finished")
	}
}

func TestLoginBackoffWithoutLockout(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	fail := func(ip string) error {
		id, _, _, err := h.Svc.LoginStart(ctx, "alice", ip, []byte("k"))
		if err != nil {
			return err
		}
		_, err = h.Svc.LoginFinish(ctx, id, ip, []byte("wrong"))
		return err
	}
	for i := 0; i < 3; i++ {
		if err := fail("bad"); !isErr(err, usecase.ErrUnauthorized) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "bad", []byte("k")); !isErr(err, usecase.ErrRateLimited) {
		t.Fatalf("fourth attempt from one address: %v, want ErrRateLimited", err)
	}
	id, _, _, err := h.Svc.LoginStart(ctx, "alice", "good", []byte("k"))
	if err != nil {
		t.Fatalf("the owner from another address is locked out by someone else's failures: %v", err)
	}
	if _, err := h.Svc.LoginFinish(ctx, id, "good", servertest.CorrectKE3([]byte("k"), u.Record)); err != nil {
		t.Fatal(err)
	}
	h.Clock.Advance(1_000)
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "bad", []byte("k")); err != nil {
		t.Fatalf("backoff did not expire after one second: %v", err)
	}
}

func TestDeviceProofRules(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	nonce, _ := h.Svc.DeviceChallenge(ctx, u.Account, u.Device)
	msg, _ := vaultsync.DeviceAuthMessage(u.Account, u.Device, nonce)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if _, _, err := h.Svc.DeviceVerify(ctx, u.Account, u.Device, nonce, ed25519.Sign(other, msg)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, _, err := h.Svc.DeviceVerify(ctx, u.Account, u.Device, nonce, ed25519.Sign(u.DSK, msg)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a nonce was usable after a failed attempt")
	}
	nonce, _ = h.Svc.DeviceChallenge(ctx, u.Account, u.Device)
	h.Clock.Advance(60_001)
	msg, _ = vaultsync.DeviceAuthMessage(u.Account, u.Device, nonce)
	if _, _, err := h.Svc.DeviceVerify(ctx, u.Account, u.Device, nonce, ed25519.Sign(u.DSK, msg)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("an expired nonce was accepted")
	}
	if _, err := h.Svc.DeviceChallenge(ctx, u.Account, vaultsync.ID{0x42}); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatalf("challenge for an uncertified device: %v", err)
	}
	h.Clock.Advance(15 * 60_000)
	if _, err := h.Svc.Authenticate(ctx, u.Access, usecase.TokenAccess); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("an access token outlived its 15 minutes")
	}
}

func TestDeviceLogAppendRevokeAndSuccession(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	p := h.Principal(u.Access)
	second := servertest.RandomID(t)
	_, dsk2, _ := ed25519.GenerateKey(rand.Reader)
	if err := h.Svc.AppendDeviceLog(ctx, p, []vaultsync.DeviceEntry{u.AddDeviceEntry(t, second, dsk2)}); err != nil {
		t.Fatal(err)
	}
	token2 := h.DeviceLogin(u, second, dsk2)

	revoke := u.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: second})
	if err := h.Svc.AppendDeviceLog(ctx, p, []vaultsync.DeviceEntry{revoke}); !isErr(err, usecase.ErrMalformed) {
		t.Fatalf("a revocation without succession was accepted: %v", err)
	}
	_, newAIK, _ := ed25519.GenerateKey(rand.Reader)
	succ := vaultsync.DeviceEntry{Account: u.Account, Seq: revoke.Seq + 1, Op: vaultsync.OpSuccession, NewSuite: vaultsync.Suite1,
		NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: bytes.Repeat([]byte{4}, 1216), SigAlg: vaultsync.SigEd25519}
	succ.Prev = u.Log.Hash()
	msg, _ := succ.SigningBytes()
	succ.Sig, succ.SuccessorSig = ed25519.Sign(u.AIK, msg), ed25519.Sign(newAIK, msg)
	if err := h.Svc.AppendDeviceLog(ctx, p, []vaultsync.DeviceEntry{revoke, succ}); err != nil {
		t.Fatalf("revoke + succession refused: %v", err)
	}
	if _, err := h.Svc.Authenticate(ctx, token2, usecase.TokenAccess); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a revoked device's token still authenticates")
	}
	if _, err := h.Svc.DeviceChallenge(ctx, u.Account, second); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a revoked device can still ask for a challenge")
	}
	look, err := h.Svc.Lookup(ctx, p, "alice")
	if err != nil || !bytes.Equal(look.EncryptionKey, bytes.Repeat([]byte{4}, 1216)) || look.DeviceLogSeq != succ.Seq {
		t.Fatalf("lookup after succession = %+v, %v; it must serve the current encryption key", look, err)
	}
	if !bytes.Equal(look.IdentityKey, u.AIK.Public().(ed25519.PublicKey)) {
		t.Fatal("lookup must serve the genesis identity key: fingerprints are computed over it")
	}
	if !bytes.Equal(look.GenesisEncryptionKey, u.AEK) || len(look.Entries) != int(succ.Seq) {
		t.Fatalf("lookup serves %d entries and genesis key %x...; an inviter replays the whole log to trust the current key", len(look.Entries), look.GenesisEncryptionKey[:4])
	}
}

func TestDeviceLogReadRules(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	a := h.Register("alice")
	b := h.Register("bob")
	pa := h.Principal(a.Access)
	entries, err := h.Svc.DeviceLog(ctx, pa, a.Account, 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("own log = %d, %v", len(entries), err)
	}
	if _, err := h.Svc.DeviceLog(ctx, pa, b.Account, 0); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("reading a stranger's device log: %v, want ErrNotFound", err)
	}
	later, _ := h.Svc.DeviceLog(ctx, pa, a.Account, 1)
	if len(later) != 0 {
		t.Fatalf("entries after seq 1 = %d", len(later))
	}
}

func TestFrozenAccountLosesAccess(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := h.Register("alice")
	if err := h.Svc.SetFrozen(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Svc.Authenticate(ctx, u.Access, usecase.TokenAccess); !isErr(err, usecase.ErrFrozen) {
		t.Fatalf("frozen account authenticated: %v", err)
	}
	if err := h.Svc.SetFrozen(ctx, "nobody", true); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("freezing an unknown account: %v", err)
	}
	if err := h.Svc.DeleteAccount(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Svc.Authenticate(ctx, u.Access, usecase.TokenAccess); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a deleted account's token still authenticates")
	}
}
