package usecase_test

import (
	"strings"
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/usecase"
)

func TestUsernameRules(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	for name, ok := range map[string]bool{
		"abc": true, strings.Repeat("a", 64): true, "a.b_c-9": true,
		"ab": false, strings.Repeat("a", 65): false, "Abc": false, "a b": false, "a/b": false, "ab@": false, "äbc": false,
	} {
		u := servertest.NewUser(t, "x")
		_, err := h.Svc.RegisterStart(ctx, h.Invite(), name, u.Account, []byte("b"))
		if ok && err != nil {
			t.Errorf("username %q refused: %v", name, err)
		}
		if !ok && !isErr(err, usecase.ErrMalformed) {
			t.Errorf("username %q accepted (err %v); the narrow alphabet keeps look-alikes out of the admin's listing", name, err)
		}
	}
}

func TestBundleSizeBoundary(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	u := servertest.NewUser(t, "alice")
	inv := h.Invite()
	r := h.Registration(u, inv)
	r.Bundle = make([]byte, 4096)
	if err := h.Svc.RegisterFinish(ctx, r); err != nil {
		t.Fatalf("a bundle of exactly 4096 bytes was refused: %v", err)
	}
}

func TestDeleteUnknownAccount(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	if err := h.Svc.DeleteAccount(ctx, "nobody"); !isErr(err, usecase.ErrNotFound) {
		t.Fatalf("deleting an unknown account: %v, want ErrNotFound", err)
	}
}

func TestRecordCapBoundary(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.PersonalVault(alice)
	c := cv.Commit(alice, cv.Record(alice, 1))
	c.Records = make([]vaultsync.Envelope, 1000)
	if _, err := cv.Push(alice, c); isErr(err, usecase.ErrTooLarge) {
		t.Fatal("exactly 1000 records were refused as too large")
	}
}

func TestQuotaBoundary(t *testing.T) {
	h := servertest.NewHarness(t, 368)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1))) // 72 + 296 bytes: exactly the quota
}

func TestChangesPaging(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.PersonalVault(alice)
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 2)))
	p := h.Principal(alice.Access)
	page, more, _ := h.Svc.Changes(ctx, p, cv.ID, 0, 2)
	if len(page) != 2 || more {
		t.Fatalf("a page exactly as long as the remaining commits: %d, more %v", len(page), more)
	}
	page, more, _ = h.Svc.Changes(ctx, p, cv.ID, 0, 1)
	if len(page) != 1 || !more || page[0].Seq != 1 {
		t.Fatalf("a page of one with one more to come: %d, more %v", len(page), more)
	}
	page, _, _ = h.Svc.Changes(ctx, p, cv.ID, 0, 0)
	if len(page) != 1 {
		t.Fatalf("a non-positive limit returned %d commits, want it treated as 1", len(page))
	}
}

func TestDeviceLogBatchCap(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	if err := h.Svc.AppendDeviceLog(ctx, h.Principal(alice.Access), nil); !isErr(err, usecase.ErrMalformed) {
		t.Errorf("empty batch: %v", err)
	}
	batch := make([]vaultsync.DeviceEntry, 17)
	if err := h.Svc.AppendDeviceLog(ctx, h.Principal(alice.Access), batch); !isErr(err, usecase.ErrMalformed) {
		t.Errorf("17 entries: %v, want ErrMalformed", err)
	}
	var sixteen []vaultsync.DeviceEntry
	for i := 0; i < 16; i++ {
		_, dsk, _ := ed25519Key()
		sixteen = append(sixteen, alice.AddDeviceEntry(t, servertest.RandomID(t), dsk))
	}
	if err := h.Svc.AppendDeviceLog(ctx, h.Principal(alice.Access), sixteen); err != nil {
		t.Errorf("exactly 16 entries were refused: %v", err)
	}
}

func TestMembershipBatchCap(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.NewTeamVault(alice)
	var ops []vaultsync.MemberOp
	for i := 0; i < 16; i++ {
		ops = append(ops, vaultsync.MemberOp{Kind: vaultsync.MemberSetPolicy, RetentionDays: uint16(i + 1)})
	}
	if err := cv.AppendOps(alice, ops...); err != nil {
		t.Fatalf("exactly 16 ops were refused: %v", err)
	}
	if err := cv.AppendOps(alice, append(ops, ops[0])...); !isErr(err, usecase.ErrMalformed) {
		t.Fatalf("17 ops: %v, want ErrMalformed", err)
	}
}

