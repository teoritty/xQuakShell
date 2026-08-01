package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain/discovery"
)

// invokeFixture builds a connection whose root holds two instances sharing a multi-capable
// "restart" action and a single-node-only "logs" action.
func invokeFixture(t *testing.T) *discoveryHarness {
	t.Helper()
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{"", "group"})

	restart := discovery.Action{ID: "restart", Label: "Restart", Multi: true}
	logs := discovery.Action{ID: "logs", Label: "Logs"}
	mustPublish(t, h, "", groupNode("group", ""), instanceNode("solo", "", restart, logs))
	mustPublish(t, h, "group", instanceNode("one", "group", restart, logs), instanceNode("two", "group", restart, logs))
	return h
}

func TestInvokeActionRelaysAValidSelection(t *testing.T) {
	h := invokeFixture(t)

	if err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"one", "two"}, "restart"); err != nil {
		t.Fatalf("valid mass action must be relayed: %v", err)
	}
	if h.caller.count() != 1 {
		t.Fatalf("expected one plugin call, got %d", h.caller.count())
	}
	call := h.caller.firstCall(t)
	if call.pluginID != "p1" {
		t.Fatalf("action went to the wrong plugin: %q", call.pluginID)
	}
	if call.payload.SessionID != "s1" || call.payload.ActionID != "restart" || len(call.payload.NodeIDs) != 2 {
		t.Fatalf("unexpected payload %+v", call.payload)
	}
}

// TestInvokeActionAddressesTheNamedPluginWhenNodeIDsCollide pins the reason InvokeAction takes a
// pluginID at all. Node IDs are plugin-chosen and each plugin owns its own tree, so two plugins may
// both publish "containers" under one connection. Resolving the owner by searching the trees would
// pick one by map iteration order, which Go randomizes — a destructive action would reach the wrong
// plugin a fraction of the time, and a single-run test would not notice.
func TestInvokeActionAddressesTheNamedPluginWhenNodeIDsCollide(t *testing.T) {
	for attempt := range 40 {
		h := newDiscoveryHarness(t,
			DiscoveryPluginTarget{PluginID: "pA", ParentProtocols: []string{"ssh"}},
			DiscoveryPluginTarget{PluginID: "pB", ParentProtocols: []string{"ssh"}},
		)
		h.sessionReady("s1", "c1")
		h.service.SetObserved("c1", []string{""})

		restart := discovery.Action{ID: "restart", Label: "Restart"}
		h.publishAs(t, "pA", "s1", "", instanceNode("containers", "", restart))
		h.publishAs(t, "pB", "s1", "", instanceNode("containers", "", restart))

		if err := h.service.InvokeAction(context.Background(), "c1", "pB", []string{"containers"}, "restart"); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if got := h.caller.firstCall(t).pluginID; got != "pB" {
			t.Fatalf("attempt %d: action reached %q, not the plugin it was addressed to", attempt, got)
		}
	}
}

func TestInvokeActionRefusesANodeThatBelongsToAnotherPlugin(t *testing.T) {
	h := newDiscoveryHarness(t,
		DiscoveryPluginTarget{PluginID: "pA", ParentProtocols: []string{"ssh"}},
		DiscoveryPluginTarget{PluginID: "pB", ParentProtocols: []string{"ssh"}},
	)
	h.sessionReady("s1", "c1")
	h.service.SetObserved("c1", []string{""})
	h.publishAs(t, "pA", "s1", "", instanceNode("only-in-a", "", discovery.Action{ID: "restart", Label: "Restart"}))

	err := h.service.InvokeAction(context.Background(), "c1", "pB", []string{"only-in-a"}, "restart")
	if !errors.Is(err, ErrDiscoveryNodeNotFound) {
		t.Fatalf("a node from another plugin's tree must not be reachable, got %v", err)
	}
	if h.caller.count() != 0 {
		t.Fatalf("nothing may reach a plugin, got %d calls", h.caller.count())
	}
}

func TestInvokeActionRefusalsNeverReachThePlugin(t *testing.T) {
	tooMany := make([]string, discovery.MaxNodesPerInvoke+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("n%d", i)
	}

	cases := []struct {
		name    string
		nodeIDs []string
		action  string
		want    error
	}{
		{"empty selection", nil, "restart", ErrDiscoveryInvokeSize},
		{"over MaxNodesPerInvoke", tooMany, "restart", ErrDiscoveryInvokeSize},
		{"node that no longer exists", []string{"one", "vanished"}, "restart", ErrDiscoveryNodeNotFound},
		{"action the node does not have", []string{"one"}, "nosuch", ErrDiscoveryActionUnavailable},
		{"non-multi action on two nodes", []string{"one", "two"}, "logs", ErrDiscoveryActionUnavailable},
		{"nodes from different parents", []string{"one", "solo"}, "restart", ErrDiscoveryMixedParents},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := invokeFixture(t)
			err := h.service.InvokeAction(context.Background(), "c1", "p1", tc.nodeIDs, tc.action)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if h.caller.count() != 0 {
				t.Fatalf("a refused action must not reach the plugin, got %d calls", h.caller.count())
			}
		})
	}
}

func TestInvokeActionIsBlockedThroughoutAStaleBranchesSubtree(t *testing.T) {
	h := invokeFixture(t)

	// The connection root goes stale — a leading-session handover — which must block actions on
	// everything beneath it, not merely on the root's own children.
	h.store.SetBranchState("c1", "p1", "", discovery.BranchStale)

	err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"one"}, "restart")
	if !errors.Is(err, ErrDiscoveryBranchNotActionable) {
		t.Fatalf("expected ErrDiscoveryBranchNotActionable, got %v", err)
	}
	if h.caller.count() != 0 {
		t.Fatalf("a stale branch must not reach the plugin, got %d calls", h.caller.count())
	}
}

func TestInvokeActionIsBlockedInAFailedBranch(t *testing.T) {
	h := invokeFixture(t)
	h.store.SetBranchState("c1", "p1", "group", discovery.BranchError)

	err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"one"}, "restart")
	if !errors.Is(err, ErrDiscoveryBranchNotActionable) {
		t.Fatalf("expected ErrDiscoveryBranchNotActionable, got %v", err)
	}
	// A sibling branch that is healthy stays actionable: the block follows the ancestry, not the
	// whole connection.
	if err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"solo"}, "restart"); err != nil {
		t.Fatalf("a healthy branch must stay actionable: %v", err)
	}
}

func TestInvokeActionNeedsALeadingSession(t *testing.T) {
	h := invokeFixture(t)
	h.leader.SessionClosed("s1", "c1")

	err := h.service.InvokeAction(context.Background(), "c1", "p1", []string{"one"}, "restart")
	if err == nil {
		t.Fatal("an action on a connection with no ready session must be refused")
	}
	if h.caller.count() != 0 {
		t.Fatalf("no plugin call may be made, got %d", h.caller.count())
	}
}
