package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/usecase"
	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

type multiSessionSettings struct {
	granted bool
}

func (m multiSessionSettings) PluginSettings() (domain.PluginSettings, error) {
	settings := domain.PluginSettings{}
	if m.granted {
		settings.MultiSessionAccessGranted = map[string]bool{"plugin-a": true, "com.test.multi": true}
	}
	return settings, nil
}

func testSessionAuthorizer(t *testing.T, registry *usecase.PluginRegistry, settings usecase.PluginSettingsReader) *usecase.PluginSessionAuthorizer {
	t.Helper()
	auth := usecase.NewPluginSessionAuthorizer(registry)
	if settings != nil {
		auth.SetSettingsReader(settings)
	}
	return auth
}

func testProcessHost(t *testing.T, auth domainplugin.SessionRPCAuthorizer, inbound domainplugin.SessionInboundPort) *infraplugin.ProcessHost {
	t.Helper()
	return infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionAuthorizer: auth,
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, auth),
	})
}

func TestDiscoveryLoadsUnsignedWithoutChecksums(t *testing.T) {
	exeDir := t.TempDir()
	pluginID := "com.test.bundled-no-sum"
	dir := filepath.Join(exeDir, "plugins", "bundled")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "` + pluginID + `",
		"name": "Bundled",
		"version": "1.0.0",
		"engine": {"type": "go-binary", "entry": "p.exe"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	loaded, err := infraplugin.LoadPluginDir(dir)
	if err != nil {
		t.Fatalf("unsigned bundled plugin without SHA256SUMS should load: %v", err)
	}
	if loaded.ChecksumsDigest != "" {
		t.Fatalf("expected empty ChecksumsDigest, got %q", loaded.ChecksumsDigest)
	}

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(exeDir, ""))
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected one unsigned plugin, got %d", len(plugins))
	}
}

func TestBundledPluginLoadsWithChecksums(t *testing.T) {
	exeDir := t.TempDir()
	dir := filepath.Join(exeDir, "plugins", "bundled")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "com.test.bundled-sum",
		"name": "Bundled",
		"version": "1.0.0",
		"engine": {"type": "go-binary", "entry": "p.exe"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(exeDir, ""))
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected bundled plugin with checksums to load, got %d", len(plugins))
	}
}

func TestSamePluginCrossSessionUpdateStateDeniedPerSession(t *testing.T) {
	auth := testSessionAuthorizer(t, nil, nil)
	inbound := usecase.NewPluginSessionInbound()
	proxy := usecase.NewPluginSessionRPCHandler(inbound, auth, usecase.PluginSessionScope{
		PluginID:         "plugin-a",
		ProcessSessionID: "sess-1",
		Isolation:        domainplugin.IsolationPerSession,
	})

	params, _ := json.Marshal(map[string]string{
		"sessionId": "sess-2",
		"state":     string(domain.SessionReady),
	})
	_, err := proxy.Handle(context.Background(), "plugin-a", "session.updateState", params)
	if err != domainplugin.ErrSessionNotBound {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
}

func TestSamePluginCrossSessionAllowedWithBindingPerPlugin(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "plugin-a", Name: "A", Version: "1",
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Isolation: domainplugin.IsolationPerPlugin,
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{AllowMultiSession: true},
			},
		},
	})
	auth := testSessionAuthorizer(t, registry, multiSessionSettings{granted: true})
	_ = testProcessHost(t, auth, usecase.NewPluginSessionInbound())
	manager := usecase.NewSessionManager(usecase.SessionManagerConfig{})
	inbound := usecase.NewPluginSessionInbound()
	inbound.SetHandler(manager)

	const sessionA = "sess-a"
	const sessionB = "sess-b"
	if err := manager.BindPluginSessionForTest(sessionA, "plugin-a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.BindPluginSessionForTest(sessionB, "plugin-a"); err != nil {
		t.Fatal(err)
	}

	if err := auth.BindSession("plugin-a", sessionA); err != nil {
		t.Fatal(err)
	}
	if err := auth.BindSession("plugin-a", sessionB); err != nil {
		t.Fatal(err)
	}

	proxy := usecase.NewPluginSessionRPCHandler(inbound, auth, usecase.PluginSessionScope{
		PluginID:          "plugin-a",
		Isolation:         domainplugin.IsolationPerPlugin,
		AllowMultiSession: true,
	})

	params, _ := json.Marshal(map[string]string{
		"sessionId": sessionB,
		"state":     string(domain.SessionReady),
	})
	if _, err := proxy.Handle(context.Background(), "plugin-a", "session.updateState", params); err != nil {
		t.Fatalf("expected bound session ok, got %v", err)
	}
}

