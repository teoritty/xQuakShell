package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
)

// --- fakes -----------------------------------------------------------------

type fakeDetailsCaller struct {
	calls    []string
	payloads []string
	reply    json.RawMessage
	err      error
}

func (c *fakeDetailsCaller) CallWithTimeout(ctx context.Context, pluginID, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	c.calls = append(c.calls, method)
	c.payloads = append(c.payloads, string(params))
	if c.err != nil {
		return nil, c.err
	}
	return c.reply, nil
}

type fakeDetailsLeader struct {
	sessionID    string
	connectionID string
	live         bool
}

func (l fakeDetailsLeader) Leading(connectionID string) (string, string, bool) {
	return l.sessionID, "ssh", l.live
}
func (l fakeDetailsLeader) ConnectionForSession(sessionID string) (string, bool) {
	if sessionID != l.sessionID {
		return "", false
	}
	return l.connectionID, true
}
func (l fakeDetailsLeader) Connections() []string { return []string{l.connectionID} }

const editableDetails = `{"sections":[{"id":"g","label":"G","fields":[
	{"id":"shell","label":"Shell","type":"text","secret":false}
]}],"values":{"shell":"/bin/sh"},"editable":true}`

// testDiscoveryPace builds the real pace with real time. publishDetails shares the publish budget
// (ADR-015 §Limits), so the tests exercise the same limiter production uses rather than a stand-in
// that could drift from it.
func testDiscoveryPace() *DiscoveryPace {
	return NewDiscoveryPace(
		NewDiscoveryPublishLimiter(nil),
		NewDiscoveryEmitCoalescer(nil, nil, nil),
	)
}

