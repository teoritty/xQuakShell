package wails

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"xquakshell/internal/domain/discovery"
	"xquakshell/internal/usecase"
)

// recordingDiscovery is a stand-in for the discovery use case that records exactly what the handler
// asked it to do. The point of the fake is the arguments: everything these tests are about happens
// on the way in.
type recordingDiscovery struct {
	snapshots []string

	observedConn  string
	observedNodes []string
	observedCalls int

	invocations []discoveryInvocation
	invokeErr   error

	tree map[string]usecase.DiscoverySnapshot
}

type discoveryInvocation struct {
	connectionID string
	pluginID     string
	nodeIDs      []string
	actionID     string
}

func (r *recordingDiscovery) Snapshot(connectionID string) usecase.DiscoverySnapshot {
	r.snapshots = append(r.snapshots, connectionID)
	if snapshot, ok := r.tree[connectionID]; ok {
		return snapshot
	}
	return usecase.DiscoverySnapshot{ConnectionID: connectionID}
}

func (r *recordingDiscovery) SetObserved(connectionID string, nodeIDs []string) {
	r.observedCalls++
	r.observedConn = connectionID
	// Copied into a non-nil slice on purpose: the difference between "an empty set arrived" and
	// "nil arrived" is precisely what one of these tests is about.
	r.observedNodes = append(make([]string, 0, len(nodeIDs)), nodeIDs...)
}

func (r *recordingDiscovery) InvokeAction(_ context.Context, connectionID, pluginID string, nodeIDs []string, actionID string) error {
	r.invocations = append(r.invocations, discoveryInvocation{
		connectionID: connectionID,
		pluginID:     pluginID,
		nodeIDs:      append([]string(nil), nodeIDs...),
		actionID:     actionID,
	})
	return r.invokeErr
}

func newDiscoveryAPI(svc DiscoveryTreeService) *AppAPI {
	api := &AppAPI{}
	api.SetDiscoveryService(svc)
	return api
}

// TestSetDiscoveryObservedForwardsTheWholeExpandedSet pins the level-triggered contract at the
// boundary: whatever the frontend says is expanded is what the use case is told, in full. A handler
// that diffed against a previous call would turn ADR-014's level into an edge, and a single lost
// update would then leave a branch empty with nothing able to repair it.
func TestSetDiscoveryObservedForwardsTheWholeExpandedSet(t *testing.T) {
	svc := &recordingDiscovery{}
	api := newDiscoveryAPI(svc)

	if err := api.SetDiscoveryObserved("conn-1", []string{"", "docker"}); err != nil {
		t.Fatalf("first SetDiscoveryObserved: %v", err)
	}
	if err := api.SetDiscoveryObserved("conn-1", []string{"", "docker", "docker/containers"}); err != nil {
		t.Fatalf("second SetDiscoveryObserved: %v", err)
	}

	if svc.observedCalls != 2 {
		t.Fatalf("expected both calls to reach the use case, got %d", svc.observedCalls)
	}
	want := []string{"", "docker", "docker/containers"}
	if !reflect.DeepEqual(svc.observedNodes, want) {
		t.Fatalf("expected the full set %v, got %v — a delta would have been {docker/containers}", want, svc.observedNodes)
	}
	if svc.observedConn != "conn-1" {
		t.Fatalf("connection id not forwarded: %q", svc.observedConn)
	}
}

// TestSetDiscoveryObservedCollapsingEverythingIsNotSilentlyDropped covers the empty set, which is
// what the frontend sends when the user collapses the connection root. It is a real instruction —
// stop enumerating — not an absence of one, so it must not be turned into a no-op on the way past.
func TestSetDiscoveryObservedCollapsingEverythingIsNotSilentlyDropped(t *testing.T) {
	svc := &recordingDiscovery{}
	api := newDiscoveryAPI(svc)

	if err := api.SetDiscoveryObserved("conn-1", nil); err != nil {
		t.Fatalf("SetDiscoveryObserved: %v", err)
	}
	if svc.observedCalls != 1 {
		t.Fatalf("expected the empty set to reach the use case, got %d calls", svc.observedCalls)
	}
	if svc.observedNodes == nil || len(svc.observedNodes) != 0 {
		t.Fatalf("expected an empty, non-nil set, got %#v", svc.observedNodes)
	}
}

