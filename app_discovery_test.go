package main

import (
	"context"
	"reflect"
	"testing"

	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// Wails binds App, not AppAPI, so these one-line delegates ARE the surface the frontend calls.
// app_bindings_test.go only proves each method exists; it cannot see a delegate that forwards its
// arguments in the wrong order — four adjacent string/[]string parameters compile happily in any
// permutation, and the resulting bug is precisely the one ADR-014 and the invoker's pluginID
// argument exist to prevent: an action delivered to the wrong plugin.

// recordingDiscoveryTree records what the delegate passed down, in the parameters it landed in.
type recordingDiscoveryTree struct {
	snapshotConnections []string

	observedConn  string
	observedNodes []string

	invokeConn   string
	invokePlugin string
	invokeNodes  []string
	invokeAction string
}

func (r *recordingDiscoveryTree) Snapshot(connectionID string) usecase.DiscoverySnapshot {
	r.snapshotConnections = append(r.snapshotConnections, connectionID)
	return usecase.DiscoverySnapshot{ConnectionID: connectionID}
}

func (r *recordingDiscoveryTree) SetObserved(connectionID string, nodeIDs []string) {
	r.observedConn = connectionID
	r.observedNodes = append(make([]string, 0, len(nodeIDs)), nodeIDs...)
}

func (r *recordingDiscoveryTree) InvokeAction(_ context.Context, connectionID, pluginID string, nodeIDs []string, actionID string) error {
	r.invokeConn = connectionID
	r.invokePlugin = pluginID
	r.invokeNodes = append(make([]string, 0, len(nodeIDs)), nodeIDs...)
	r.invokeAction = actionID
	return nil
}

func newRecordingDiscoveryApp() (*App, *recordingDiscoveryTree) {
	rec := &recordingDiscoveryTree{}
	api := &presentation.AppAPI{}
	api.SetDiscoveryService(rec)
	return &App{api: api}, rec
}

// TestAppInvokeDiscoveryActionKeepsArgumentOrder pins the connection/plugin pair specifically.
// Every value is distinguishable on sight, so a swapped pair cannot pass by coincidence.
func TestAppInvokeDiscoveryActionKeepsArgumentOrder(t *testing.T) {
	app, rec := newRecordingDiscoveryApp()

	if err := app.InvokeDiscoveryAction("conn-1", "plugin-a", []string{"node-1", "node-2"}, "restart"); err != nil {
		t.Fatalf("InvokeDiscoveryAction: %v", err)
	}
	if rec.invokeConn != "conn-1" {
		t.Errorf("connectionId landed as %q, want conn-1", rec.invokeConn)
	}
	if rec.invokePlugin != "plugin-a" {
		t.Errorf("pluginId landed as %q, want plugin-a — an action addressed to the wrong plugin is exactly what ADR-014 forbids", rec.invokePlugin)
	}
	if !reflect.DeepEqual(rec.invokeNodes, []string{"node-1", "node-2"}) {
		t.Errorf("nodeIds landed as %v", rec.invokeNodes)
	}
	if rec.invokeAction != "restart" {
		t.Errorf("actionId landed as %q, want restart", rec.invokeAction)
	}
}

// TestAppDiscoveryReadAndObserveDelegatesKeepArgumentOrder covers the other two delegates. Distinct
// connection ids per call also prove each one forwards its own argument rather than a shared value.
func TestAppDiscoveryReadAndObserveDelegatesKeepArgumentOrder(t *testing.T) {
	app, rec := newRecordingDiscoveryApp()

	snapshot, err := app.GetDiscoveryTree("conn-read")
	if err != nil {
		t.Fatalf("GetDiscoveryTree: %v", err)
	}
	if !reflect.DeepEqual(rec.snapshotConnections, []string{"conn-read"}) {
		t.Errorf("GetDiscoveryTree forwarded %v", rec.snapshotConnections)
	}
	if snapshot.ConnectionID != "conn-read" {
		t.Errorf("snapshot came back for %q", snapshot.ConnectionID)
	}

	if err := app.SetDiscoveryObserved("conn-observe", []string{"", "node-1"}); err != nil {
		t.Fatalf("SetDiscoveryObserved: %v", err)
	}
	if rec.observedConn != "conn-observe" {
		t.Errorf("SetDiscoveryObserved forwarded connection %q", rec.observedConn)
	}
	if !reflect.DeepEqual(rec.observedNodes, []string{"", "node-1"}) {
		t.Errorf("SetDiscoveryObserved forwarded nodes %v", rec.observedNodes)
	}
}

// TestAppDiscoveryDelegatesReturnBackendErrors makes sure a delegate propagates a refusal instead
// of dropping it: the frontend's only signal that an action was denied is this return value.
func TestAppDiscoveryDelegatesReturnBackendErrors(t *testing.T) {
	app := &App{api: &presentation.AppAPI{}} // no discovery service wired

	if err := app.SetDiscoveryObserved("conn-1", nil); err == nil {
		t.Error("expected SetDiscoveryObserved to report the missing service")
	}
	if err := app.InvokeDiscoveryAction("conn-1", "plugin-a", []string{"n"}, "stop"); err == nil {
		t.Error("expected InvokeDiscoveryAction to report the missing service")
	}
	if _, err := app.GetDiscoveryTree("conn-1"); err != nil {
		t.Errorf("reading a tree that does not exist is not an error: %v", err)
	}
}
