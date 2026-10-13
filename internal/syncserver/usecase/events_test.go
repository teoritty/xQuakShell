package usecase_test

import (
	"testing"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/usecase"
)

func TestCommitNotifiesMembersOnly(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, dave := h.Register("alice"), h.Register("bob"), h.Register("dave")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	bobEvents, cancelBob, err := h.Svc.Subscribe(bob.Account)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelBob()
	daveEvents, cancelDave, _ := h.Svc.Subscribe(dave.Account)
	defer cancelDave()
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	select {
	case ev := <-bobEvents:
		if ev.Vault != cv.ID || ev.Seq != 1 {
			t.Fatalf("event = %+v, want vault %s seq 1", ev, cv.ID)
		}
	default:
		t.Fatal("a member was not notified of a commit")
	}
	select {
	case ev := <-daveEvents:
		t.Fatalf("an outsider received %+v; events must not reveal vault activity to non-members", ev)
	default:
	}
}

func TestMembershipChangeNotifiesMembers(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice, bob, carol := h.Register("alice"), h.Register("bob"), h.Register("carol")
	cv := h.NewTeamVault(alice, servertest.Member{User: bob, Role: vaultsync.RoleEditor})
	events, cancel, _ := h.Svc.Subscribe(bob.Account)
	defer cancel()
	cv.Users[carol.Account] = carol
	if err := cv.AppendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: carol.Account, Role: vaultsync.RoleViewer, Fingerprint: servertest.Fingerprint(carol),
		Verified: true, Wraps: servertest.WrapFor(carol.Account)}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		if ev.Vault != cv.ID || ev.MembershipSeq != cv.M.Seq() {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("a member was not told the membership changed")
	}
}

func TestSubscriptionCap(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	var cancels []func()
	for i := 0; i < 16; i++ {
		_, cancel, err := h.Svc.Subscribe(alice.Account)
		if err != nil {
			t.Fatalf("subscription %d refused: %v", i+1, err)
		}
		cancels = append(cancels, cancel)
	}
	if _, _, err := h.Svc.Subscribe(alice.Account); !isErr(err, usecase.ErrRateLimited) {
		t.Fatalf("17th subscription: %v, want ErrRateLimited", err)
	}
	cancels[0]()
	if _, cancel, err := h.Svc.Subscribe(alice.Account); err != nil {
		t.Fatalf("a freed slot was not reusable: %v", err)
	} else {
		cancel()
	}
	for _, c := range cancels[1:] {
		c()
	}
}

// A subscriber that never reads must not block commits: once its buffer is full it is dropped and
// its channel closed, telling the transport to hang up so the client reconnects and pulls.
func TestSlowSubscriberIsDroppedNotWaitedFor(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	cv := h.PersonalVault(alice)
	events, cancel, _ := h.Svc.Subscribe(alice.Account)
	defer cancel()
	for i := 0; i < 20; i++ {
		cv.Accept(alice, cv.Commit(alice, cv.Record(alice, byte(i+1))))
	}
	n := 0
	for range events {
		n++
	}
	if n == 0 || n > 16 {
		t.Fatalf("a dropped subscriber delivered %d events before closing, want 1..16", n)
	}
}

func TestDeviceLogAppendNotifiesTheAccount(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	alice := h.Register("alice")
	events, cancel, _ := h.Svc.Subscribe(alice.Account)
	defer cancel()
	second := servertest.RandomID(t)
	_, dsk, _ := ed25519Key()
	if err := h.Svc.AppendDeviceLog(ctx, h.Principal(alice.Access), []vaultsync.DeviceEntry{alice.AddDeviceEntry(t, second, dsk)}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		if ev.Account != alice.Account || ev.DeviceLogSeq != 2 {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("other devices of the account were not told a device was added; that alert is the defence against a stolen password")
	}
}