// TestInvokeDiscoveryActionRoutesByStatedPluginNotByNodeID is the regression this handler exists
// for. Two plugins legitimately publish a node with the SAME id under one connection; if the
// addressee were inferred from the node id, the owner would be chosen by map iteration order, and a
// destructive action would reach the wrong plugin a fraction of the time. Both pairs are exercised,
// with the node id held constant, so a routing that ignored pluginId cannot pass by luck.
func TestInvokeDiscoveryActionRoutesByStatedPluginNotByNodeID(t *testing.T) {
	svc := &recordingDiscovery{}
	api := newDiscoveryAPI(svc)

	if err := api.InvokeDiscoveryAction("conn-1", "plugin-a", []string{"containers"}, "restart"); err != nil {
		t.Fatalf("first invoke: %v", err)
	}
	if err := api.InvokeDiscoveryAction("conn-1", "plugin-b", []string{"containers"}, "restart"); err != nil {
		t.Fatalf("second invoke: %v", err)
	}

	if len(svc.invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(svc.invocations))
	}
	if svc.invocations[0].pluginID != "plugin-a" {
		t.Fatalf("first action went to %q, want plugin-a", svc.invocations[0].pluginID)
	}
	if svc.invocations[1].pluginID != "plugin-b" {
		t.Fatalf("second action went to %q, want plugin-b", svc.invocations[1].pluginID)
	}
	for i, got := range svc.invocations {
		if got.connectionID != "conn-1" || got.actionID != "restart" {
			t.Fatalf("invocation %d lost its addressing: %+v", i, got)
		}
		if !reflect.DeepEqual(got.nodeIDs, []string{"containers"}) {
			t.Fatalf("invocation %d node list: %v", i, got.nodeIDs)
		}
	}
}

// TestInvokeDiscoveryActionForwardsMassSelectionsWhole checks that a multi-node selection travels
// as one call with the whole list. There is no separate bulk verb by design: a single action is a
// mass action over a list of one, and splitting a selection into per-node calls here would give the
// plugin no way to see it was one gesture.
func TestInvokeDiscoveryActionForwardsMassSelectionsWhole(t *testing.T) {
	svc := &recordingDiscovery{}
	api := newDiscoveryAPI(svc)

	nodes := []string{"c1", "c2", "c3"}
	if err := api.InvokeDiscoveryAction("conn-1", "plugin-a", nodes, "stop"); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if len(svc.invocations) != 1 {
		t.Fatalf("expected one call carrying the whole selection, got %d", len(svc.invocations))
	}
	if !reflect.DeepEqual(svc.invocations[0].nodeIDs, nodes) {
		t.Fatalf("selection changed on the way through: %v", svc.invocations[0].nodeIDs)
	}
}

// TestInvokeDiscoveryActionReturnsTheUseCaseRefusal makes sure a refusal (stale branch, unknown
// node, no leading session) reaches the frontend instead of being swallowed into a silent success.
func TestInvokeDiscoveryActionReturnsTheUseCaseRefusal(t *testing.T) {
	refusal := errors.New("branch is stale")
	api := newDiscoveryAPI(&recordingDiscovery{invokeErr: refusal})

	if err := api.InvokeDiscoveryAction("conn-1", "plugin-a", []string{"c1"}, "stop"); !errors.Is(err, refusal) {
		t.Fatalf("expected the refusal to propagate, got %v", err)
	}
}

