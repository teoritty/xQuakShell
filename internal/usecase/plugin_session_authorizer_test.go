package usecase_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

func TestPluginSessionAuthorizerPerSessionIsolation(t *testing.T) {
	auth := usecase.NewPluginSessionAuthorizer(nil)
	if err := auth.AuthorizeSessionRPC("p", "sess-1", domainplugin.IsolationPerSession, false, "sess-2"); err != domainplugin.ErrSessionNotBound {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
	if err := auth.AuthorizeSessionRPC("p", "sess-1", domainplugin.IsolationPerSession, false, "sess-1"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestPluginSessionAuthorizerBindRejectsSecondSessionWithoutAllowMulti(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	_ = registry.Register(domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.one", Name: "O", Version: "1",
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Isolation: domainplugin.IsolationPerPlugin,
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{Terminal: true},
			},
		},
	})
	auth := usecase.NewPluginSessionAuthorizer(registry)
	if err := auth.BindSession("com.test.one", "sess-a"); err != nil {
		t.Fatal(err)
	}
	if err := auth.BindSession("com.test.one", "sess-b"); err != domainplugin.ErrSessionNotBound {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
}
