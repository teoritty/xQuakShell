package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/discovery"
)

// unlockedPluginSettings answers the one question the supervisor asks before restarting anything.
//
// It is overridden here because the composed runtime reads plugin settings from a vault that no
// test unlocks, PluginSettings() therefore returns an error, and isPluginEnabled reads an error as
// "disabled" — so the supervisor logs "skip restart for disabled plugin" for a plugin nobody
// disabled and never reaches the give-up path at all. That is a real defect on its own and is
// reported as one; it is stubbed out here rather than left to silently gut this test.
type unlockedPluginSettings struct{}

func (unlockedPluginSettings) PluginSettings() (domain.PluginSettings, error) {
	return domain.DefaultPluginSettings(), nil
}

// Wiring, again, and for the same reason as main_discovery_wiring_test.go: both halves of the
// crash story are unit-tested, and the seam between them is one line in newPluginRuntime that no
// test in this repository touched. Delete supervisor.SetGaveUpHandler and every unit test still
// passes while abandoned subtrees stay dimmed-and-stale forever, which is the exact silence the
// give-up hook was added to end.
//
// The leader's other new dependency — the plugin registry and manager it uses to start and
// authorize discovery plugins — needs no test here: they are constructor arguments, so a leader
// built without them does not compile. That is deliberately a stronger guarantee than this file
// can offer, and it is why they were not added as setters.

