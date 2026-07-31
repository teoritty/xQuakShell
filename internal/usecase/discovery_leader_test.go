package usecase

import (
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestLeaderIsTheEarliestReadySession(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c1")

	sessionID, protocol, ok := h.leader.Leading("c1")
	if !ok || sessionID != "s1" {
		t.Fatalf("earliest ready session must lead, got %q (ok=%v)", sessionID, ok)
	}
	if protocol != "ssh" {
		t.Fatalf("connection protocol must be resolved for addressing, got %q", protocol)
	}
}

func TestLeaderHandoverKeepsTheTreeMarksItStaleAndReObservesTheNewLeader(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c1")
	h.service.SetObserved("c1", []string{"", "a"})
	mustPublish(t, h, "", groupNode("a", ""))
	h.notifier.reset()

	h.leader.SessionClosed("s1", "c1")
	// The re-observe rides at the end of the handover's binding reconciliation, so that the new
	// leader's plugins are authorized for the session named in it before they are told to watch
	// anything through it.
	h.leader.awaitReconcile()

	if ids := nodeIDsOf(h.service.Snapshot("c1")); !containsID(ids, "a") {
		t.Fatalf("a handover must not delete the tree, got %v", ids)
	}
	if state := branchOf(t, h.service.Snapshot("c1"), "p1", "").State; state != discovery.BranchStale {
		t.Fatalf("branches must go stale for the handover, got %q", state)
	}
	sent := h.notifier.toPlugin("p1")
	if len(sent) != 1 {
		t.Fatalf("the new leader's plugins must be re-observed, got %d observes", len(sent))
	}
	if sent[0].sessionID != "s2" {
		t.Fatalf("observe must now address the new leader, got %q", sent[0].sessionID)
	}
	if len(sent[0].nodeIDs) != 2 {
		t.Fatalf("the resend must be the full set, got %v", sent[0].nodeIDs)
	}
}

func TestNonLeadingSessionClosingChangesNothing(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c1")
	h.service.SetObserved("c1", []string{""})
	mustPublish(t, h, "", groupNode("a", ""))
	h.notifier.reset()

	h.leader.SessionClosed("s2", "c1")

	if state := branchOf(t, h.service.Snapshot("c1"), "p1", "").State; state == discovery.BranchStale {
		t.Fatal("closing a follower must not stale the tree")
	}
	if got := len(h.notifier.all()); got != 0 {
		t.Fatalf("closing a follower must not re-observe anyone, got %d notifications", got)
	}
}

func TestLastReadySessionLeavingDeletesTheTree(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a"})
	mustPublish(t, h, "", groupNode("a", ""))

	h.leader.SessionClosed("s1", "c1")

	if snapshot := h.service.Snapshot("c1"); len(snapshot.Plugins) != 0 {
		t.Fatalf("nothing is cached once no ready session remains, got %d plugin trees", len(snapshot.Plugins))
	}
	if h.observer.IsObserved("c1", "a") {
		t.Fatal("the observed set must go with the tree")
	}
	if _, _, ok := h.leader.Leading("c1"); ok {
		t.Fatal("no session may lead a connection with none ready")
	}
	if h.emitCount() == 0 {
		t.Fatal("the frontend must be told the tree is gone")
	}
}

func TestPublishFromASessionThatLostTheLeadIsIgnored(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c1")
	h.service.SetObserved("c1", []string{""})

	h.leader.SessionClosed("s1", "c1")

	// s1 was mid-enumeration when the handover happened; its snapshot is not an error.
	if err := h.publish(t, "p1", "s1", "", instanceNode("ghost", "")); err != nil {
		t.Fatalf("a publish racing a handover must not be an error: %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "ghost") {
		t.Fatalf("a former leader must not keep writing to the tree, got %v", ids)
	}
}

func TestPublishForAnUnknownSessionIsIgnoredLikeAnyOtherNonLeader(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	// A session the host never heard of is treated exactly like a former leader: the host cannot
	// tell a handover race from garbage, and refusing one but not the other would be a distinction
	// it has no evidence for. Rejecting the caller's session is IDOR defence and lives in the
	// capability layer, decided from the plugin's binding rather than guessed at here.
	if err := h.publish(t, "p1", "nope", "", instanceNode("a", "")); err != nil {
		t.Fatalf("a publish from a session the host is not addressing must be ignored, not refused: %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "a") {
		t.Fatalf("an unaddressed session must not write to the tree, got %v", ids)
	}
}
