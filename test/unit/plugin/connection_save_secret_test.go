package plugin_test

import (
	"context"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// renameProtocolManifest declares one required non-secret field and one required secret, the shape
// every "host + credentials" protocol has and the shape the reported RDP defect needed.
func renameProtocolManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.test.rename",
		Name:    "Rename",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "rdp",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "auth",
							Label: "Auth",
							Fields: []domainplugin.FieldDef{
								{ID: "username", Label: "User", Type: domainplugin.FieldTypeText, Required: true},
								{ID: "password", Label: "Pass", Type: domainplugin.FieldTypePassword, Required: true, Secret: true},
							},
						},
					},
				},
			},
		},
		Capabilities: domainplugin.CapabilitySet{
			Session: &domainplugin.SessionCaps{ConnectProtocols: []string{"rdp"}, Terminal: true},
		},
		Isolation: domainplugin.IsolationPerSession,
	}
}

// newRenameHarness returns a VaultService wired to real vault-backed repositories and a registry
// holding renameProtocolManifest, plus the vault repo so a test can inspect the stored secrets.
func newRenameHarness(t *testing.T) (*usecase.VaultService, *persistence.VaultRepo) {
	t.Helper()
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}
	registry := usecase.NewPluginRegistry()
	if err := registry.Register(domainplugin.InstalledPlugin{Manifest: renameProtocolManifest()}); err != nil {
		t.Fatal(err)
	}
	svc := usecase.NewVaultService(usecase.VaultServiceConfig{
		ConnRepo:     persistence.NewConnectionRepo(vaultRepo),
		PasswordRepo: persistence.NewPasswordRepo(vaultRepo),
		IdentRepo:    persistence.NewIdentityRepo(vaultRepo, nil, nil),
		PluginFields: usecase.NewPluginFieldsService(vaultRepo, registry),
	})
	return svc, vaultRepo
}

func seedRenameConnection(t *testing.T, svc *usecase.VaultService) *domain.Connection {
	t.Helper()
	conn := &domain.Connection{ID: "conn-rename", Name: "Desk", Host: "10.0.0.9", Port: 3389, Protocol: "rdp"}
	saved, err := svc.SaveConnection(context.Background(), conn, map[string]string{
		"username": "admin",
		"password": "hunter2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved.PluginFields["password"], "secret:") {
		t.Fatalf("expected the secret to be stored by reference, got %q", saved.PluginFields["password"])
	}
	return saved
}

// A save that omits a field must keep what is on record. The UI never receives a stored secret, so
// it cannot send one back: every partial save - the tree's inline rename above all - arrives without
// it. Building the record from the payload alone dropped the reference and orphaned the secret.
func TestSaveConnectionKeepsAStoredSecretThePayloadOmits(t *testing.T) {
	svc, vaultRepo := newRenameHarness(t)
	seeded := seedRenameConnection(t, svc)
	storedRef := seeded.PluginFields["password"]

	renamed, err := svc.SaveConnection(context.Background(), &domain.Connection{
		ID: seeded.ID, Name: "Desk (office)", Host: seeded.Host, Port: seeded.Port, Protocol: "rdp",
	}, map[string]string{"username": "admin"})
	if err != nil {
		t.Fatalf("rename must not fail on a payload that omits a stored secret: %v", err)
	}
	if renamed.Name != "Desk (office)" {
		t.Fatalf("name = %q, want the renamed one", renamed.Name)
	}
	if renamed.PluginFields["password"] != storedRef {
		t.Fatalf("password ref = %q, want %q; a payload without the field must not clear it", renamed.PluginFields["password"], storedRef)
	}

	data, err := vaultRepo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if string(data.PluginSecrets[storedRef]) != "hunter2" {
		t.Fatal("the secret itself must survive a rename")
	}
}

// The one instruction an empty value carries is "clear it", and only the user can give it. Keeping
// that working is what stops the merge above from turning into "a secret can never be removed".
func TestSaveConnectionStillClearsASecretSentEmpty(t *testing.T) {
	svc, vaultRepo := newRenameHarness(t)
	seeded := seedRenameConnection(t, svc)

	_, err := svc.SaveConnection(context.Background(), &domain.Connection{
		ID: seeded.ID, Name: seeded.Name, Host: seeded.Host, Port: seeded.Port, Protocol: "rdp",
	}, map[string]string{"username": "admin", "password": ""})
	if err == nil {
		t.Fatal("expected a required field sent empty to be refused")
	}

	data, err := vaultRepo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.PluginSecrets) != 1 {
		t.Fatalf("a refused save must not touch the vault, got %d secrets", len(data.PluginSecrets))
	}
}

// The other half of the same contract, on the wire this time: the DTO must not carry an empty value
// for a stored secret, because the UI hands the record it was given straight back on a rename and an
// empty value there means "clear it". storedSecretFields is what tells the editor the secret exists.
func TestConnectionDTOOmitsStoredSecretsEntirely(t *testing.T) {
	dto := wails.ConnectionToDTO(domain.Connection{
		ID: "c1", Name: "Desk", Host: "h", Port: 3389, Protocol: "rdp",
		PluginFields: map[string]string{"username": "admin", "password": "secret:c1.password"},
	})
	if _, present := dto.PluginFields["password"]; present {
		t.Fatalf("pluginFields must not carry the secret field at all, got %q", dto.PluginFields["password"])
	}
	if dto.PluginFields["username"] != "admin" {
		t.Fatalf("username = %q, want admin", dto.PluginFields["username"])
	}
	if len(dto.StoredSecretFields) != 1 || dto.StoredSecretFields[0] != "password" {
		t.Fatalf("storedSecretFields = %v, want [password]", dto.StoredSecretFields)
	}
}
