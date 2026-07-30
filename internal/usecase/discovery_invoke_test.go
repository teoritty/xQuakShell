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

	if err := h.service.InvokeAction(context.Background(), "c1", []string{"one", "two"}, "restart"); err != nil {
		t.Fatalf("valid mass action must be relayed: %v", err)
	}
	if h.caller.count() != 1 {
		t.Fatalf("expected one plugin call, got %d", h.caller.count())
	}
	call := h.caller.calls[0]
	if call.SessionID != "s1" || call.ActionID != "restart" || len(call.NodeIDs) != 2 {
		t.Fatalf("unexpected payload %+v", call)
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
			err := h.service.InvokeAction(context.Background(), "c1", tc.nodeIDs, tc.action)
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

	err := h.service.InvokeAction(context.Background(), "c1", []string{"one"}, "restart")
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

	err := h.service.InvokeAction(context.Background(), "c1", []string{"one"}, "restart")
	if !errors.Is(err, ErrDiscoveryBranchNotActionable) {
		t.Fatalf("expected ErrDiscoveryBranchNotActionable, got %v", err)
	}
	// A sibling branch that is healthy stays actionable: the block follows the ancestry, not the
	// whole connection.
	if err := h.service.InvokeAction(context.Background(), "c1", []string{"solo"}, "restart"); err != nil {
		t.Fatalf("a healthy branch must stay actionable: %v", err)
	}
}

func TestInvokeActionNeedsALeadingSession(t *testing.T) {
	h := invokeFixture(t)
	h.leader.SessionClosed("s1", "c1")

	err := h.service.InvokeAction(context.Background(), "c1", []string{"one"}, "restart")
	if err == nil {
		t.Fatal("an action on a connection with no ready session must be refused")
	}
	if h.caller.count() != 0 {
		t.Fatalf("no plugin call may be made, got %d", h.caller.count())
	}
}
