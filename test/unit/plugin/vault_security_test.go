package plugin_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

type vaultConnRepo struct {
	conn *domain.Connection
}

func (r vaultConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return nil, nil
}
func (r vaultConnRepo) SaveFolder(context.Context, *domain.ConnectionFolder) error { return nil }
func (r vaultConnRepo) DeleteFolder(context.Context, string) error                 { return nil }
func (r vaultConnRepo) GetAllConnections(context.Context) ([]domain.Connection, error) {
	return nil, nil
}
func (r vaultConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}
func (r vaultConnRepo) GetByID(context.Context, string) (*domain.Connection, error) {
	return r.conn, nil
}
func (r vaultConnRepo) Save(context.Context, *domain.Connection) error { return nil }
func (r vaultConnRepo) Delete(context.Context, string) error           { return nil }
func (r vaultConnRepo) MoveToFolder(context.Context, []string, string) error {
	return nil
}
func (r vaultConnRepo) MoveFolder(context.Context, string, string) error { return nil }
func (r vaultConnRepo) ReorderConnections(context.Context, []string, string) error {
	return nil
}
func (r vaultConnRepo) ReorderFolders(context.Context, []string, string) error { return nil }

type vaultPasswordRepo struct{}

func (vaultPasswordRepo) Import(context.Context, []byte, string) (string, error) { return "", nil }
func (vaultPasswordRepo) Get(context.Context, string) ([]byte, error) {
	return []byte("secret"), nil
}
func (vaultPasswordRepo) Delete(context.Context, string) error { return nil }
func (vaultPasswordRepo) List(context.Context) ([]domain.PasswordBlob, error) {
	return nil, nil
}

type vaultIdentRepo struct{}

func (vaultIdentRepo) GetAll(context.Context) ([]domain.SSHIdentity, error) { return nil, nil }
func (vaultIdentRepo) GetKeyBlob(context.Context, string) ([]byte, error) {
	return []byte("pem"), nil
}
func (vaultIdentRepo) Import(context.Context, []byte, string) (*domain.SSHIdentity, error) {
	return nil, nil
}
func (vaultIdentRepo) Delete(context.Context, string) error { return nil }

type vaultSettingsReader struct {
	granted bool
}

func (v vaultSettingsReader) PluginSettings() (domain.PluginSettings, error) {
	settings := domain.PluginSettings{}
	if v.granted {
		settings.SecretAccessGranted = map[string]bool{"com.test.vault": true}
	}
	return settings, nil
}

type vaultAuthorizer struct {
	owns map[string]bool
}

func (a vaultAuthorizer) PluginOwnsConnection(pluginID, connectionID string) bool {
	return a.owns[pluginID+":"+connectionID]
}

type recordingVaultAudit struct {
	events []domainplugin.VaultAccessEvent
	fail   bool
}

func (a *recordingVaultAudit) RecordVaultAccess(_ context.Context, event domainplugin.VaultAccessEvent) error {
	if a.fail {
		return domainplugin.ErrVaultAuditFailed
	}
	a.events = append(a.events, event)
	return nil
}

func TestVaultGetConnectionDeniedWithoutOwnership(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{ReadConnectionFields: []string{"host"}},
			},
		},
	})

	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: &domain.Connection{ID: "c1", Host: "h"}},
		vaultPasswordRepo{},
		vaultIdentRepo{},
		vaultSettingsReader{},
		nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{}})

	params, _ := json.Marshal(map[string]string{"connectionId": "c1"})
	_, err := inbound.GetConnection(context.Background(), "com.test.vault", params)
	if err == nil {
		t.Fatal("expected IDOR denial")
	}
}

func TestVaultGetConnectionAllowedWithOwnership(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{ReadConnectionFields: []string{"host"}},
			},
		},
	})

	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: &domain.Connection{ID: "c1", Host: "h"}},
		vaultPasswordRepo{},
		vaultIdentRepo{},
		vaultSettingsReader{},
		nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})
	audit := &recordingVaultAudit{}
	inbound.SetAuditLogger(audit)

	params, _ := json.Marshal(map[string]string{"connectionId": "c1"})
	raw, err := inbound.GetConnection(context.Background(), "com.test.vault", params)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["host"] != "h" {
		t.Fatalf("unexpected payload %v", out)
	}
	if len(audit.events) != 1 || audit.events[0].Method != domainplugin.VaultAccessGetConnection {
		t.Fatalf("expected vault access audit, got %+v", audit.events)
	}
}

