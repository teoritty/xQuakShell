package usecase

import (
	"context"
	"slices"
	"testing"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
)

// TestPluginWithoutDiscoveryCapabilityIsNeverAddressed covers the half of the security model the
// gate cannot: observe and invokeAction travel host->plugin, so nothing denies them at the plugin's
// door. The host simply never sends them, and "never" is expressed as absence from
// DiscoveryPlugins() — a plugin without capabilities.discovery is not in that list at all.
//
// Both verbs are checked in one test on purpose: they are two consequences of one decision, and a
// change that starts addressing a capability-less plugin would almost certainly affect both.
func TestPluginWithoutDiscoveryCapabilityIsNeverAddressed(t *testing.T) {
	// Only "declared" appears in the lookup; "undeclared" is what an installed plugin without the
	// capability looks like from here.
	h := newDiscoveryHarness(t, DiscoveryPluginTarget{PluginID: "declared", ParentProtocols: []string{"ssh"}})
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "group"})

	if got := len(h.notifier.toPlugin("undeclared")); got != 0 {
		t.Fatalf("a plugin without capabilities.discovery must receive no observe, got %d", got)
	}
	if got := len(h.notifier.toPlugin("declared")); got == 0 {
		t.Fatal("the declaring plugin must still be observed, otherwise this test proves nothing")
	}

	// The declaring plugin publishes a node with an action. The undeclared plugin owns no tree, so
	// an action aimed at it cannot resolve and never leaves the host.
	restart := discovery.Action{ID: "restart", Label: "Restart"}
	h.publishAs(t, "declared", "s1", "", instanceNode("one", "", restart))

	if err := h.service.InvokeAction(context.Background(), "c1", "undeclared", []string{"one"}, "restart"); err == nil {
		t.Fatal("an action addressed to a plugin without a discovery subtree must be refused")
	}
	if h.caller.count() != 0 {
		t.Fatalf("no invokeAction may reach a plugin without the capability, got %d calls", h.caller.count())
	}
}

// TestUndeclaredIconIsDroppedAndThePublishSurvives pins ADR-014's explicit choice: an iconId the
// manifest never registered costs the node its icon, not the branch its contents.
func TestUndeclaredIconIsDroppedAndThePublishSurvives(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.icons.declare("p1", "known")
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	good := discovery.Node{ID: "good", Kind: discovery.KindInstance, Label: "good", IconID: "known"}
	bad := discovery.Node{ID: "bad", Kind: discovery.KindInstance, Label: "bad", IconID: "never-declared"}
	if err := h.publish(t, "p1", "s1", "", good, bad); err != nil {
		t.Fatalf("an undeclared iconId must not fail the publish: %v", err)
	}

	ids := nodeIDsOf(h.service.Snapshot("c1"))
	if !containsID(ids, "bad") {
		t.Fatalf("the node itself must still be published, got %v", ids)
	}
	if got := iconOf(t, h, "p1", "bad"); got != "" {
		t.Fatalf("an undeclared iconId must be dropped, got %q", got)
	}
	if got := iconOf(t, h, "p1", "good"); got != "known" {
		t.Fatalf("a declared iconId must survive untouched, got %q", got)
	}
}

// TestUndeclaredActionIconIsDroppedWithoutLosingTheAction checks the field the node-level check is
// easy to stop at. An action's icon is drawn from the same registry and is just as unvalidatable
// later, but dropping the whole action would remove a capability the plugin does offer.
func TestUndeclaredActionIconIsDroppedWithoutLosingTheAction(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.icons.declare("p1", "known")
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	node := discovery.Node{
		ID: "one", Kind: discovery.KindInstance, Label: "one", IconID: "known",
		Actions: []discovery.Action{
			{ID: "restart", Label: "Restart", IconID: "bogus"},
			{ID: "logs", Label: "Logs", IconID: "known"},
		},
	}
	if err := h.publish(t, "p1", "s1", "", node); err != nil {
		t.Fatalf("publish: %v", err)
	}

	actions := actionsOf(t, h, "p1", "one")
	if len(actions) != 2 {
		t.Fatalf("both actions must survive, got %d", len(actions))
	}
	if actions[0].IconID != "" {
		t.Fatalf("undeclared action icon must be dropped, got %q", actions[0].IconID)
	}
	if actions[1].IconID != "known" {
		t.Fatalf("declared action icon must survive, got %q", actions[1].IconID)
	}
	// The plugin's own node payload must not be corrupted for the caller either: only the icon is
	// touched, and nothing else about the action changes.
	if actions[0].Label != "Restart" || actions[0].ID != "restart" {
		t.Fatalf("only the iconId may be rewritten, got %+v", actions[0])
	}
}