// TestGetDiscoveryTreeForAConnectionWithNoTreeIsEmptyNotAnError covers the ordinary case for every
// user who has no discovery plugins at all: an empty snapshot, no error, and — because the frontend
// iterates it directly — a non-nil plugins array rather than a JSON null.
func TestGetDiscoveryTreeForAConnectionWithNoTreeIsEmptyNotAnError(t *testing.T) {
	api := newDiscoveryAPI(&recordingDiscovery{})

	snapshot, err := api.GetDiscoveryTree("conn-unknown")
	if err != nil {
		t.Fatalf("expected no error for a connection with no tree, got %v", err)
	}
	if snapshot.ConnectionID != "conn-unknown" {
		t.Fatalf("snapshot lost its connection id: %q", snapshot.ConnectionID)
	}
	if snapshot.Plugins == nil || len(snapshot.Plugins) != 0 {
		t.Fatalf("expected an empty, non-nil plugin list, got %#v", snapshot.Plugins)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"plugins":[]`) {
		t.Fatalf("empty tree must serialize as an empty array, got %s", encoded)
	}
}

// TestDiscoveryHandlersWithoutAServiceDoNotPanic covers the window before wiring and any build
// where discovery is absent. Reading yields emptiness; commanding yields a stated error rather than
// pretending it worked.
func TestDiscoveryHandlersWithoutAServiceDoNotPanic(t *testing.T) {
	api := &AppAPI{}

	snapshot, err := api.GetDiscoveryTree("conn-1")
	if err != nil {
		t.Fatalf("GetDiscoveryTree: %v", err)
	}
	if snapshot.ConnectionID != "conn-1" || len(snapshot.Plugins) != 0 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if err := api.SetDiscoveryObserved("conn-1", []string{""}); !errors.Is(err, errDiscoveryUnavailable) {
		t.Fatalf("expected errDiscoveryUnavailable, got %v", err)
	}
	if err := api.InvokeDiscoveryAction("conn-1", "plugin-a", []string{"n"}, "stop"); !errors.Is(err, errDiscoveryUnavailable) {
		t.Fatalf("expected errDiscoveryUnavailable, got %v", err)
	}
	// No Wails context: an emit must be a no-op, not a nil dereference on the runtime.
	api.EmitDiscoveryTreeChanged("conn-1", "docker")
}

// TestGetDiscoveryTreeMapsTheWholeReadModel checks the mapping itself: the DTO is a separate
// contract from the use case read model, so every field the frontend draws with has to be carried
// across deliberately. A forgotten field here is a row that renders blank.
func TestGetDiscoveryTreeMapsTheWholeReadModel(t *testing.T) {
	api := newDiscoveryAPI(&recordingDiscovery{tree: map[string]usecase.DiscoverySnapshot{"conn-1": fullDiscoverySnapshot()}})

	snapshot, err := api.GetDiscoveryTree("conn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Plugins) != 1 || snapshot.Plugins[0].PluginID != "plugin-a" {
		t.Fatalf("plugin grouping lost: %+v", snapshot.Plugins)
	}
	tree := snapshot.Plugins[0]
	if len(tree.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(tree.Nodes))
	}
	group, instance := tree.Nodes[0], tree.Nodes[1]
	if group.ID != "grp" || group.Kind != string(discovery.KindGroup) || group.Label != "Group" {
		t.Fatalf("group node mangled: %+v", group)
	}
	// The RESOLVED icon, not the node's own field: the use case walked the ancestors, and the child
	// below inherits the group's icon while declaring none of its own.
	if group.IconID != "grp-icon" || instance.IconID != "grp-icon" {
		t.Fatalf("resolved icons not carried: group=%q instance=%q", group.IconID, instance.IconID)
	}
	if instance.ParentID != "grp" || instance.Order != 7 || instance.DefaultActionID != "open" {
		t.Fatalf("instance node mangled: %+v", instance)
	}
	if group.Status != nil {
		t.Fatalf("a node with no status must stay statusless, got %+v", group.Status)
	}
	if instance.Status == nil || instance.Status.Tone != string(discovery.ToneWarn) ||
		instance.Status.Color != "#ff8800" || instance.Status.Tooltip != "degraded" {
		t.Fatalf("status not carried: %+v", instance.Status)
	}
	if len(instance.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(instance.Actions))
	}
	action := instance.Actions[0]
	if action.ID != "open" || action.Label != "Open" || action.IconID != "act-icon" ||
		!action.Danger || action.Confirm != "Sure?" || !action.Multi {
		t.Fatalf("action not carried: %+v", action)
	}
	branch, ok := tree.Branches["grp"]
	if !ok {
		t.Fatalf("branch state missing: %+v", tree.Branches)
	}
	if branch.State != string(discovery.BranchReady) || branch.Error != "partial" {
		t.Fatalf("branch not carried: %+v", branch)
	}
	if branch.Truncated == nil || branch.Truncated.Shown != 500 || branch.Truncated.Total != 812 {
		t.Fatalf("truncation not carried: %+v", branch.Truncated)
	}
}

// TestDiscoveryDTOCarriesNoSessionID is the boundary check ADR-014 makes normative: the frontend
// addresses everything by connectionId, and sessionId is a backend transport detail resolved in the
// leader. It is done by marshalling a fully populated snapshot and searching the bytes rather than
// by reading the struct, because a field somebody adds later is exactly what reading misses.
func TestDiscoveryDTOCarriesNoSessionID(t *testing.T) {
	api := newDiscoveryAPI(&recordingDiscovery{tree: map[string]usecase.DiscoverySnapshot{"conn-1": fullDiscoverySnapshot()}})
	snapshot, err := api.GetDiscoveryTree("conn-1")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	// Guard against the check passing on a snapshot that happens to be empty.
	for _, marker := range []string{`"connectionId":"conn-1"`, `"pluginId":"plugin-a"`, `"nodes"`, `"branches"`, `"actions"`, `"status"`, `"truncated"`} {
		if !strings.Contains(string(encoded), marker) {
			t.Fatalf("snapshot under test is not fully populated, missing %s: %s", marker, encoded)
		}
	}
	assertNoSessionID(t, "snapshot", encoded)

	payload, err := json.Marshal(DiscoveryTreeChangedPayload{ConnectionID: "conn-1", NodeID: "grp"})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSessionID(t, "event payload", payload)

	assertNoSessionIDInTags(t, reflect.TypeOf(DiscoverySnapshotDTO{}), map[reflect.Type]bool{})
	assertNoSessionIDInTags(t, reflect.TypeOf(DiscoveryTreeChangedPayload{}), map[reflect.Type]bool{})
}

func assertNoSessionID(t *testing.T, what string, encoded []byte) {
	t.Helper()
	if strings.Contains(strings.ToLower(string(encoded)), "session") {
		t.Fatalf("%s leaks a session to the frontend: %s", what, encoded)
	}
}

// assertNoSessionIDInTags walks the DTO type graph so that a session-shaped field added later is
// caught even when the test data leaves it empty and omitempty hides it from the marshalled bytes.
func assertNoSessionIDInTags(t *testing.T, typ reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if strings.Contains(strings.ToLower(field.Name), "session") ||
			strings.Contains(strings.ToLower(field.Tag.Get("json")), "session") {
			t.Fatalf("%s.%s exposes a session to the frontend", typ.Name(), field.Name)
		}
		assertNoSessionIDInTags(t, field.Type, seen)
	}
}

// TestDiscoveryTreeChangedPayloadNamesBothConnectionAndNode: the coalescing window upstream is per
// node, so an event without the node would leave the frontend able only to refetch everything, and
// the interval would end up bounding events instead of re-rendering.
func TestDiscoveryTreeChangedPayloadNamesBothConnectionAndNode(t *testing.T) {
	encoded, err := json.Marshal(DiscoveryTreeChangedPayload{ConnectionID: "conn-1", NodeID: "grp"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["connectionId"] != "conn-1" || decoded["nodeId"] != "grp" {
		t.Fatalf("event payload must name both connection and node, got %s", encoded)
	}
	// The connection root is a legitimate node id and must survive as an empty string rather than
	// being omitted: "the whole subtree changed" is what a plugin stopping reports.
	root, err := json.Marshal(DiscoveryTreeChangedPayload{ConnectionID: "conn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), `"nodeId":""`) {
		t.Fatalf("root-level change must still carry nodeId, got %s", root)
	}
}

// fullDiscoverySnapshot builds a read model with every optional field populated, shaped the way the
// store produces one: pre-order nodes, branches keyed by node id, icons already resolved.
func fullDiscoverySnapshot() usecase.DiscoverySnapshot {
	return usecase.DiscoverySnapshot{
		ConnectionID: "conn-1",
		Plugins: []usecase.DiscoveryPluginTreeView{{
			PluginID: "plugin-a",
			Nodes: []usecase.DiscoveryNodeView{
				{
					Node: discovery.Node{
						ID:     "grp",
						Kind:   discovery.KindGroup,
						Label:  "Group",
						IconID: "grp-icon",
					},
					Icon: "grp-icon",
				},
				{
					Node: discovery.Node{
						ID:              "inst",
						ParentID:        "grp",
						Kind:            discovery.KindInstance,
						Label:           "Instance",
						Order:           7,
						Status:          &discovery.Status{Tone: discovery.ToneWarn, Color: "#ff8800", Tooltip: "degraded"},
						Actions:         []discovery.Action{{ID: "open", Label: "Open", IconID: "act-icon", Danger: true, Confirm: "Sure?", Multi: true}},
						DefaultActionID: "open",
					},
					// Inherited from the group: the node declared no icon of its own.
					Icon: "grp-icon",
				},
			},
			Branches: map[string]discovery.Branch{
				"grp": {State: discovery.BranchReady, Error: "partial", Truncated: &discovery.Truncated{Shown: 500, Total: 812}},
			},
		}},
	}
}
