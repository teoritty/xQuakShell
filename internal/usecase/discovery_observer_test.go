package usecase

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestObserveGoesOnlyToPluginsDeclaringTheConnectionProtocol(t *testing.T) {
	h := newDiscoveryHarness(t,
		DiscoveryPluginTarget{PluginID: "ssh-plugin", ParentProtocols: []string{"ssh"}},
		DiscoveryPluginTarget{PluginID: "other-plugin", ParentProtocols: []string{"telnet"}},
	)
	h.sessionReady("s1", "c1")

	h.service.SetObserved("c1", []string{"", "a"})

	if got := len(h.notifier.toPlugin("ssh-plugin")); got != 1 {
		t.Fatalf("plugin declaring ssh must be told, got %d notifications", got)
	}
	if got := len(h.notifier.toPlugin("other-plugin")); got != 0 {
		t.Fatalf("parentProtocols is an addressing filter, not decoration: got %d notifications", got)
	}
}

func TestObserveCarriesTheLeadingSessionAndTheFullSet(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")

	h.service.SetObserved("c1", []string{"", "a", "b"})

	sent := h.notifier.toPlugin("p1")
	if len(sent) != 1 {
		t.Fatalf("expected one observe, got %d", len(sent))
	}
	if sent[0].method != discoveryObserveMethod {
		t.Fatalf("wrong method %q", sent[0].method)
	}
	if sent[0].sessionID != "s1" {
		t.Fatalf("observe must address the leading session, got %q", sent[0].sessionID)
	}
	if len(sent[0].nodeIDs) != 3 {
		t.Fatalf("observe must carry the full set, got %v", sent[0].nodeIDs)
	}
}

func TestCascadeDeleteDropsVanishedNodesFromObservedAndResendsObserve(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a", "a1"})

	mustPublish(t, h, "", groupNode("a", ""))
	mustPublish(t, h, "a", groupNode("a1", "a"))
	mustPublish(t, h, "a1", instanceNode("a1x", "a1"))
	h.notifier.reset()

	// "a" disappears, taking a1 and a1x with it.
	mustPublish(t, h, "")

	if h.observer.IsObserved("c1", "a") || h.observer.IsObserved("c1", "a1") {
		t.Fatal("vanished nodes must fall out of the observed set")
	}
	if !h.observer.IsObserved("c1", "") {
		t.Fatal("the connection root must stay observed")
	}
	sent := h.notifier.toPlugin("p1")
	if len(sent) != 1 {
		t.Fatalf("recomputed set must be resent exactly once, got %d observes", len(sent))
	}
	if len(sent[0].nodeIDs) != 1 || sent[0].nodeIDs[0] != "" {
		t.Fatalf("resent set must contain only the root, got %v", sent[0].nodeIDs)
	}
}

func TestPluginRestartResendsTheFullObservedSet(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a"})
	h.notifier.reset()

	// The plugin process died and came back; nobody re-expanded anything.
	h.observer.PluginStarted("p1")

	sent := h.notifier.toPlugin("p1")
	if len(sent) != 1 {
		t.Fatalf("a restarted plugin must be re-told what to watch, got %d observes", len(sent))
	}
	if len(sent[0].nodeIDs) != 2 {
		t.Fatalf("the resend must be the full set, got %v", sent[0].nodeIDs)
	}
}

func TestPluginRestartTellsNothingWhenNoSessionIsReady(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.service.SetObserved("c1", []string{"", "a"})
	h.notifier.reset()

	h.observer.PluginStarted("p1")

	if got := len(h.notifier.all()); got != 0 {
		t.Fatalf("with no ready session there is no transport to address, got %d notifications", got)
	}
}

func TestObservedSetSurvivesUntilTheConnectionIsCleared(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a"})
	mustPublish(t, h, "", groupNode("a", ""))

	h.service.ClearConnection("c1")

	if h.observer.IsObserved("c1", "a") {
		t.Fatal("clearing a connection must drop its observed set")
	}
	if snapshot := h.service.Snapshot("c1"); len(snapshot.Plugins) != 0 {
		t.Fatalf("clearing a connection must drop its tree, got %d plugin trees", len(snapshot.Plugins))
	}
	if _, ok := branchesOfPlugin(h.service.Snapshot("c1"), "p1"); ok {
		t.Fatal("no branches may survive a cleared connection")
	}
}

// TestConcurrentObservedSetChangesLeavePluginsWithTheCurrentSet pins the ordering guarantee. The
// set is read inside the per-connection send gate rather than captured before it, so whichever
// caller sends last necessarily read the newest state. Without that, two callers can each read
// their own version and deliver them in the opposite order, leaving the plugin watching a set the
// host has abandoned — and the branch the user just expanded never fills.
func TestConcurrentObservedSetChangesLeavePluginsWithTheCurrentSet(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")

	const writers = 24
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Every writer publishes a different set, so a stale delivery is visible as a mismatch
			// rather than hidden behind identical payloads.
			h.service.SetObserved("c1", []string{"", fmt.Sprintf("n%d", i)})
		}()
	}
	wg.Wait()

	sent := h.notifier.toPlugin("p1")
	if len(sent) == 0 {
		t.Fatal("expected at least one observe")
	}
	delivered := sent[len(sent)-1].nodeIDs
	current := h.observer.nodeIDs("c1")
	if !slices.Equal(delivered, current) {
		t.Fatalf("the plugin was left with %v while the host holds %v", delivered, current)
	}
}

func TestRepeatedBroadcastWithoutAChangeSaysNothingTwice(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a"})
	h.notifier.reset()

	// Nothing changed: an already-delivered version has nothing to say.
	h.observer.ConnectionChanged("c1")
	first := len(h.notifier.toPlugin("p1"))
	h.notifier.reset()
	h.service.SetObserved("c1", []string{"", "a"})

	if first != 1 {
		t.Fatalf("a handover must resend even an unchanged set, got %d", first)
	}
	if got := len(h.notifier.toPlugin("p1")); got != 1 {
		t.Fatalf("an explicit SetObserved must always be delivered, got %d", got)
	}
}

func branchesOfPlugin(snapshot DiscoverySnapshot, pluginID string) (map[string]discovery.Branch, bool) {
	for _, tree := range snapshot.Plugins {
		if tree.PluginID == pluginID {
			return tree.Branches, true
		}
	}
	return nil, false
}