// TestInvokeActionAuditCarriesEveryNodeID is the reason DiscoveryAuditEntry holds a slice. A mass
// action's audit line is the only lasting record of what it hit; a count or a first-node sample
// would be exactly as useless as no entry at all when an incident has to be reconstructed.
func TestInvokeActionAuditCarriesEveryNodeID(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	restart := discovery.Action{ID: "restart", Label: "Restart", Multi: true}
	nodes := make([]discovery.Node, 0, discovery.MaxNodesPerInvoke)
	want := make([]string, 0, discovery.MaxNodesPerInvoke)
	for i := range discovery.MaxNodesPerInvoke {
		id := "n" + itoa(i)
		nodes = append(nodes, instanceNode(id, "", restart))
		want = append(want, id)
	}
	mustPublish(t, h, "", nodes...)

	if err := h.service.InvokeAction(context.Background(), "c1", "p1", want, "restart"); err != nil {
		t.Fatalf("mass action must be relayed: %v", err)
	}

	entries := h.audit.all()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one audit entry for one dispatched action, got %d", len(entries))
	}
	entry := entries[0]
	if !slices.Equal(entry.NodeIDs, want) {
		t.Fatalf("audit must carry all %d node ids in order, got %d: %v", len(want), len(entry.NodeIDs), entry.NodeIDs)
	}
	if entry.PluginID != "p1" || entry.SessionID != "s1" || entry.ActionID != "restart" || !entry.Success {
		t.Fatalf("audit entry missing required context: %+v", entry)
	}
	if entry.Action != domainplugin.DiscoveryAuditDispatch {
		t.Fatalf("a relayed action must be recorded as a dispatch, got %q", entry.Action)
	}
}

// TestInvokeActionAuditIsIndependentOfTheCallerSlice guards the copy in record(): the audit entry
// must describe the invocation forever, not alias a slice the caller is free to reuse.
func TestInvokeActionAuditIsIndependentOfTheCallerSlice(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	restart := discovery.Action{ID: "restart", Label: "Restart", Multi: true}
	mustPublish(t, h, "", instanceNode("one", "", restart), instanceNode("two", "", restart))

	selection := []string{"one", "two"}
	if err := h.service.InvokeAction(context.Background(), "c1", "p1", selection, "restart"); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	selection[0] = "tampered"

	entries := h.audit.all()
	if len(entries) == 0 || entries[0].NodeIDs[0] != "one" {
		t.Fatalf("audit must not alias the caller's slice, got %+v", entries)
	}
}

// TestStoppingAPluginClearsOnlyItsOwnSubtree is the deactivation boundary case: a plugin the user
// disabled or uninstalled loses its tree at once, and its neighbours under the same connection lose
// nothing. Sharing a connection is not sharing a fate.
func TestStoppingAPluginClearsOnlyItsOwnSubtree(t *testing.T) {
	h := newDiscoveryHarness(t,
		DiscoveryPluginTarget{PluginID: "pA", ParentProtocols: []string{"ssh"}},
		DiscoveryPluginTarget{PluginID: "pB", ParentProtocols: []string{"ssh"}},
	)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a-group"})

	h.publishAs(t, "pA", "s1", "", groupNode("a-group", ""))
	h.publishAs(t, "pA", "s1", "a-group", instanceNode("a-child", "a-group"))
	h.publishAs(t, "pB", "s1", "", instanceNode("b-node", ""))

	h.service.ClearPlugin("pA")

	ids := nodeIDsOf(h.service.Snapshot("c1"))
	if containsID(ids, "a-group") || containsID(ids, "a-child") {
		t.Fatalf("the stopped plugin's whole subtree must be gone, got %v", ids)
	}
	if !containsID(ids, "b-node") {
		t.Fatalf("a neighbouring plugin's subtree must be untouched, got %v", ids)
	}

	// The observed set must shed the vanished IDs too, or the host keeps telling the remaining
	// plugins to watch a node nobody has.
	h.notifier.reset()
	h.service.SetObserved("c1", []string{"", "a-group"})
	if got := h.notifier.toPlugin("pB"); len(got) == 0 {
		t.Fatal("expected pB to still be observed")
	}
}

