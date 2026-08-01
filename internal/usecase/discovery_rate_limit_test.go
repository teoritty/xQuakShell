package usecase

import (
	"testing"
	"time"

	"xquakshell/internal/domain/discovery"
)

func TestPublishLimiterDropsTheTwentyFirstPublishInOneSecond(t *testing.T) {
	clock := newFakeClock()
	limiter := NewDiscoveryPublishLimiter(clock.Now)

	for i := range discovery.MaxPublishPerSecond {
		if !limiter.Allow("p1", "c1") {
			t.Fatalf("publish %d must be allowed within the budget", i+1)
		}
	}
	if limiter.Allow("p1", "c1") {
		t.Fatal("publish beyond MaxPublishPerSecond must be refused")
	}
	// The window is a budget, not a strike counter: the next second starts clean.
	clock.advance(time.Second)
	if !limiter.Allow("p1", "c1") {
		t.Fatal("the next second must start with a fresh budget")
	}
}

func TestPublishLimiterBudgetsEachPluginConnectionPairSeparately(t *testing.T) {
	clock := newFakeClock()
	limiter := NewDiscoveryPublishLimiter(clock.Now)

	for range discovery.MaxPublishPerSecond + 1 {
		limiter.Allow("p1", "c1")
	}
	if !limiter.Allow("p1", "c2") {
		t.Fatal("a second connection must have its own budget")
	}
	if !limiter.Allow("p2", "c1") {
		t.Fatal("a second plugin must have its own budget")
	}
}

func TestRateLimitedPublishIsDroppedWithoutFailingThePlugin(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	for range discovery.MaxPublishPerSecond {
		mustPublish(t, h, "", instanceNode("a", ""))
	}
	// The 21st snapshot in the same second: dropped, not an error, and the plugin keeps running.
	if err := h.publish(t, "p1", "s1", "", instanceNode("b", "")); err != nil {
		t.Fatalf("rate limiting must not surface as an error to the plugin: %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "b") {
		t.Fatalf("the dropped snapshot must not reach the tree, got %v", ids)
	}
	// The plugin is still addressable: nothing tore it down.
	h.notifier.reset()
	h.service.SetObserved("c1", []string{""})
	if len(h.notifier.toPlugin("p1")) != 1 {
		t.Fatal("the plugin must still be addressed after a rate-limited publish")
	}
}

func TestEmitCoalescerDoesNotEmitTwiceWithinOneInterval(t *testing.T) {
	clock := newFakeClock()
	var emits []emittedChange
	var timers []func()
	coalescer := NewDiscoveryEmitCoalescer(
		func(connectionID, nodeID string) {
			emits = append(emits, emittedChange{connectionID: connectionID, nodeID: nodeID})
		},
		clock.Now,
		func(_ time.Duration, fn func()) { timers = append(timers, fn) },
	)

	coalescer.Submit("c1", "a")
	if len(emits) != 1 {
		t.Fatalf("the first update must emit immediately, got %d", len(emits))
	}
	if emits[0].nodeID != "a" || emits[0].connectionID != "c1" {
		t.Fatalf("the emit must name the node that changed, got %+v", emits[0])
	}
	clock.advance(discovery.CoalesceInterval / 4)
	coalescer.Submit("c1", "a")
	coalescer.Submit("c1", "a")
	if len(emits) != 1 {
		t.Fatalf("updates inside the window must not emit, got %d", len(emits))
	}
	if len(timers) != 1 {
		t.Fatalf("the window must book exactly one trailing emit, got %d", len(timers))
	}

	// The window closes: the deferred update lands, so the last state is never simply lost.
	clock.advance(discovery.CoalesceInterval)
	timers[0]()
	if len(emits) != 2 {
		t.Fatalf("the trailing emit must fire once, got %d", len(emits))
	}
	if emits[1].nodeID != "a" {
		t.Fatalf("the trailing emit must still name its node, got %+v", emits[1])
	}
}

func TestEmitCoalescerWindowsAreIndependentPerNode(t *testing.T) {
	clock := newFakeClock()
	var emitted []string
	coalescer := NewDiscoveryEmitCoalescer(
		func(_, nodeID string) { emitted = append(emitted, nodeID) },
		clock.Now,
		func(time.Duration, func()) {},
	)

	coalescer.Submit("c1", "a")
	coalescer.Submit("c1", "b")
	if len(emitted) != 2 {
		t.Fatalf("a busy node must not silence a quiet one, got %d emits", len(emitted))
	}
	if emitted[0] != "a" || emitted[1] != "b" {
		t.Fatalf("each emit must name its own node, got %v", emitted)
	}
}

func TestForgettingAConnectionDropsItsPaceState(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	// Burn the connection's whole publish budget and open a coalescing window on the root.
	for range discovery.MaxPublishPerSecond {
		mustPublish(t, h, "", instanceNode("a", ""))
	}
	if h.publish(t, "p1", "s1", "", instanceNode("a", "")) == nil && h.pace.AllowPublish("p1", "c1") {
		t.Fatal("precondition: the budget should be exhausted")
	}

	h.service.ClearConnection("c1")

	// A connection that is gone must leave nothing behind that a reconnect would inherit.
	if !h.pace.AllowPublish("p1", "c1") {
		t.Fatal("clearing a connection must drop its publish window, not carry the old budget over")
	}
}

func TestPublishEmitsThroughTheCoalescer(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	mustPublish(t, h, "", instanceNode("a", ""))
	first := h.emitCount()
	// The emit must name the branch that changed, not just the connection: the frontend contract is
	// DiscoveryTreeChanged{connectionId, nodeId}, and a reader given only the connection could do
	// nothing but redraw the whole tree.
	if change := h.lastEmit(t); change.connectionID != "c1" || change.nodeID != "" {
		t.Fatalf("expected a change on the connection root, got %+v", change)
	}

	mustPublish(t, h, "", instanceNode("a", ""), instanceNode("b", ""))
	if h.emitCount() != first {
		t.Fatal("a second publish inside the interval must not emit again")
	}
	h.clock.advance(discovery.CoalesceInterval)
	h.fireTimers()
	if h.emitCount() != first+1 {
		t.Fatalf("the deferred update must reach the frontend, got %d emits", h.emitCount())
	}
}

func TestPublishOfADeeperBranchEmitsThatBranchesNode(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "group"})
	mustPublish(t, h, "", groupNode("group", ""))

	mustPublish(t, h, "group", instanceNode("leaf", "group"))

	if change := h.lastEmit(t); change.nodeID != "group" {
		t.Fatalf("the emit must name the branch that was filled, got %+v", change)
	}
}
