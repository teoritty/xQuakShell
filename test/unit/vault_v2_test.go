package unit

import (
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/vault"
)

func TestMigrateV1ToV2(t *testing.T) {
	data := &domain.VaultData{
		Version:     1,
		Folders:     []domain.ConnectionFolder{{ID: "f1", Name: "Test Folder", ParentID: "", Order: 0}},
		Connections: []domain.Connection{
			{
				ID:          "c1",
				FolderID:    "f1",
				Name:        "Test Connection",
				Host:        "example.com",
				Port:        22,
				User:        "admin",
				IdentityIDs: []string{"key1"},
			},
		},
		Identities:  map[string]domain.SSHIdentity{},
		KeyBlobs:    map[string]domain.IdentityBlob{},
		KnownHosts:  []string{},
		Passwords:   nil,
		VPNProfiles: nil,
		Settings:    nil,
	}

	vault.MigrateVaultData(data)

	if data.Version != domain.CurrentVaultVersion {
		t.Errorf("Version: got %d, want %d", data.Version, domain.CurrentVaultVersion)
	}
	if data.Passwords == nil {
		t.Fatal("Passwords should be initialized, got nil")
	}
	if data.VPNProfiles == nil {
		t.Fatal("VPNProfiles should be initialized, got nil")
	}
	if data.Settings == nil {
		t.Fatal("Settings should be initialized, got nil")
	}
	if len(data.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(data.Connections))
	}
	conn := &data.Connections[0]
	if len(conn.Users) != 1 {
		t.Fatalf("Connections[0].Users: got %d entries, want 1", len(conn.Users))
	}
	if conn.Users[0].Username != "admin" {
		t.Errorf("Connections[0].Users[0].Username: got %q, want 'admin'", conn.Users[0].Username)
	}
	if conn.DefaultUserID == "" {
		t.Error("Connections[0].DefaultUserID should be set, got empty")
	}
	if conn.Users[0].Auth != domain.AuthMethodKey {
		t.Errorf("Connections[0].Users[0].Auth: got %q, want %q", conn.Users[0].Auth, domain.AuthMethodKey)
	}
	if conn.Users[0].KeyAuth == nil {
		t.Fatal("Connections[0].Users[0].KeyAuth should be set, got nil")
	}
	if len(conn.Users[0].KeyAuth.IdentityIDs) != 1 || conn.Users[0].KeyAuth.IdentityIDs[0] != "key1" {
		t.Errorf("Connections[0].Users[0].KeyAuth.IdentityIDs: got %v, want [\"key1\"]", conn.Users[0].KeyAuth.IdentityIDs)
	}
}

func TestMigrateV2Noop(t *testing.T) {
	data := domain.NewVaultData()
	data.Folders = []domain.ConnectionFolder{{ID: "f1", Name: "Folder", ParentID: "", Order: 0}}
	data.Connections = []domain.Connection{
		{
			ID:            "c1",
			FolderID:      "f1",
			Name:          "Conn",
			Host:          "host.example.com",
			Port:          22,
			Users:         []domain.ConnectionUser{{ID: "u1", Username: "root", Auth: domain.AuthMethodKey, KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"key1"}}}},
			DefaultUserID: "u1",
		},
	}

	origVersion := data.Version
	origConnCount := len(data.Connections)
	origUserCount := len(data.Connections[0].Users)

	vault.MigrateVaultData(data)

	if data.Version != origVersion {
		t.Errorf("Version changed: got %d, want %d", data.Version, origVersion)
	}
	if len(data.Connections) != origConnCount {
		t.Errorf("Connections count changed: got %d, want %d", len(data.Connections), origConnCount)
	}
	if len(data.Connections[0].Users) != origUserCount {
		t.Errorf("Users count changed: got %d, want %d", len(data.Connections[0].Users), origUserCount)
	}
	if data.Connections[0].Users[0].Username != "root" {
		t.Errorf("Username changed: got %q, want 'root'", data.Connections[0].Users[0].Username)
	}
}

func TestNewVaultDataIsV2(t *testing.T) {
	data := domain.NewVaultData()

	if data.Version != domain.CurrentVaultVersion {
		t.Errorf("Version: got %d, want %d", data.Version, domain.CurrentVaultVersion)
	}
	if data.Passwords == nil {
		t.Error("Passwords should be initialized, got nil")
	}
	if data.VPNProfiles == nil {
		t.Error("VPNProfiles should be initialized, got nil")
	}
	if data.Settings == nil {
		t.Error("Settings should be initialized, got nil")
	}
}