// TestStoppingAPluginFreesItsRateLimitWindow covers the tail task 3 left: a stopped plugin's
// half-spent publish budget must not throttle it the moment it is started again.
func TestStoppingAPluginFreesItsRateLimitWindow(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	// Spend the whole second's budget without advancing the injected clock.
	for i := 0; i < discovery.MaxPublishPerSecond; i++ {
		mustPublish(t, h, "", instanceNode("n", ""))
	}
	if h.pace.AllowPublish("p1", "c1") {
		t.Fatal("the budget was expected to be exhausted; the rest of this test proves nothing")
	}

	h.service.ClearPlugin("p1")

	if !h.pace.AllowPublish("p1", "c1") {
		t.Fatal("stopping a plugin must drop its window, so a restart publishes immediately")
	}
}

// TestStoppingAPluginFreesAWindowItLeftWithoutATree is the tail behind the tail. A publish is
// counted against the budget BEFORE it can be refused, so a plugin that only ever published into
// collapsed branches spent budget and created no tree at all. Sweeping the windows by walking the
// trees would forget exactly the plugins that have one and strand the ones that do not.
func TestStoppingAPluginFreesAWindowItLeftWithoutATree(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	// Nothing is observed, so every publish below is dropped and no tree is ever created.
	h.service.SetObserved("c1", nil)

	for i := 0; i < discovery.MaxPublishPerSecond; i++ {
		if err := h.publish(t, "p1", "s1", "", instanceNode("n", "")); err != nil {
			t.Fatalf("a publish for an unobserved node must be dropped, not failed: %v", err)
		}
	}
	if h.pace.AllowPublish("p1", "c1") {
		t.Fatal("dropped publishes were expected to spend budget; the rest of this test proves nothing")
	}
	if ids := h.store.ConnectionsWithPlugin("p1"); len(ids) != 0 {
		t.Fatalf("no tree should exist for this plugin, got %v", ids)
	}

	h.service.ClearPlugin("p1")

	if !h.pace.AllowPublish("p1", "c1") {
		t.Fatal("a window with no tree behind it must still be forgotten when the plugin stops")
	}
}

// TestCrashedPluginBranchesGoStaleAndKeepTheirNodes is the ADR-014 crash case, and the same
// treatment an idle suspension gets: the nodes stay, the branch stops claiming to be current, and
// the subtree is not deleted — the process comes back (supervisor restart, or the next activation)
// and the replayed observed set refills it, so a teardown here would make a recoverable absence
// look like a disappearance.
func TestCrashedPluginBranchesGoStaleAndKeepTheirNodes(t *testing.T) {
	h := newDiscoveryHarness(t,
		DiscoveryPluginTarget{PluginID: "pA", ParentProtocols: []string{"ssh"}},
		DiscoveryPluginTarget{PluginID: "pB", ParentProtocols: []string{"ssh"}},
	)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a-group"})

	h.publishAs(t, "pA", "s1", "", groupNode("a-group", ""))
	h.publishAs(t, "pA", "s1", "a-group", instanceNode("a-child", "a-group"))
	h.publishAs(t, "pB", "s1", "", instanceNode("b-node", ""))

	h.service.MarkPluginStale("pA")

	ids := nodeIDsOf(h.service.Snapshot("c1"))
	if !containsID(ids, "a-group") || !containsID(ids, "a-child") {
		t.Fatalf("a crash must not delete the crashed plugin's nodes, got %v", ids)
	}
	if got := branchOf(t, h.service.Snapshot("c1"), "pA", "").State; got != discovery.BranchStale {
		t.Fatalf("the crashed plugin's root branch must go stale, got %q", got)
	}
	if got := branchOf(t, h.service.Snapshot("c1"), "pA", "a-group").State; got != discovery.BranchStale {
		t.Fatalf("every branch of the crashed plugin must go stale, got %q", got)
	}
	// A neighbour under the same connection is still running and still authoritative.
	if got := branchOf(t, h.service.Snapshot("c1"), "pB", "").State; got != discovery.BranchReady {
		t.Fatalf("another plugin's branch must be untouched by the crash, got %q", got)
	}
}

