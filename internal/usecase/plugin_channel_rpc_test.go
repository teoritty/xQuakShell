package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

type fakeChannelInbound struct {
	openCalls int
}

func (f *fakeChannelInbound) Open(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	f.openCalls++
	return json.Marshal(map[string]uint32{"channelId": 1})
}

func (f *fakeChannelInbound) Close(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"ok": true})
}

// TestChannelOpen_RejectsUnownedParentSession is the required test that channel.open with a
// parentSessionId the plugin does not own an active binding for is denied via the existing
// SessionRPCAuthorizer, before the channel backend is ever invoked (IDOR, checked first).
func TestChannelOpen_RejectsUnownedParentSession(t *testing.T) {
	auth := usecase.NewPluginSessionAuthorizer(nil)
	channels := &fakeChannelInbound{}
	handler := usecase.NewPluginSessionRPCHandler(
		usecase.NewPluginSessionInbound(),
		usecase.NewPluginEmbedInbound(),
		channels,
		nil, nil, auth, usecase.PluginSessionScope{
			PluginID:         "plugin-a",
			ProcessSessionID: "sess-owned",
			Isolation:        domainplugin.IsolationPerSession,
		},
	)

	params, _ := json.Marshal(map[string]string{
		"parentSessionId": "sess-foreign",
		"purpose":         "exec",
	})
	_, err := handler.Handle(context.Background(), "plugin-a", "channel.open", params)
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
	if channels.openCalls != 0 {
		t.Fatal("channel backend must not be invoked when parentSessionId is not owned")
	}
}

func TestChannelOpen_AllowsOwnedParentSession(t *testing.T) {
	auth := usecase.NewPluginSessionAuthorizer(nil)
	channels := &fakeChannelInbound{}
	handler := usecase.NewPluginSessionRPCHandler(
		usecase.NewPluginSessionInbound(),
		usecase.NewPluginEmbedInbound(),
		channels,
		nil, nil, auth, usecase.PluginSessionScope{
			PluginID:         "plugin-a",
			ProcessSessionID: "sess-owned",
			Isolation:        domainplugin.IsolationPerSession,
		},
	)

	params, _ := json.Marshal(map[string]string{
		"parentSessionId": "sess-owned",
		"purpose":         "exec",
	})
	if _, err := handler.Handle(context.Background(), "plugin-a", "channel.open", params); err != nil {
		t.Fatalf("expected owned parentSessionId to be allowed through to the backend: %v", err)
	}
	if channels.openCalls != 1 {
		t.Fatalf("expected 1 channel backend call, got %d", channels.openCalls)
	}
}
