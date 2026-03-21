package persistence

import (
	"context"
	"testing"

	"ssh-client/internal/domain"
)

type memVault struct {
	data *domain.VaultData
}

func (m *memVault) Unlock(context.Context, string) error { return nil }
func (m *memVault) Lock()                                {}
func (m *memVault) IsUnlocked() bool                     { return true }
func (m *memVault) GetData() (*domain.VaultData, error)  { return m.data, nil }
func (m *memVault) SaveData(_ context.Context, d *domain.VaultData) error {
	m.data = d
	return nil
}

func TestDeleteFolder_RemovesConnectionsAndChildFolders(t *testing.T) {
	d := domain.NewVaultData()
	d.Folders = []domain.ConnectionFolder{
		{ID: "a", Name: "A", ParentID: ""},
		{ID: "b", Name: "B", ParentID: "a"},
	}
	d.Connections = []domain.Connection{
		{ID: "c1", Name: "x", FolderID: "a", Host: "h1", Port: 22},
		{ID: "c2", Name: "y", FolderID: "b", Host: "h2", Port: 22},
		{ID: "c3", Name: "z", FolderID: "", Host: "h3", Port: 22},
	}
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	if err := r.DeleteFolder(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	out, err := v.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Connections) != 1 || out.Connections[0].ID != "c3" {
		t.Fatalf("connections: got %+v", out.Connections)
	}
	if len(out.Folders) != 0 {
		t.Fatalf("folders: got %+v", out.Folders)
	}
}
