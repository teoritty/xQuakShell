package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// fakeDiscoveryInbound records whether a snapshot was ever handed to the discovery usecase. The
// assertion that matters in an IDOR test is not the error the plugin gets back but that nothing
// downstream ever saw the payload.
type fakeDiscoveryInbound struct {
	publishCalls int
	sessionIDs   []string
}

func (f *fakeDiscoveryInbound) Publish(_ context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	f.publishCalls++
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(params, &payload)
	f.sessionIDs = append(f.sessionIDs, payload.SessionID)
	return nil, nil
}

func newDiscoveryRPCHandler(discovery domainplugin.DiscoveryInboundPort) *usecase.PluginSessionRPCHandler {
	return usecase.NewPluginSessionRPCHandler(
		usecase.NewPluginSessionInbound(),
		usecase.NewPluginEmbedInbound(),
		nil,
		discovery,
		nil,
		nil,
		nil,
		usecase.NewPluginSessionAuthorizer(nil),
		usecase.PluginSessionScope{
			PluginID:         "plugin-a",
			ProcessSessionID: "sess-owned",
			Isolation:        domainplugin.IsolationPerSession,
		},
	)
}

// TestDiscoveryPublish_RejectsUnownedSession is the required IDOR test, and it mirrors
// TestChannelOpen_RejectsUnownedParentSession deliberately: publish and channel.open name a session
// the same way, so they must be refused by the same authorizer on the same path. A future change
// that moves one and not the other will break exactly one of these two tests.
func TestDiscoveryPublish_RejectsUnownedSession(t *testing.T) {
	discovery := &fakeDiscoveryInbound{}
	handler := newDiscoveryRPCHandler(discovery)

	params, _ := json.Marshal(map[string]any{
		"sessionId": "sess-foreign",
		"nodeId":    "",
		"state":     "ready",
		"children":  []any{map[string]any{"id": "n1", "kind": "instance", "label": "n1"}},
	})
	_, err := handler.Handle(context.Background(), "plugin-a", "discovery.publish", params)
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
	if discovery.publishCalls != 0 {
		t.Fatalf("discovery must not see a snapshot for a session the plugin does not own, got %d calls", discovery.publishCalls)
	}
}

// TestDiscoveryPublish_RejectsEmptySession covers the shape an attacker actually reaches for once
// the check above is known to exist: omit the field entirely and hope the authorizer is skipped for
// want of anything to compare.
func TestDiscoveryPublish_RejectsEmptySession(t *testing.T) {
	discovery := &fakeDiscoveryInbound{}
	handler := newDiscoveryRPCHandler(discovery)

	params, _ := json.Marshal(map[string]any{"nodeId": "", "state": "ready"})
	_, err := handler.Handle(context.Background(), "plugin-a", "discovery.publish", params)
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("expected ErrSessionNotBound for a missing sessionId, got %v", err)
	}
	if discovery.publishCalls != 0 {
		t.Fatalf("a publish with no sessionId must not reach discovery, got %d calls", discovery.publishCalls)
	}
}

func TestDiscoveryPublish_AllowsOwnedSession(t *testing.T) {
	discovery := &fakeDiscoveryInbound{}
	handler := newDiscoveryRPCHandler(discovery)

	params, _ := json.Marshal(map[string]any{
		"sessionId": "sess-owned",
		"nodeId":    "",
		"state":     "ready",
	})
	if _, err := handler.Handle(context.Background(), "plugin-a", "discovery.publish", params); err != nil {
		t.Fatalf("expected an owned session to be allowed through: %v", err)
	}
	if discovery.publishCalls != 1 {
		t.Fatalf("expected 1 publish to reach discovery, got %d", discovery.publishCalls)
	}
	// The whole, unmodified params blob must reach the usecase: this layer authorizes the session
	// and decodes nothing else.
	if len(discovery.sessionIDs) != 1 || discovery.sessionIDs[0] != "sess-owned" {
		t.Fatalf("expected the original payload to be forwarded intact, got %v", discovery.sessionIDs)
	}
}

// TestDiscoveryPublish_DeniedWithoutInboundPort pins the nil-port behaviour to the same answer
// channel.open gives: a capability denial, never a panic and never a silent success.
func TestDiscoveryPublish_DeniedWithoutInboundPort(t *testing.T) {
	handler := newDiscoveryRPCHandler(nil)
	params, _ := json.Marshal(map[string]any{"sessionId": "sess-owned"})
	_, err := handler.Handle(context.Background(), "plugin-a", "discovery.publish", params)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected ErrCapabilityDenied with no discovery port wired, got %v", err)
	}
}