func detailsHarness(t *testing.T, reply string) (*DiscoveryDetailsService, *fakeDetailsCaller, *DiscoveryStore) {
	t.Helper()
	store := NewDiscoveryStore()
	if _, err := store.ApplySnapshot("conn-1", "plugin-a", "", discovery.BranchReady, "", []discovery.Node{
		{ID: "node-1", Kind: discovery.KindInstance, Label: "web"},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	caller := &fakeDetailsCaller{reply: json.RawMessage(reply)}
	svc := NewDiscoveryDetailsService(
		store,
		fakeDetailsLeader{sessionID: "sess-1", connectionID: "conn-1", live: true},
		caller,
		nil,
		func(pluginID string) *domainplugin.UICaps {
			return &domainplugin.UICaps{NodeDetails: true}
		},
		testDiscoveryPace(),
	)
	return svc, caller, store
}

// --- describe --------------------------------------------------------------

// The host answers questions about a node only while it is looking at one; otherwise a frontend
// could address any id at all through this path.
func TestDescribeNodeRejectsNodeNotInTheStore(t *testing.T) {
	svc, caller, _ := detailsHarness(t, editableDetails)
	_, err := svc.Describe(context.Background(), "conn-1", "plugin-a", "node-nope")
	if !errors.Is(err, ErrDiscoveryNodeNotFound) {
		t.Fatalf("got %v, want ErrDiscoveryNodeNotFound", err)
	}
	if len(caller.calls) != 0 {
		t.Fatal("an unknown node must not reach the plugin")
	}
}

func TestDescribeNodeRequiresALeadingSession(t *testing.T) {
	store := NewDiscoveryStore()
	_, _ = store.ApplySnapshot("conn-1", "plugin-a", "", discovery.BranchReady, "", []discovery.Node{
		{ID: "node-1", Kind: discovery.KindInstance, Label: "web"},
	})
	svc := NewDiscoveryDetailsService(store, fakeDetailsLeader{live: false}, &fakeDetailsCaller{}, nil, nil, testDiscoveryPace())
	if _, err := svc.Describe(context.Background(), "conn-1", "plugin-a", "node-1"); !errors.Is(err, ErrDiscoveryNoLeadingSession) {
		t.Fatalf("got %v, want ErrDiscoveryNoLeadingSession", err)
	}
}

// The session is how the host reaches the plugin and must not appear in what comes back — the
// separation ADR-014 set for the tree, kept here.
func TestDescribeNodeSendsTheSessionAndReturnsNoneOfIt(t *testing.T) {
	svc, caller, _ := detailsHarness(t, editableDetails)
	details, err := svc.Describe(context.Background(), "conn-1", "plugin-a", "node-1")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if !strings.Contains(caller.payloads[0], `"sessionId":"sess-1"`) {
		t.Fatalf("the plugin must be addressed by session: %s", caller.payloads[0])
	}
	if len(details.Sections) != 1 || details.Values["shell"] != "/bin/sh" {
		t.Fatalf("details = %+v", details)
	}
}

func TestDescribeNodeRejectsASecretFieldFromThePlugin(t *testing.T) {
	reply := `{"sections":[{"id":"g","label":"G","fields":[
		{"id":"p","label":"P","type":"password","secret":true}
	]}],"editable":true}`
	svc, _, _ := detailsHarness(t, reply)
	if _, err := svc.Describe(context.Background(), "conn-1", "plugin-a", "node-1"); err == nil {
		t.Fatal("expected a secret field in a details panel to be refused")
	}
}

func TestDescribeNodeReportsAnUnreachablePlugin(t *testing.T) {
	svc, caller, _ := detailsHarness(t, editableDetails)
	caller.err = errors.New("timeout")
	if _, err := svc.Describe(context.Background(), "conn-1", "plugin-a", "node-1"); !errors.Is(err, ErrDiscoveryDetailsUnavailable) {
		t.Fatalf("got %v, want ErrDiscoveryDetailsUnavailable", err)
	}
}

// --- apply -----------------------------------------------------------------

func TestApplyDetailsDropsUndeclaredKeys(t *testing.T) {
	svc, caller, _ := detailsHarness(t, editableDetails)
	err := svc.Apply(context.Background(), "conn-1", "plugin-a", "node-1", map[string]string{
		"shell":  "/bin/bash",
		"sneaky": "x",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Two calls: the describe that re-reads the declaration, then the apply itself.
	if len(caller.payloads) != 2 {
		t.Fatalf("calls = %v", caller.calls)
	}
	applied := caller.payloads[1]
	if strings.Contains(applied, "sneaky") {
		t.Fatalf("an undeclared key reached the plugin: %s", applied)
	}
	if !strings.Contains(applied, "/bin/bash") {
		t.Fatalf("the declared value was lost: %s", applied)
	}
}

// A panel the plugin marked read-only is describing facts about a remote resource, not preferences
// the user owns; saving one would be writing an answer nobody asked for.
func TestApplyDetailsRefusesAReadOnlyNode(t *testing.T) {
	svc, _, _ := detailsHarness(t, `{"sections":[{"id":"g","label":"G","fields":[
		{"id":"shell","label":"Shell","type":"text","secret":false}
	]}],"editable":false}`)
	err := svc.Apply(context.Background(), "conn-1", "plugin-a", "node-1", map[string]string{"shell": "x"})
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
}

func TestApplyDetailsIsAudited(t *testing.T) {
	var entries []domainplugin.DiscoveryAuditEntry
	store := NewDiscoveryStore()
	_, _ = store.ApplySnapshot("conn-1", "plugin-a", "", discovery.BranchReady, "", []discovery.Node{
		{ID: "node-1", Kind: discovery.KindInstance, Label: "web"},
	})
	svc := NewDiscoveryDetailsService(
		store,
		fakeDetailsLeader{sessionID: "sess-1", connectionID: "conn-1", live: true},
		&fakeDetailsCaller{reply: json.RawMessage(editableDetails)},
		func(e domainplugin.DiscoveryAuditEntry) { entries = append(entries, e) },
		nil,
		testDiscoveryPace(),
	)
	if err := svc.Apply(context.Background(), "conn-1", "plugin-a", "node-1", map[string]string{"shell": "/bin/sh"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Dispatch and result, matching InvokeAction: an apply that reached the plugin and then timed
	// out has still been dispatched.
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want 2", len(entries))
	}
	if entries[0].NodeIDs[0] != "node-1" || entries[0].ConnectionID != "conn-1" {
		t.Fatalf("audit entry = %+v", entries[0])
	}
}

// --- publishDetails --------------------------------------------------------

func TestPublishDetailsRequiresTheGrant(t *testing.T) {
	svc, _, _ := detailsHarness(t, editableDetails)
	svc.caps = func(string) *domainplugin.UICaps { return &domainplugin.UICaps{} }
	_, err := svc.PublishDetails(context.Background(), "plugin-a", json.RawMessage(`{"sessionId":"sess-1","nodeId":"node-1"}`))
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
}

func TestPublishDetailsRejectsASecretField(t *testing.T) {
	svc, _, _ := detailsHarness(t, editableDetails)
	params := `{"sessionId":"sess-1","nodeId":"node-1","sections":[{"id":"g","label":"G","fields":[
		{"id":"p","label":"P","type":"password","secret":true}
	]}]}`
	if _, err := svc.PublishDetails(context.Background(), "plugin-a", json.RawMessage(params)); err == nil {
		t.Fatal("expected a secret field to be refused")
	}
}

// A snapshot for a session that has stopped leading is accepted and dropped: the plugin is racing
// a handover, which ADR-014 treats as normal rather than as an error.
func TestPublishDetailsForANonLeadingSessionIsAcceptedAndDropped(t *testing.T) {
	svc, _, _ := detailsHarness(t, editableDetails)
	emitted := 0
	svc.SetEmitter(func(string, string, string) { emitted++ })
	if _, err := svc.PublishDetails(context.Background(), "plugin-a", json.RawMessage(`{"sessionId":"other","nodeId":"node-1"}`)); err != nil {
		t.Fatalf("a racing snapshot must be accepted, got %v", err)
	}
	if emitted != 0 {
		t.Fatal("a snapshot for a non-leading session must not reach the frontend")
	}
}

func TestPublishDetailsForTheLeadingSessionNotifiesTheFrontend(t *testing.T) {
	svc, _, _ := detailsHarness(t, editableDetails)
	var got [3]string
	svc.SetEmitter(func(connectionID, pluginID, nodeID string) {
		got = [3]string{connectionID, pluginID, nodeID}
	})
	if _, err := svc.PublishDetails(context.Background(), "plugin-a", json.RawMessage(`{"sessionId":"sess-1","nodeId":"node-1"}`)); err != nil {
		t.Fatalf("PublishDetails: %v", err)
	}
	if got != [3]string{"conn-1", "plugin-a", "node-1"} {
		t.Fatalf("emitted %v", got)
	}
}

// The budget is the publish budget, not a second one: a push costs the host a round trip back to
// the plugin when the panel re-reads, so an unmetered one is an amplifier (ADR-015 §Limits).
func TestPublishDetailsIsMeteredOnTheDiscoveryBudget(t *testing.T) {
	svc, _, _ := detailsHarness(t, editableDetails)
	emitted := 0
	svc.SetEmitter(func(string, string, string) { emitted++ })

	params := json.RawMessage(`{"sessionId":"sess-1","nodeId":"node-1"}`)
	var lastErr error
	for i := 0; i < discovery.MaxPublishPerSecond+5; i++ {
		if _, err := svc.PublishDetails(context.Background(), "plugin-a", params); err != nil {
			lastErr = err
			break
		}
	}
	if !errors.Is(lastErr, domainplugin.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited past the budget", lastErr)
	}
	if emitted > discovery.MaxPublishPerSecond {
		t.Fatalf("a refused push must not reach the frontend: emitted %d", emitted)
	}
}

// The budget is shared with discovery.publish and keyed the same way, so one plugin's spending
// cannot exhaust another's.
func TestPublishDetailsBudgetIsPerPlugin(t *testing.T) {
	svc, _, store := detailsHarness(t, editableDetails)
	if _, err := store.ApplySnapshot("conn-1", "plugin-b", "", discovery.BranchReady, "", []discovery.Node{
		{ID: "node-1", Kind: discovery.KindInstance, Label: "web"},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	svc.SetEmitter(func(string, string, string) {})

	params := json.RawMessage(`{"sessionId":"sess-1","nodeId":"node-1"}`)
	for i := 0; i < discovery.MaxPublishPerSecond+5; i++ {
		if _, err := svc.PublishDetails(context.Background(), "plugin-a", params); err != nil {
			break
		}
	}
	if _, err := svc.PublishDetails(context.Background(), "plugin-b", params); err != nil {
		t.Fatalf("another plugin's budget must be untouched: %v", err)
	}
}