func TestTokenExpiresAtItsInstant(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	h.Clock.Advance(15*60_000 - 1)
	if _, err := h.Svc.Authenticate(ctx, alice.Access, usecase.TokenAccess); err != nil {
		t.Fatalf("a token one millisecond before expiry was refused: %v", err)
	}
	h.Clock.Advance(1)
	if _, err := h.Svc.Authenticate(ctx, alice.Access, usecase.TokenAccess); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a token at its expiry instant still authenticates")
	}
}

func TestChallengeExpiresAtItsInstant(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	nonce, _ := h.Svc.DeviceChallenge(ctx, alice.Account, alice.Device)
	h.Clock.Advance(60_000 - 1)
	msg, _ := vaultsync.DeviceAuthMessage(alice.Account, alice.Device, nonce)
	if _, _, err := h.Svc.DeviceVerify(ctx, alice.Account, alice.Device, nonce, sign(alice, msg)); err != nil {
		t.Fatalf("a nonce one millisecond before expiry was refused: %v", err)
	}
	nonce, _ = h.Svc.DeviceChallenge(ctx, alice.Account, alice.Device)
	h.Clock.Advance(60_000)
	msg, _ = vaultsync.DeviceAuthMessage(alice.Account, alice.Device, nonce)
	if _, _, err := h.Svc.DeviceVerify(ctx, alice.Account, alice.Device, nonce, sign(alice, msg)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a nonce at its expiry instant was accepted")
	}
}

func TestPendingLoginExpiresAtItsInstant(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	id, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	h.Clock.Advance(120_000 - 1)
	if _, err := h.Svc.LoginFinish(ctx, id, "ip", servertest.CorrectKE3([]byte("k"), alice.Record)); err != nil {
		t.Fatalf("a login finished one millisecond before expiry was refused: %v", err)
	}
	id, _, _, _ = h.Svc.LoginStart(ctx, "alice", "ip", []byte("k"))
	h.Clock.Advance(120_000)
	if _, err := h.Svc.LoginFinish(ctx, id, "ip", servertest.CorrectKE3([]byte("k"), alice.Record)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a login finished at its expiry instant")
	}
}

func TestLoginFromAnotherAddressThanItStarted(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	id, _, _, _ := h.Svc.LoginStart(ctx, "alice", "ip1", []byte("k"))
	if _, err := h.Svc.LoginFinish(ctx, id, "ip2", servertest.CorrectKE3([]byte("k"), alice.Record)); !isErr(err, usecase.ErrUnauthorized) {
		t.Fatal("a login started from one address was finished from another")
	}
}

func failLogin(h *servertest.Harness, user, ip string) error {
	id, _, _, err := h.Svc.LoginStart(ctx, user, ip, []byte("k"))
	if err != nil {
		return err
	}
	_, err = h.Svc.LoginFinish(ctx, id, ip, []byte("wrong"))
	return err
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	h.Register("alice")
	for i := 0; i < 4; i++ {
		_ = failLogin(h, "alice", "a")
		h.Clock.Advance(10 * 60_000)
	}
	_ = failLogin(h, "alice", "a") // fifth failure: backoff 4 s
	h.Clock.Advance(3_999)
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "a", []byte("k")); !isErr(err, usecase.ErrRateLimited) {
		t.Fatalf("backoff after five failures did not reach 4 s: %v", err)
	}
	h.Clock.Advance(1)
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "a", []byte("k")); err != nil {
		t.Fatalf("backoff after five failures exceeded 4 s: %v", err)
	}
	for i := 0; i < 25; i++ {
		_ = failLogin(h, "alice", "b")
		h.Clock.Advance(6 * 60_000)
	}
	_ = failLogin(h, "alice", "b")
	h.Clock.Advance(5*60_000 - 1)
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "b", []byte("k")); !isErr(err, usecase.ErrRateLimited) {
		t.Fatal("backoff fell below its five-minute cap")
	}
}

// Past the hourly budget, attempts against one username are spaced two minutes apart from every
// address, but never refused outright.
func TestPerUserBudgetSlowsWithoutLocking(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	h.Register("alice")
	for i := 0; i < 30; i++ {
		_ = failLogin(h, "alice", "spray-"+strings.Repeat("x", i))
	}
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "fresh", []byte("k")); !isErr(err, usecase.ErrRateLimited) {
		t.Fatalf("31st attempt from a fresh address right after 30 failures: %v, want ErrRateLimited", err)
	}
	h.Clock.Advance(2 * 60_000)
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "fresh", []byte("k")); err != nil {
		t.Fatalf("after two minutes the owner is still refused; the budget must slow, not lock: %v", err)
	}
	h.Clock.Advance(60 * 60_000)
	if _, _, _, err := h.Svc.LoginStart(ctx, "alice", "fresh2", []byte("k")); err != nil {
		t.Fatalf("a new hour did not reset the budget: %v", err)
	}
}
