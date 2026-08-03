package plugin_test

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/usecase"
)

func TestPluginFieldsServiceSaveAndResolve(t *testing.T) {
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "test-pass"); err != nil {
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
	if err := vaultRepo.Create(ctx, "test-pass"); err != nil {
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

func authMethodManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.test.authmethod",
		Name:    "AuthMethod",
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
								{
									ID:    "auth_type",
									Label: "Auth type",
									Type:  domainplugin.FieldTypeSelect,
									Options: []domainplugin.FieldOption{
										{Value: "password", Label: "Password"},
										{Value: "ssh_key", Label: "SSH key"},
									},
								},
								// Truthy dependsOn hides a field only when the dependency
								// value is empty. Plugins gate password/ssh_key fields via
								// sentinel values the UI clears when switching auth mode.
								{
									ID:    "password_active",
									Label: "Password active",
									Type:  domainplugin.FieldTypeText,
								},
								{
									ID:        "password",
									Label:     "Password",
									Type:      domainplugin.FieldTypePassword,
									Secret:    true,
									DependsOn: "password_active",
								},
								{
									ID:    "ssh_key_active",
									Label: "SSH key active",
									Type:  domainplugin.FieldTypeText,
								},
								{
									ID:        "ssh_key",
									Label:     "SSH key",
									Type:      domainplugin.FieldTypeTextarea,
									DependsOn: "ssh_key_active",
								},
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
}

func TestPluginFieldsServiceClearsHiddenSecretOnAuthTypeChange(t *testing.T) {
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}

	registry := usecase.NewPluginRegistry()
	manifest := authMethodManifest()
	if err := manifest.ValidateCapabilitiesAndFields(); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	svc := usecase.NewPluginFieldsService(vaultRepo, registry)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	conn := &domain.Connection{ID: "conn-auth", Host: "h", Protocol: "telnet", Port: 23}
	if err := connRepo.Save(ctx, conn); err != nil {
		t.Fatal(err)
	}

	if err := svc.SavePluginFields(ctx, conn, map[string]string{
		"auth_type":        "password",
		"password_active":  "1",
		"password":         "old-secret",
	}); err != nil {
		t.Fatal(err)
	}

	data, err := vaultRepo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.PluginSecrets) != 1 {
		t.Fatalf("expected one secret before auth change, got %d", len(data.PluginSecrets))
	}

	if err := svc.SavePluginFields(ctx, conn, map[string]string{
		"auth_type":       "ssh_key",
		"password_active": "",
		"ssh_key_active":  "1",
		"ssh_key":         "-----BEGIN KEY-----",
	}); err != nil {
		t.Fatal(err)
	}

	if conn.PluginFields["password"] != "" {
		t.Fatalf("password ref should be cleared from connection, got %q", conn.PluginFields["password"])
	}
	data, err = vaultRepo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	for ref := range data.PluginSecrets {
		if ref == "secret:conn-auth.password" {
			t.Fatalf("hidden password secret should be deleted from vault, still present: %q", ref)
		}
	}
	if conn.PluginFields["ssh_key"] != "-----BEGIN KEY-----" {
		t.Fatalf("ssh_key not persisted: %q", conn.PluginFields["ssh_key"])
	}
}

func TestPluginFieldsServiceSkipsRequiredWhenHidden(t *testing.T) {
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}

	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.requiredhidden",
		Name:    "RequiredHidden",
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
								{
									ID:    "auth_type",
									Label: "Auth type",
									Type:  domainplugin.FieldTypeSelect,
									Options: []domainplugin.FieldOption{
										{Value: "password", Label: "Password"},
										{Value: "ssh_key", Label: "SSH key"},
									},
								},
								{
									ID:    "password_active",
									Label: "Password active",
									Type:  domainplugin.FieldTypeText,
								},
								{
									ID:        "password",
									Label:     "Password",
									Type:      domainplugin.FieldTypePassword,
									Secret:    true,
									Required:  true,
									DependsOn: "password_active",
								},
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
	if err := manifest.ValidateCapabilitiesAndFields(); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}

	svc := usecase.NewPluginFieldsService(vaultRepo, registry)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	conn := &domain.Connection{ID: "conn-req", Host: "h", Protocol: "telnet", Port: 23}
	if err := connRepo.Save(ctx, conn); err != nil {
		t.Fatal(err)
	}

	if err := svc.SavePluginFields(ctx, conn, map[string]string{
		"auth_type":       "ssh_key",
		"password_active": "",
	}); err != nil {
		t.Fatalf("hidden required field should not block save: %v", err)
	}
}

func TestPluginFieldsServiceRejectsEmptyVisibleRequired(t *testing.T) {
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}

	registry := usecase.NewPluginRegistry()
	manifest := domainplugin.Manifest{
		ID:      "com.test.requiredvisible",
		Name:    "RequiredVisible",
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
								{
									ID:       "username",
									Label:    "User",
									Type:     domainplugin.FieldTypeText,
									Required: true,
								},
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
	conn := &domain.Connection{ID: "conn-reqvis", Host: "h", Protocol: "telnet", Port: 23}
	if err := connRepo.Save(ctx, conn); err != nil {
		t.Fatal(err)
	}

	if err := svc.SavePluginFields(ctx, conn, map[string]string{"username": ""}); err == nil {
		t.Fatal("expected visible required field to block save")
	}
}
