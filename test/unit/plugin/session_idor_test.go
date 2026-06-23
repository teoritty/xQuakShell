package plugin_test

import (
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/usecase"
)

func TestCrossPluginSessionUpdateStateIDOR(t *testing.T) {
	manager := usecase.NewSessionManager(usecase.SessionManagerConfig{})

	sessionID := openBoundPluginSession(t, manager, "plugin-a")

	err := manager.HandlePluginUpdateState("plugin-b", sessionID, string(domain.SessionReady), "")
	if err != domain.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestCrossPluginWriteTerminalIDOR(t *testing.T) {
	manager := usecase.NewSessionManager(usecase.SessionManagerConfig{})

	sessionID := openBoundPluginSession(t, manager, "plugin-a")

	err := manager.HandlePluginWriteTerminal("plugin-b", sessionID, []byte("hello"))
	if err != domain.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func openBoundPluginSession(t *testing.T, manager *usecase.SessionManager, pluginID string) string {
	t.Helper()
	const sessionID = "sess-idor-test"
	if err := manager.BindPluginSessionForTest(sessionID, pluginID); err != nil {
		t.Fatal(err)
	}
	return sessionID
}
