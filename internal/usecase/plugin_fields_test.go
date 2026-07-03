package usecase_test

import (
	"context"
	"testing"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/persistence"
	"ssh-client/internal/usecase"
)

func TestPluginFieldsServiceSaveAndResolve(t *testing.T) {
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Unlock(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}

	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.fields",
		Name:    "Fields",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "telnet",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "auth",
							Label: "Auth",
							Fields: []domainplugin.FieldDef{
								{ID: "username", Label: "User", Type: domainplugin.FieldTypeText, Required: true, Secret: false},
								{ID: "password", Label: "Pass", Type: domainplugin.FieldTypePassword, Secret: true},
							},
						},
					},
				},
			},
		},
		Capabilities: domainplugin.CapabilitySet{
			Session: &domainplugin.SessionCaps{ConnectProtocols: []string{"telnet"}, Terminal: true},
		},
		Isolation: domainplugin.IsolationPerSession,
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	svc := usecase.NewPluginFieldsService(vaultRepo, registry)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	conn := &domain.Connection{
		ID:       "conn-1",
		Host:     "example.com",
		Protocol: "telnet",
	}
	if err := connRepo.Save(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := svc.SavePluginFields(ctx, conn, map[string]string{
		"username": "admin",
		"password": "secret123",
	}); err != nil {
		t.Fatal(err)
	}

	protoDef := registry.GetProtocolDef(manifest.ID, "telnet")
	resolved, err := svc.ResolvePluginFields(ctx, conn, protoDef)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["username"] != "admin" {
		t.Fatalf("username: %q", resolved["username"])
	}
	if resolved["password"] != "secret123" {
		t.Fatalf("password not resolved")
	}
}

func TestPluginFieldsServiceClearsEmptySecret(t *testing.T) {
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Unlock(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}

	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.clear",
		Name:    "Clear",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "telnet",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "auth",
							Label: "Auth",
							Fields: []domainplugin.FieldDef{
								{ID: "password", Label: "Pass", Type: domainplugin.FieldTypePassword, Secret: true},
							},
						},
					},
				},
			},
		},
		Capabilities: domainplugin.CapabilitySet{
			Session: &domainplugin.SessionCaps{ConnectProtocols: []string{"telnet"}, Terminal: true},
		},
		Isolation: domainplugin.IsolationPerSession,
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	svc := usecase.NewPluginFieldsService(vaultRepo, registry)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	conn := &domain.Connection{ID: "conn-2", Host: "h", Protocol: "telnet", Port: 23}
	if err := connRepo.Save(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := svc.SavePluginFields(ctx, conn, map[string]string{"password": "pw"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SavePluginFields(ctx, conn, map[string]string{"password": ""}); err != nil {
		t.Fatal(err)
	}

	data, err := vaultRepo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.PluginSecrets) != 0 {
		t.Fatalf("expected secrets cleared, got %d", len(data.PluginSecrets))
	}
}
