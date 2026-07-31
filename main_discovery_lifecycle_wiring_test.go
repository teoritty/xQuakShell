package main

import (
	"context"
	"encoding/json"
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