// TestSupervisorGivingUpFailsTheSubtreeThroughTheComposedRuntime drives the whole seam over the
// real composition: a tree published through the composed discovery service, a supervisor that
// cannot restart the plugin, and the branch state the user ends up looking at.
func TestSupervisorGivingUpFailsTheSubtreeThroughTheComposedRuntime(t *testing.T) {
	_, runtime := composeDiscoveryRuntime(t)

	const (
		pluginID  = "com.example.abandoned"
		sessionID = "sess-wiring"
		connID    = "conn-wiring"
	)

	// A tree to fail. The publish goes in through the service's own inbound port, which is what a
	// plugin's discovery.publish reaches after the gate and the IDOR check upstream of it.
	runtime.discoveryLeader.SessionReady(sessionID, connID)
	runtime.discoveryService.SetObserved(connID, []string{""})
	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"nodeId":    "",
		"state":     "ready",
		"children": []map[string]any{
			{"id": "n1", "parentId": "", "kind": "instance", "label": "One"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.discoveryService.Publish(context.Background(), pluginID, params); err != nil {
		t.Fatalf("publish through the composed service: %v", err)
	}
	if state := branchStateOf(t, runtime, connID, pluginID); state != discovery.BranchReady {
		t.Fatalf("the branch must start ready, got %q; the assertion below would prove nothing", state)
	}

	// HandleCrash only acts while the plugin still has sessions open, and it restarts a plugin the
	// registry has never heard of — so every attempt fails and the loop runs out for real.
	runtime.manager.SetSettingsReader(unlockedPluginSettings{})
	runtime.manager.SessionOpened(pluginID)
	runtime.supervisor.HandleCrash(pluginID, sessionID)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if branchStateOf(t, runtime, connID, pluginID) == discovery.BranchError {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the supervisor gave up and the subtree was never told: branch is still %q",
		branchStateOf(t, runtime, connID, pluginID))
}

// TestLeaderHandoverReachesTheFrontendThroughTheComposedRuntime covers the leader's own change
// callback, which was passed as nil for the whole of this branch's life.
//
// A handover marks every branch stale, and from that moment the backend refuses actions inside them
// (checkBranchActionable). With no callback the frontend was never told, so the rows kept rendering
// as live and a click came back with "branch is stale or failed" and nothing on screen to explain
// it. The existing wireEmbed test does not reach this: it asserts what the emit coalescer is wired
// to, and the leader deliberately bypasses the coalescer — a teardown deferred behind a 100 ms
// window would leave the user looking at a tree that no longer exists.
//
// The assertion goes through discoveryEmitHolder rather than comparing function pointers, because
// the holder is exactly what nil would have replaced: filling it and then driving a real handover
// proves the whole path, and needs no Wails runtime to do it.
func TestLeaderHandoverReachesTheFrontendThroughTheComposedRuntime(t *testing.T) {
	_, runtime := composeDiscoveryRuntime(t)

	changes := make(chan discoveryChange, 8)
	runtime.discoveryEmit.set(func(connectionID, nodeID string) {
		changes <- discoveryChange{connectionID: connectionID, nodeID: nodeID}
	})

	const connID = "conn-handover"
	runtime.discoveryLeader.SessionReady("sess-first", connID)
	runtime.discoveryLeader.SessionReady("sess-second", connID)
	drain(changes)

	// The leading session goes; the role passes to the second, every branch goes stale, and the
	// frontend must be told to repaint the connection.
	runtime.discoveryLeader.SessionClosed("sess-first", connID)

	select {
	case got := <-changes:
		if got.connectionID != connID {
			t.Fatalf("the handover named connection %q, want %q", got.connectionID, connID)
		}
		if got.nodeID != "" {
			t.Fatalf("a whole-connection change is addressed at the connection root, got node %q", got.nodeID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a leading-session handover told the frontend nothing: the rows keep rendering as " +
			"live while the backend already refuses every action inside them")
	}
}

// discoveryChange is what the frontend would be told: a connection and the node inside it that
// changed, with "" meaning the whole subtree.
type discoveryChange struct{ connectionID, nodeID string }

func drain(ch <-chan discoveryChange) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// TestPluginRetentionCheckerIsWired guards the one line that keeps a discovery plugin alive: with
// it unset, the idle sweeper reclaims the plugin after five quiet minutes and the supervisor
// declines to restart it, so every subtree it drew goes stale and stays there.
//
// It is asserted on the source, not by behaviour: making the composed leader hold a real binding
// needs an installed, startable discovery plugin, and a test that builds one to check a single
// assignment would be paying an order of magnitude more than the assignment is worth. The predicate
// itself, and both places that consult it, are covered by behaviour in internal/usecase. This is
// the same technique architecture.test.ts and discoveryState.test.ts use for wiring that cannot be
// exercised in-process.
func TestPluginRetentionCheckerIsWired(t *testing.T) {
	source := readCompositionSource(t)
	if !strings.Contains(source, "manager.SetPluginRetentionChecker(") {
		t.Fatal("main_plugins.go must call manager.SetPluginRetentionChecker; without it a plugin " +
			"drawing a subtree counts as idle and is suspended out from under the user")
	}
	if !strings.Contains(source, "discoveryLeader.HoldsBindings(pluginID)") {
		t.Fatal("the retention checker must consult discoveryLeader.HoldsBindings; without it a plugin " +
			"drawing a subtree counts as idle and is suspended out from under the user")
	}
	// The same trap, one capability later (ADR-015): a plugin streaming a log into a tab the user
	// is watching is silent on the RPC channel, which is exactly what idle looks like from here.
	if !strings.Contains(source, "surfaceService.HoldsSurfaces(pluginID)") {
		t.Fatal("the retention checker must consult surfaceService.HoldsSurfaces; without it a plugin " +
			"feeding an open surface is suspended out from under the user")
	}
}

// TestLeaderChangeCallbackIsNotNil pins the argument the behavioural test above depends on. The
// behavioural test would also fail if it were nil, but this one says which line to fix.
func TestLeaderChangeCallbackIsNotNil(t *testing.T) {
	source := readCompositionSource(t)
	if strings.Contains(source, "discoveryPace, nil)") {
		t.Fatal("NewDiscoveryLeader is still being handed a nil change callback in main_plugins.go")
	}
	if !strings.Contains(source, "discoveryPace, discoveryEmit.notify)") {
		t.Fatal("main_plugins.go must hand NewDiscoveryLeader the discoveryEmit.notify callback")
	}
}

func readCompositionSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("main_plugins.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	// Comments in this file discuss both wirings at length; strip them so the prose explaining a
	// rule cannot satisfy the rule.
	var out strings.Builder
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

func branchStateOf(t *testing.T, runtime *pluginRuntime, connID, pluginID string) discovery.BranchState {
	t.Helper()
	for _, tree := range runtime.discoveryService.Snapshot(connID).Plugins {
		if tree.PluginID != pluginID {
			continue
		}
		return tree.Branches[""].State
	}
	t.Fatalf("no tree for plugin %q under connection %q", pluginID, connID)
	return ""
}
