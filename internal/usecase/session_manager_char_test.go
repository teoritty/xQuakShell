package usecase

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

func charTestSessionManager(t *testing.T) *SessionManager {
	t.Helper()
	bridge := NewPluginSessionBridge(PluginSessionBridgeConfig{})
	return NewSessionManager(SessionManagerConfig{PluginBridge: bridge})
}

func charTestEmbedPlugin(t *testing.T) (*PluginManager, *PluginSessionBridge) {
	t.Helper()
	registry := NewPluginRegistry()
	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vnc", Name: "VNC", Version: "1.0.0",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{
					Embed:             true,
					ConnectProtocols:  []string{"vnc"},
				},
			},
			Contributions: domainplugin.Contributions{
				ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{{
					ID:         "vnc",
					EmbedEntry: "ui/vnc.html",
				}},
			},
		},
	}
	if err := registry.Register(plugin); err != nil {
		t.Fatal(err)
	}
	pm := NewPluginManagerWithConfig(PluginManagerConfig{
		Registry:    registry,
		InstallRoot: t.TempDir(),
	})
	bridge := NewPluginSessionBridge(PluginSessionBridgeConfig{Plugins: pm})
	return pm, bridge
}

func charTestEmbedSessionManager(t *testing.T) (*SessionManager, string) {
	t.Helper()
	_, bridge := charTestEmbedPlugin(t)
	sm := NewSessionManager(SessionManagerConfig{PluginBridge: bridge})
	sm.SetEmbedTunnelService(newTestEmbedTunnelService())
	const sessionID = "sess-embed-1"
	if err := sm.BindPluginSessionForTest(sessionID, "com.test.vnc"); err != nil {
		t.Fatal(err)
	}
	sm.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.connectionID = "conn-vnc"
		e.info.Protocol = "vnc"
	})
	return sm, sessionID
}

func TestChar_HandlePluginUpdateState_Ready(t *testing.T) {
	sm := charTestSessionManager(t)
	const sessionID = "sess-term-1"
	if err := sm.BindPluginSessionForTest(sessionID, "com.test.plugin"); err != nil {
		t.Fatal(err)
	}

	var streamReady sync.WaitGroup
	streamReady.Add(1)
	sm.plugins.WireSessionRuntime(PluginSessionRuntimeConfig{
		Registry:      sm.registry,
		OnStreamReady: func(id string, ch <-chan []byte) {
			if id == sessionID && ch != nil {
				streamReady.Done()
			}
		},
	})

	if err := sm.HandlePluginUpdateState("com.test.plugin", sessionID, string(domain.SessionReady), ""); err != nil {
		t.Fatal(err)
	}
	info, err := sm.GetState(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if info.State != domain.SessionReady {
		t.Fatalf("state: got %q, want ready", info.State)
	}
	waitDone(t, &streamReady, time.Second)
}

func TestChar_HandlePluginUpdateState_IDOR(t *testing.T) {
	sm := charTestSessionManager(t)
	if err := sm.BindPluginSessionForTest("sess-1", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	err := sm.HandlePluginUpdateState("plugin-b", "sess-1", string(domain.SessionReady), "")
	if err != domain.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestChar_HandlePluginProcessCrashed(t *testing.T) {
	sm := charTestSessionManager(t)
	if err := sm.BindPluginSessionForTest("sess-crash", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	sm.registry.Mutate("sess-crash", func(e *sessionEntry) {
		e.info.State = domain.SessionReady
	})

	sm.HandlePluginProcessCrashed("plugin-a", "sess-crash")

	info, err := sm.GetState("sess-crash")
	if err != nil {
		t.Fatal(err)
	}
	if info.State != domain.SessionConnecting {
		t.Fatalf("state: got %q, want connecting", info.State)
	}
	if info.ErrorMessage != "Recovering from plugin crash" {
		t.Fatalf("message: got %q", info.ErrorMessage)
	}
}

func TestChar_FailPluginSessionRecovery(t *testing.T) {
	sm := charTestSessionManager(t)
	if err := sm.BindPluginSessionForTest("sess-fail", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	sm.registry.Mutate("sess-fail", func(e *sessionEntry) {
		e.info.State = domain.SessionConnecting
	})

	sm.FailPluginSessionRecovery("plugin-a", "sess-fail")

	info, err := sm.GetState("sess-fail")
	if err != nil {
		t.Fatal(err)
	}
	if info.State != domain.SessionError {
		t.Fatalf("state: got %q, want error", info.State)
	}
}

func TestChar_PluginOwnsSession(t *testing.T) {
	sm := charTestSessionManager(t)
	if err := sm.BindPluginSessionForTest("sess-own", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	if !sm.PluginOwnsSession("plugin-a", "sess-own") {
		t.Fatal("expected ownership")
	}
	if sm.PluginOwnsSession("plugin-b", "sess-own") {
		t.Fatal("expected no ownership for wrong plugin")
	}
}

func TestChar_HandlePluginRegisterEmbed(t *testing.T) {
	sm, sessionID := charTestEmbedSessionManager(t)
	ctx := context.Background()

	raw, err := sm.HandlePluginRegisterEmbed(ctx, "com.test.vnc", sessionID, "ui/vnc.html", []string{"main"})
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["embedToken"] == "" || resp["uiUrl"] == "" {
		t.Fatalf("expected embed response fields, got %v", resp)
	}
	pid, ok := sm.PluginIDForSession(sessionID)
	if !ok || pid != "com.test.vnc" {
		t.Fatalf("PluginIDForSession: got %q, ok=%v", pid, ok)
	}
}

func TestChar_HandlePluginRegisterEmbed_UIEntryMismatch(t *testing.T) {
	sm, sessionID := charTestEmbedSessionManager(t)
	_, err := sm.HandlePluginRegisterEmbed(context.Background(), "com.test.vnc", sessionID, "ui/wrong.html", nil)
	if err == nil {
		t.Fatal("expected uiEntry mismatch error")
	}
}

func TestChar_CloseSession_RevokesEmbed(t *testing.T) {
	sm, sessionID := charTestEmbedSessionManager(t)
	ctx := context.Background()
	raw, err := sm.HandlePluginRegisterEmbed(ctx, "com.test.vnc", sessionID, "ui/vnc.html", []string{"main"})
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	token, _ := resp["embedToken"].(string)

	if err := sm.CloseSession(sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.embed.Lookup(token); err == nil {
		t.Fatal("expected embed token revoked after close")
	}
}

func TestChar_RetrySession_NotHostKeyRequired(t *testing.T) {
	sm := charTestSessionManager(t)
	if err := sm.BindPluginSessionForTest("sess-retry", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	err := sm.RetrySession(context.Background(), "sess-retry")
	if err == nil {
		t.Fatal("expected error when not in hostkey-required state")
	}
}

func TestChar_GetAllSessions(t *testing.T) {
	sm := charTestSessionManager(t)
	if err := sm.BindPluginSessionForTest("sess-a", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	if err := sm.BindPluginSessionForTest("sess-b", "plugin-b"); err != nil {
		t.Fatal(err)
	}
	all := sm.GetAllSessions()
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}
}

func waitDone(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for callback")
	}
}