// TestActionsInsideAStaleBranchAreRefusedRatherThanTimingOut is why stale is more than a label. It
// is the same marking for a crash and for an idle suspension: in both the process is gone, so the
// nodes on screen name resources nothing has re-confirmed. Without the mark the action is
// dispatched into a dead process and fails on the 5 s ack timeout, which tells the user nothing
// about why — the refusal is both faster and honest.
func TestActionsInsideAStaleBranchAreRefusedRatherThanTimingOut(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	restart := discovery.Action{ID: "restart", Label: "Restart"}
	mustPublish(t, h, "", instanceNode("one", "", restart))
	if err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"one"}, "restart"); err != nil {
		t.Fatalf("the action must work before the crash, or this test proves nothing: %v", err)
	}
	before := h.caller.count()

	h.service.MarkPluginStale("p1")

	if err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"one"}, "restart"); err == nil {
		t.Fatal("an action inside a stale branch must be refused")
	}
	if h.caller.count() != before {
		t.Fatal("a refused action must never reach the plugin")
	}
}

// TestCrashOfAPluginWithNoSubtreeIsHarmless: every crash and every idle suspension reaches
// MarkPluginStale, and most plugins have drawn nothing.
func TestCrashOfAPluginWithNoSubtreeIsHarmless(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})
	mustPublish(t, h, "", instanceNode("one", ""))
	before := h.emitCount()

	h.service.MarkPluginStale("never-published")

	if ids := nodeIDsOf(h.service.Snapshot("c1")); !containsID(ids, "one") {
		t.Fatalf("an unrelated plugin's crash must change nothing, got %v", ids)
	}
	if h.emitCount() != before {
		t.Fatal("a crash that changed no branch must not announce a redraw")
	}
}

// TestClearingAPluginWithNoSubtreeIsHarmless: every plugin stop reaches ClearPlugin, and the vast
// majority of plugins never drew anything.
func TestClearingAPluginWithNoSubtreeIsHarmless(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})
	mustPublish(t, h, "", instanceNode("one", ""))

	h.service.ClearPlugin("never-published")

	if ids := nodeIDsOf(h.service.Snapshot("c1")); !containsID(ids, "one") {
		t.Fatalf("clearing an unrelated plugin must change nothing, got %v", ids)
	}
}

func iconOf(t *testing.T, h *discoveryHarness, pluginID, nodeID string) string {
	t.Helper()
	return findNode(t, h, pluginID, nodeID).IconID
}

func actionsOf(t *testing.T, h *discoveryHarness, pluginID, nodeID string) []discovery.Action {
	t.Helper()
	return findNode(t, h, pluginID, nodeID).Actions
}

func findNode(t *testing.T, h *discoveryHarness, pluginID, nodeID string) discovery.Node {
	t.Helper()
	for _, tree := range h.service.Snapshot("c1").Plugins {
		if tree.PluginID != pluginID {
			continue
		}
		for _, view := range tree.Nodes {
			if view.Node.ID == nodeID {
				return view.Node
			}
		}
	}
	t.Fatalf("no node %q for plugin %q", nodeID, pluginID)
	return discovery.Node{}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
