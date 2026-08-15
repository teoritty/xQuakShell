package usecase_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

type fakePeerTrustInbound struct {
	calls    int
	trusted  bool
	lastSub  string
	lastMat  []byte
	lastSess string
	lastPlug string
}

func (f *fakePeerTrustInbound) VerifyPeer(_ context.Context, pluginID, sessionID, subject string, material []byte) (bool, error) {
	f.calls++
	f.lastPlug, f.lastSess, f.lastSub = pluginID, sessionID, subject
	f.lastMat = append([]byte(nil), material...)
	return f.trusted, nil
}

func newTrustRPCHandler(inbound domainplugin.PeerTrustInboundPort) *usecase.PluginSessionRPCHandler {
	return usecase.NewPluginSessionRPCHandler(
		usecase.PluginSessionRPCPorts{
			Sessions:  usecase.NewPluginSessionInbound(),
			Embed:     usecase.NewPluginEmbedInbound(),
			PeerTrust: inbound,
		},
		usecase.NewPluginSessionAuthorizer(nil),
		usecase.PluginSessionScope{
			PluginID:         "plugin-a",
			ProcessSessionID: "sess-owned",
			Isolation:        domainplugin.IsolationPerSession,
		},
	)
}

func verifyPeerParamsJSON(t *testing.T, sessionID, subject string, material []byte) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{
		"sessionId":      sessionID,
		"subject":        subject,
		"materialBase64": base64.StdEncoding.EncodeToString(material),
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return raw
}

// Session ownership is checked BEFORE the question reaches the trust mechanism. Otherwise a
// plugin could raise a dialog about someone else's session, and the user would be shown a question
// about a connection they never opened.
func TestVerifyPeer_RejectsUnownedSession(t *testing.T) {
	inbound := &fakePeerTrustInbound{}
	handler := newTrustRPCHandler(inbound)

	_, err := handler.Handle(context.Background(), "plugin-a", "trust.verifyPeer",
		verifyPeerParamsJSON(t, "sess-foreign", "h:3389", []byte{1}))
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("err = %v, want ErrSessionNotBound", err)
	}
	if inbound.calls != 0 {
		t.Fatal("the trust mechanism was reached for a session the plugin does not own")
	}
}

// With no port wired the method is refused as an unavailable capability rather than panicking: a
// build without this service is a legitimate configuration.
func TestVerifyPeer_WithoutPortIsDenied(t *testing.T) {
	handler := newTrustRPCHandler(nil)

	_, err := handler.Handle(context.Background(), "plugin-a", "trust.verifyPeer",
		verifyPeerParamsJSON(t, "sess-owned", "h:3389", []byte{1}))
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}

// The material must reach the mechanism byte for byte: a mangled encoding would mean trusting a
// key other than the one the plugin observed.
func TestVerifyPeer_PassesMaterialAndSubjectThrough(t *testing.T) {
	inbound := &fakePeerTrustInbound{trusted: true}
	handler := newTrustRPCHandler(inbound)

	material := []byte{0x30, 0x59, 0x00, 0xFF}
	raw, err := handler.Handle(context.Background(), "plugin-a", "trust.verifyPeer",
		verifyPeerParamsJSON(t, "sess-owned", "10.0.0.5:3389", material))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if inbound.calls != 1 {
		t.Fatalf("mechanism calls = %d, want 1", inbound.calls)
	}
	if inbound.lastPlug != "plugin-a" || inbound.lastSess != "sess-owned" {
		t.Fatalf("plugin/session forwarded wrongly: %q / %q", inbound.lastPlug, inbound.lastSess)
	}
	if inbound.lastSub != "10.0.0.5:3389" {
		t.Fatalf("subject = %q", inbound.lastSub)
	}
	if string(inbound.lastMat) != string(material) {
		t.Fatalf("material was mangled: % x", inbound.lastMat)
	}

	var got struct {
		Trusted bool `json:"trusted"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.Trusted {
		t.Fatal("the response did not carry trusted:true")
	}
}

// The scope written into the trust record is the one the session binding was authorized against,
// not the id the dispatcher happened to be handed. Two sources for one quantity in a trust
// boundary is how the weaker one becomes the way in.
func TestVerifyPeer_ScopeComesFromTheAuthorizedBinding(t *testing.T) {
	inbound := &fakePeerTrustInbound{}
	handler := newTrustRPCHandler(inbound)

	if _, err := handler.Handle(context.Background(), "plugin-impostor", "trust.verifyPeer",
		verifyPeerParamsJSON(t, "sess-owned", "h:3389", []byte{1})); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if inbound.lastPlug != "plugin-a" {
		t.Fatalf("scope = %q, want the bound plugin-a", inbound.lastPlug)
	}
}

// Malformed base64 is untrusted input. It must be an error rather than decay into empty material
// that something further down reads as trust.
func TestVerifyPeer_RejectsMalformedMaterial(t *testing.T) {
	inbound := &fakePeerTrustInbound{}
	handler := newTrustRPCHandler(inbound)

	raw, err := json.Marshal(map[string]string{
		"sessionId":      "sess-owned",
		"subject":        "h:3389",
		"materialBase64": "!!!not base64!!!",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := handler.Handle(context.Background(), "plugin-a", "trust.verifyPeer", raw); err == nil {
		t.Fatal("malformed material was accepted")
	}
	if inbound.calls != 0 {
		t.Fatal("the trust mechanism was called with malformed material")
	}
}