func TestVaultGetSecretDeniedWithoutGrant(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
			},
		},
	})

	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: &domain.Connection{
			ID: "c1",
			Users: []domain.ConnectionUser{{
				ID: "u1", Username: "u", Auth: domain.AuthMethodPassword,
				PassAuth: &domain.PasswordAuthConfig{PasswordID: "p1"},
			}},
			DefaultUserID: "u1",
		}},
		vaultPasswordRepo{},
		vaultIdentRepo{},
		vaultSettingsReader{granted: false},
		nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})

	params, _ := json.Marshal(map[string]string{"connectionId": "c1", "field": "password"})
	_, err := inbound.GetSecret(context.Background(), "com.test.vault", params)
	if err == nil {
		t.Fatal("expected secret grant denial")
	}
}

type memoryPassphraseCache struct {
	values map[string]string
}

func (c memoryPassphraseCache) Get(identityID string) (string, bool) {
	if c.values == nil {
		return "", false
	}
	v, ok := c.values[identityID]
	return v, ok
}

func (c memoryPassphraseCache) Set(identityID, passphrase string) {
	if c.values == nil {
		c.values = make(map[string]string)
	}
	c.values[identityID] = passphrase
}

func (c memoryPassphraseCache) Clear() { c.values = nil }

type passphraseIdentRepo struct {
	idents []domain.SSHIdentity
}

func (r passphraseIdentRepo) GetAll(context.Context) ([]domain.SSHIdentity, error) {
	return r.idents, nil
}
func (passphraseIdentRepo) GetKeyBlob(context.Context, string) ([]byte, error) {
	return []byte("pem"), nil
}
func (passphraseIdentRepo) Import(context.Context, []byte, string) (*domain.SSHIdentity, error) {
	return nil, nil
}
func (passphraseIdentRepo) Delete(context.Context, string) error { return nil }

func TestVaultPassphraseRequiresCache(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{"passphrase"}},
			},
		},
	})

	conn := &domain.Connection{
		ID: "c1",
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "u", Auth: domain.AuthMethodKey,
			KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"id1"}},
		}},
		DefaultUserID: "u1",
	}

	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: conn},
		vaultPasswordRepo{},
		passphraseIdentRepo{idents: []domain.SSHIdentity{{ID: "id1", Encrypted: true}}},
		vaultSettingsReader{granted: true},
		memoryPassphraseCache{values: map[string]string{"id1": "secret-pass"}},
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})
	inbound.SetAuditLogger(&recordingVaultAudit{})

	params, _ := json.Marshal(map[string]string{"connectionId": "c1", "field": "passphrase"})
	raw, err := inbound.GetSecret(context.Background(), "com.test.vault", params)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(out["valueBase64"])
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "secret-pass" {
		t.Fatalf("unexpected passphrase %q", decoded)
	}
}

func TestVaultGetSecretFailsWhenAuditWriteFails(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
			},
		},
	})

	conn := &domain.Connection{
		ID: "c1",
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "u", Auth: domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{PasswordID: "p1"},
		}},
		DefaultUserID: "u1",
	}

	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: conn},
		vaultPasswordRepo{},
		vaultIdentRepo{},
		vaultSettingsReader{granted: true},
		nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})
	inbound.SetAuditLogger(&recordingVaultAudit{fail: true})

	params, _ := json.Marshal(map[string]string{"connectionId": "c1", "field": "password"})
	_, err := inbound.GetSecret(context.Background(), "com.test.vault", params)
	if err == nil {
		t.Fatal("expected audit failure to block secret release")
	}
}

func TestVaultGetSecretAuditOmitsSecretValue(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
			},
		},
	})

	conn := &domain.Connection{
		ID: "c1",
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "u", Auth: domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{PasswordID: "p1"},
		}},
		DefaultUserID: "u1",
	}

	audit := &recordingVaultAudit{}
	inbound := usecase.NewPluginVaultInbound(
		registry,
		vaultConnRepo{conn: conn},
		vaultPasswordRepo{},
		vaultIdentRepo{},
		vaultSettingsReader{granted: true},
		nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c1": true}})
	inbound.SetAuditLogger(audit)

	params, _ := json.Marshal(map[string]string{"connectionId": "c1", "field": "password"})
	if _, err := inbound.GetSecret(context.Background(), "com.test.vault", params); err != nil {
		t.Fatal(err)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected one audit event, got %d", len(audit.events))
	}
	ev := audit.events[0]
	if ev.Method != domainplugin.VaultAccessGetSecret || ev.Field != "password" || ev.ConnectionID != "c1" {
		t.Fatalf("unexpected audit event %+v", ev)
	}
}
