package usecase

import (
	"strings"
	"testing"
)

// The window this file exists for: a plugin was told what to observe before it was allowed to
// answer.
//
// Two paths produced it. In the reconciliation, EnsureRunning came first, and starting a plugin
// fires the host's process-started hook synchronously — which replays the observed set to it — so
// the observe went out while BindSession was still one line away. And SessionReady sent the observe
// itself, alongside a reconciliation that had only just been handed to a goroutine.
//
// The consequence is not a retry. The publish that observe triggers is refused with -32001 and
// audit-logged as a denial, and nothing sends the observe again: broadcast has already marked that
// version delivered, and the process-started replay does not bump it. The branch stays empty until
// the user collapses and re-expands it by hand — precisely the unrecoverable emptiness the
// level-triggered design exists to rule out.

// TestAPluginIsAuthorizedBeforeItIsToldWhatToObserve drives the real ordering: the fake runtime
// fires the observed-set replay from inside EnsureRunning, exactly as PluginManager does.
func TestAPluginIsAuthorizedBeforeItIsToldWhatToObserve(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.runtime.onStart = func(pluginID string) { h.observer.PluginStarted(pluginID) }
	// Something to observe, so the replay has a non-empty set to carry and the test is not passing
	// on an observe nobody would have sent anyway.
	h.service.SetObserved("c1", []string{""})
	h.protocols.bySession["s1"] = "ssh"

	h.leader.SessionReady("s1", "c1")
	h.leader.awaitReconcile()

	events := h.callLog.all()
	bindAt := h.callLog.firstIndexOf("bind:p1")
	observeAt := h.callLog.firstIndexOf("observe:p1")
	if bindAt < 0 {
		t.Fatalf("the plugin was never authorized at all: %v", events)
	}
	if observeAt < 0 {
		t.Fatalf("the plugin was never told what to observe: %v", events)
	}
	if bindAt > observeAt {
		t.Fatalf("observe reached the plugin before its binding existed; the publish it triggers "+
			"would be refused with -32001 and never replayed. Order was: %v", events)
	}
}

// TestTheStartedPluginsReplayIsFollowedByAFreshObserve is the second half of the same rule. The
// replay fired from inside EnsureRunning is not enough on its own: it goes out mid-reconciliation,
// and a plugin that fails to start, or starts after its neighbours, must still end up holding the
// current set. The reconciliation therefore ends with an observe of its own.
func TestTheStartedPluginsReplayIsFollowedByAFreshObserve(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.service.SetObserved("c1", []string{""})
	h.protocols.bySession["s1"] = "ssh"

	h.leader.SessionReady("s1", "c1")
	h.leader.awaitReconcile()

	sent := h.notifier.toPlugin("p1")
	if len(sent) == 0 {
		t.Fatal("the leading session was never announced to the plugin")
	}
	last := sent[len(sent)-1]
	if last.sessionID != "s1" {
		t.Fatalf("the final observe must name the leading session, got %q", last.sessionID)
	}
	if len(last.nodeIDs) != 1 || last.nodeIDs[0] != "" {
		t.Fatalf("the final observe must carry the current set, got %v", last.nodeIDs)
	}
}

// TestTeardownSendsNoObserve: the abandoned path releases authorizations and must not follow them
// with a notification. There is no transport left to carry it, and asking the observer to broadcast
// would re-create the per-connection state that ClearConnection is about to delete.
func TestTeardownSendsNoObserve(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})
	h.notifier.reset()

	h.leader.SessionClosed("s1", "c1")
	h.leader.awaitReconcile()

	for _, sent := range h.notifier.all() {
		t.Fatalf("teardown must send nothing, got an observe to %q for session %q", sent.pluginID, sent.sessionID)
	}
	if !h.runtime.wasUnbound(discoveryBinding{pluginID: "p1", sessionID: "s1"}) {
		t.Fatal("teardown must still release the authorization")
	}
	if strings.Join(h.callLog.all(), " ") == "" {
		t.Fatal("nothing was recorded at all; the harness is not wired to the call log")
	}
}
