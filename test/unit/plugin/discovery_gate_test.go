package plugin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/ipc"
)

// discoveryPublishParams is a syntactically valid snapshot. It is deliberately well-formed: a
// denial must come from the capability gate, not from a payload the handler could not parse.
func discoveryPublishParams(t *testing.T) json.RawMessage {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"sessionId": "sess-1",
		"nodeId":    "",
		"state":     "ready",
		"children":  []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return params
}

func discoveryManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "disco",
		Name:    "Disco",
		Version: "1.0.0",
		Capabilities: domainplugin.CapabilitySet{
			Discovery: &domainplugin.DiscoveryCaps{ParentProtocols: []string{"ssh"}},
		},
	}
}

// recordingDiscoveryInbound proves whether the snapshot reached the usecase layer at all. A gate
// that denied the method but let the payload through would still fail this test.
type recordingDiscoveryInbound struct {
	calls int
}

func (r *recordingDiscoveryInbound) Publish(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	r.calls++
	return nil, nil
}

// sessionRPCFunc adapts a function to domainplugin.SessionRPCHandler.
type sessionRPCFunc func(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error)

func (f sessionRPCFunc) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	return f(ctx, pluginID, method, params)
}

// TestDiscoveryPublishDeniedWithoutCapability pins the two observable consequences ADR-014 promises
// for an undeclared discovery.publish: the -32001 the plugin sees, and the audit line the operator
// sees. Both matter — a silent denial is indistinguishable from a lost message during an incident.
func TestDiscoveryPublishDeniedWithoutCapability(t *testing.T) {
	var audited []string
	inbound := &recordingDiscoveryInbound{}
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "no-disco",
		// A manifest with no discovery capability at all: the plugin never asked for it.
		Gate: newGate(t, domainplugin.Manifest{ID: "no-disco", Name: "No Disco", Version: "1.0.0"}),
		Sessions: sessionRPCFunc(func(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
			return inbound.Publish(ctx, pluginID, params)
		}),
		Audit: func(pluginID, method string, denied bool, detail string) {
			if denied {
				audited = append(audited, pluginID+" "+method)
			}
		},
	})

	_, rpcErr := server.HandleRequest(context.Background(), "discovery.publish", discoveryPublishParams(t))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("expected capability denied -32001, got %#v", rpcErr)
	}
	if inbound.calls != 0 {
		t.Fatalf("a denied publish must never reach the discovery usecase, got %d calls", inbound.calls)
	}
	if len(audited) != 1 || !strings.Contains(audited[0], "discovery.publish") {
		t.Fatalf("expected one audited denial naming discovery.publish, got %v", audited)
	}
}

// TestDiscoveryPublishAllowedWithCapability is the other half: the same request, the same server,
// one manifest field different. Without it the test above would still pass if the gate denied
// discovery.publish unconditionally.
func TestDiscoveryPublishAllowedWithCapability(t *testing.T) {
	inbound := &recordingDiscoveryInbound{}
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "disco",
		Gate:     newGate(t, discoveryManifest()),
		Sessions: sessionRPCFunc(func(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
			if method != "discovery.publish" {
				t.Fatalf("unexpected method routed to the session handler: %q", method)
			}
			return inbound.Publish(ctx, pluginID, params)
		}),
		Audit: func(pluginID, method string, denied bool, detail string) {
			if denied {
				t.Fatalf("a declared discovery.publish must not be audited as denied: %s %s", pluginID, method)
			}
		},
	})

	if _, rpcErr := server.HandleRequest(context.Background(), "discovery.publish", discoveryPublishParams(t)); rpcErr != nil {
		t.Fatalf("expected discovery.publish to pass the gate, got %#v", rpcErr)
	}
	if inbound.calls != 1 {
		t.Fatalf("expected the snapshot to reach the discovery usecase once, got %d", inbound.calls)
	}
}

// TestDiscoveryHostToPluginVerbsAreNotGated documents why observe and invokeAction have no gate
// entry: they travel host->plugin. A plugin cannot call them, so the gate's default denial is the
// correct and only answer, and the capability that would grant publish must not accidentally grant
// these as inbound methods too.
func TestDiscoveryHostToPluginVerbsAreNotGated(t *testing.T) {
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "disco",
		Gate:     newGate(t, discoveryManifest()),
	})
	for _, method := range []string{"discovery.observe", "discovery.invokeAction"} {
		_, rpcErr := server.HandleRequest(context.Background(), method, nil)
		if rpcErr == nil || rpcErr.Code != -32001 {
			t.Fatalf("%s must be denied inbound even with the discovery capability, got %#v", method, rpcErr)
		}
	}
}
