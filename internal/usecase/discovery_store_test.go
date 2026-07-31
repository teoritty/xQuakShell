package usecase

import (
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestSnapshotReplacesBranchInsteadOfMergingIntoIt(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	if err := h.publish(t, "p1", "s1", "", instanceNode("a", ""), instanceNode("b", "")); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if err := h.publish(t, "p1", "s1", "", instanceNode("c", "")); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	ids := nodeIDsOf(h.service.Snapshot("c1"))
	if len(ids) != 1 || ids[0] != "c" {
		t.Fatalf("second snapshot must replace the branch, got %v", ids)
	}
}

func TestSnapshotKeepsGrandchildrenOfNodesThatSurvive(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a"})

	mustPublish(t, h, "", groupNode("a", ""), groupNode("b", ""))
	mustPublish(t, h, "a", instanceNode("a1", "a"))
	// "a" is republished, "b" is not: only b's subtree may disappear.
	mustPublish(t, h, "", groupNode("a", ""))

	ids := nodeIDsOf(h.service.Snapshot("c1"))
	if !containsID(ids, "a") || !containsID(ids, "a1") || containsID(ids, "b") {
		t.Fatalf("expected a and a1 to survive and b to go, got %v", ids)
	}
}

func TestPublishForUnobservedNodeIsDroppedSilently(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	mustPublish(t, h, "", groupNode("a", ""))
	// Nobody is watching "a": the user never expanded it.
	if err := h.publish(t, "p1", "s1", "a", instanceNode("a1", "a")); err != nil {
		t.Fatalf("unobserved publish must not be an error: %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "a1") {
		t.Fatalf("unobserved publish must not reach the tree, got %v", ids)
	}
}

// TestPublishUnderAnInstanceIsRefused pins the one thing "instance" means: a leaf. Without the
// check a plugin could hang children off a node it declared childless, and the tree widget would
// draw a row that is a leaf and a branch at the same time.
//
// It is an error rather than one of the silent drops beside it because the plugin needs to see it:
// there is a correct way to express "expandable but currently empty" — kind=group with no children
// — so nothing legitimate is being refused here.
func TestPublishUnderAnInstanceIsRefused(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "leaf"})

	mustPublish(t, h, "", instanceNode("leaf", ""))

	err := h.publish(t, "p1", "s1", "leaf", instanceNode("under-leaf", "leaf"))
	if !errors.Is(err, ErrDiscoveryLeafParent) {
		t.Fatalf("publishing under an instance must be refused, got %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "under-leaf") {
		t.Fatalf("a refused publish must leave the tree untouched, got %v", ids)
	}
}

// TestPublishUnderAGroupWithNoChildrenStaysAllowed is the other half: the empty branch a plugin is
// supposed to use instead. Without it the test above would still pass if publishing under any
// childless node were refused.
func TestPublishUnderAGroupWithNoChildrenStaysAllowed(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "empty"})

	mustPublish(t, h, "", groupNode("empty", ""))
	if err := h.publish(t, "p1", "s1", "empty"); err != nil {
		t.Fatalf("an empty group is how a plugin says \"nothing here\": %v", err)
	}
	if got := branchOf(t, h.service.Snapshot("c1"), "p1", "empty").State; got != discovery.BranchReady {
		t.Fatalf("the empty branch must be ready, got %q", got)
	}
}

func TestDuplicateNodeUnderDifferentParentRejectsWholeSnapshot(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a", "b"})

	mustPublish(t, h, "", groupNode("a", ""), groupNode("b", ""))
	mustPublish(t, h, "a", instanceNode("shared", "a"))

	before := nodeIDsOf(h.service.Snapshot("c1"))
	err := h.publish(t, "p1", "s1", "b", instanceNode("fresh", "b"), instanceNode("shared", "b"))
	if !errors.Is(err, ErrDiscoveryDuplicateNode) {
		t.Fatalf("expected ErrDiscoveryDuplicateNode, got %v", err)
	}
	after := nodeIDsOf(h.service.Snapshot("c1"))
	if len(before) != len(after) {
		t.Fatalf("refused snapshot must leave the tree untouched: before %v, after %v", before, after)
	}
	if containsID(after, "fresh") {
		t.Fatalf("the valid half of a refused snapshot must not be applied, got %v", after)
	}
}