func TestSessionRPCDeniedWhenNotBound(t *testing.T) {
	auth := testSessionAuthorizer(t, nil, nil)
	inbound := usecase.NewPluginSessionInbound()
	proxy := usecase.NewPluginSessionRPCHandler(inbound, auth, usecase.PluginSessionScope{
		PluginID:  "plugin-a",
		Isolation: domainplugin.IsolationPerPlugin,
	})

	params, _ := json.Marshal(map[string]string{
		"sessionId": "sess-orphan",
		"state":     string(domain.SessionReady),
	})
	_, err := proxy.Handle(context.Background(), "plugin-a", "session.updateState", params)
	if err != domainplugin.ErrSessionNotBound {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
}

func TestProcessHostBindSessionDelegatesToUsecaseAuthorizer(t *testing.T) {
	auth := testSessionAuthorizer(t, nil, nil)
	inbound := usecase.NewPluginSessionInbound()
	host := testProcessHost(t, auth, inbound)

	if err := host.BindSession("plugin-a", "sess-1"); err != nil {
		t.Fatal(err)
	}

	manager := usecase.NewSessionManager(usecase.SessionManagerConfig{})
	if err := manager.BindPluginSessionForTest("sess-1", "plugin-a"); err != nil {
		t.Fatal(err)
	}
	inbound.SetHandler(manager)
	proxy := usecase.NewPluginSessionRPCHandler(inbound, auth, usecase.PluginSessionScope{
		PluginID:   "plugin-a",
		Isolation:  domainplugin.IsolationPerPlugin,
	})
	params, _ := json.Marshal(map[string]string{
		"sessionId": "sess-1",
		"state":     string(domain.SessionReady),
	})
	if _, err := proxy.Handle(context.Background(), "plugin-a", "session.updateState", params); err != nil {
		t.Fatalf("expected bound session via host delegation, got %v", err)
	}
}

func TestBindSessionRejectsSecondSessionWithoutAllowMultiSession(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.single", Name: "S", Version: "1",
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Isolation: domainplugin.IsolationPerPlugin,
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{Terminal: true, AllowMultiSession: false},
			},
		},
	})
	auth := testSessionAuthorizer(t, registry, nil)
	host := testProcessHost(t, auth, usecase.NewPluginSessionInbound())

	if err := host.BindSession("com.test.single", "sess-a"); err != nil {
		t.Fatal(err)
	}
	if err := host.BindSession("com.test.single", "sess-b"); err != domainplugin.ErrSessionNotBound {
		t.Fatalf("expected ErrSessionNotBound for second session, got %v", err)
	}
}

func TestBindSessionRejectsSecondSessionWithoutInstallConsent(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.multi", Name: "M", Version: "1",
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Isolation: domainplugin.IsolationPerPlugin,
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{AllowMultiSession: true},
			},
		},
	})
	auth := testSessionAuthorizer(t, registry, multiSessionSettings{granted: false})
	host := testProcessHost(t, auth, usecase.NewPluginSessionInbound())

	if err := host.BindSession("com.test.multi", "sess-a"); err != nil {
		t.Fatal(err)
	}
	if err := host.BindSession("com.test.multi", "sess-b"); err != domainplugin.ErrSessionNotBound {
		t.Fatalf("expected ErrSessionNotBound without install consent, got %v", err)
	}
}

func TestBindSessionAllowsSecondSessionWithAllowMultiSession(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.multi", Name: "M", Version: "1",
			Engine:    domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Isolation: domainplugin.IsolationPerPlugin,
			Capabilities: domainplugin.CapabilitySet{
				Session: &domainplugin.SessionCaps{AllowMultiSession: true},
			},
		},
	})
	auth := testSessionAuthorizer(t, registry, multiSessionSettings{granted: true})
	host := testProcessHost(t, auth, usecase.NewPluginSessionInbound())

	if err := host.BindSession("com.test.multi", "sess-a"); err != nil {
		t.Fatal(err)
	}
	if err := host.BindSession("com.test.multi", "sess-b"); err != nil {
		t.Fatalf("expected second session bind ok, got %v", err)
	}
}