func TestChildrenBeyondMaxDepthAreTruncatedNotRefused(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")

	// Build a chain to exactly MaxDepth, observing each level as we go.
	parent := ""
	observed := []string{""}
	for depth := 1; depth <= discovery.MaxDepth; depth++ {
		h.service.SetObserved("c1", observed)
		id := fmt.Sprintf("n%d", depth)
		mustPublish(t, h, parent, groupNode(id, parent))
		parent = id
		observed = append(observed, id)
	}
	h.service.SetObserved("c1", observed)

	// One level deeper than the tree may go.
	if err := h.publish(t, "p1", "s1", parent, instanceNode("toodeep", parent)); err != nil {
		t.Fatalf("over-depth publish must be truncation, not refusal: %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "toodeep") {
		t.Fatalf("node beyond MaxDepth must be dropped, got %v", ids)
	}
	branch := branchOf(t, h.service.Snapshot("c1"), "p1", parent)
	if branch.Truncated == nil || branch.Truncated.Shown != 0 || branch.Truncated.Total != 1 {
		t.Fatalf("parent branch must report truncation, got %+v", branch.Truncated)
	}
}

func TestNodesBeyondMaxNodesPerPluginAreTruncatedNotRefused(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")

	// Fill the budget through separate branches, each publish under the per-publish cap.
	roots := make([]discovery.Node, 0, 5)
	observed := []string{""}
	for i := range 5 {
		id := fmt.Sprintf("g%d", i)
		roots = append(roots, groupNode(id, ""))
		observed = append(observed, id)
	}
	h.service.SetObserved("c1", observed)
	mustPublish(t, h, "", roots...)

	for i := range 4 {
		parent := fmt.Sprintf("g%d", i)
		children := make([]discovery.Node, 0, 500)
		for j := range 500 {
			children = append(children, instanceNode(fmt.Sprintf("%s-%d", parent, j), parent))
		}
		mustPublish(t, h, parent, children...)
	}

	// 5 groups + 2000 leaves already exceed MaxNodesPerPlugin, so nothing new may be admitted.
	if err := h.publish(t, "p1", "s1", "g4", instanceNode("overflow", "g4")); err != nil {
		t.Fatalf("over-budget publish must be truncation, not refusal: %v", err)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); containsID(ids, "overflow") {
		t.Fatalf("node beyond MaxNodesPerPlugin must be dropped")
	}
	branch := branchOf(t, h.service.Snapshot("c1"), "p1", "g4")
	if branch.Truncated == nil || branch.Truncated.Shown != 0 || branch.Truncated.Total != 1 {
		t.Fatalf("branch must report truncation, got %+v", branch.Truncated)
	}
}

func TestPluginPublishedStaleIsIgnoredWhileTheSnapshotIsApplied(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	removed, err := h.store.ApplySnapshot("c1", "p1", "", discovery.BranchStale, "", []discovery.Node{instanceNode("a", "")})
	if err != nil {
		t.Fatalf("a stray host-only state must not fail the snapshot: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("nothing existed to remove, got %v", removed)
	}
	if ids := nodeIDsOf(h.service.Snapshot("c1")); !containsID(ids, "a") {
		t.Fatalf("the rest of the snapshot must still be applied, got %v", ids)
	}
	if state := branchOf(t, h.service.Snapshot("c1"), "p1", "").State; state == discovery.BranchStale {
		t.Fatalf("plugin must not be able to set stale, branch state is %q", state)
	}
}

func TestCascadeDeleteReportsEveryVanishedDescendant(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "a", "a1"})

	mustPublish(t, h, "", groupNode("a", ""))
	mustPublish(t, h, "a", groupNode("a1", "a"))
	mustPublish(t, h, "a1", instanceNode("a1x", "a1"))

	removed, err := h.store.ApplySnapshot("c1", "p1", "", discovery.BranchReady, "", nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	for _, want := range []string{"a", "a1", "a1x"} {
		if !containsID(removed, want) {
			t.Fatalf("removed set must include the whole subtree, missing %q in %v", want, removed)
		}
	}
}

func TestSetBranchStateDoesNotDiscardTruncation(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})

	children := make([]discovery.Node, 0, discovery.MaxChildrenPerPublish+1)
	for i := range discovery.MaxChildrenPerPublish + 1 {
		children = append(children, instanceNode(fmt.Sprintf("n%d", i), ""))
	}
	mustPublish(t, h, "", children...)

	h.store.SetBranchState("c1", "p1", "", discovery.BranchStale)
	branch := branchOf(t, h.service.Snapshot("c1"), "p1", "")
	if branch.State != discovery.BranchStale {
		t.Fatalf("host transition must apply, got %q", branch.State)
	}
	if branch.Truncated == nil || branch.Truncated.Total != discovery.MaxChildrenPerPublish+1 {
		t.Fatalf("truncation describes a past publish and must survive a state change, got %+v", branch.Truncated)
	}
}

func mustPublish(t *testing.T, h *discoveryHarness, nodeID string, children ...discovery.Node) {
	t.Helper()
	if err := h.publish(t, "p1", "s1", nodeID, children...); err != nil {
		t.Fatalf("publish %q: %v", nodeID, err)
	}
}
