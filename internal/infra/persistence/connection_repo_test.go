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
func (m *memVault) GetData() (*domain.VaultData, error)  { return domain.CloneVaultData(m.data), nil }
func (m *memVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	return mutate(m.data)
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

func TestConnectionRepo_Save_BackfillsJumpHopIDs(t *testing.T) {
	d := domain.NewVaultData()
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	conn := &domain.Connection{
		Name:     "jump-test",
		FolderID: "",
		Host:     "target",
		Port:     22,
		JumpChain: domain.JumpChainConfig{
			Hops: []domain.JumpHop{
				{Host: "bastion1", Port: 22, Username: "user1", Auth: domain.AuthMethodKey},
				{Host: "bastion2", Port: 2222, Username: "user2", Auth: domain.AuthMethodPassword},
			},
		},
	}

	if err := r.Save(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if conn.JumpChain.Hops[0].ID == "" || conn.JumpChain.Hops[1].ID == "" {
		t.Fatal("expected hop IDs to be backfilled on Save")
	}
	if conn.JumpChain.Hops[0].ID == conn.JumpChain.Hops[1].ID {
		t.Fatal("expected distinct hop IDs")
	}

	saved, err := r.GetByID(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.JumpChain.Hops[0].ID != conn.JumpChain.Hops[0].ID {
		t.Fatalf("hop[0] id mismatch: saved %q conn %q", saved.JumpChain.Hops[0].ID, conn.JumpChain.Hops[0].ID)
	}
	if saved.JumpChain.Hops[1].ID != conn.JumpChain.Hops[1].ID {
		t.Fatalf("hop[1] id mismatch: saved %q conn %q", saved.JumpChain.Hops[1].ID, conn.JumpChain.Hops[1].ID)
	}
}

func TestConnectionRepo_Save_RejectsDuplicateJumpHopIDs(t *testing.T) {
	d := domain.NewVaultData()
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	conn := &domain.Connection{
		Name:     "jump-dup",
		FolderID: "",
		Host:     "target",
		Port:     22,
		JumpChain: domain.JumpChainConfig{
			Hops: []domain.JumpHop{
				{ID: "dup-id", Host: "bastion1", Port: 22},
				{ID: "dup-id", Host: "bastion2", Port: 2222},
			},
		},
	}

	err := r.Save(context.Background(), conn)
	if err == nil {
		t.Fatal("expected duplicate hop id save to fail")
	}
}

func TestConnectionRepo_Save_PreservesExistingJumpHopID(t *testing.T) {
	d := domain.NewVaultData()
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	existingID := "hop-persist-123"
	conn := &domain.Connection{
		Name:     "jump-test",
		FolderID: "",
		Host:     "target",
		Port:     22,
		JumpChain: domain.JumpChainConfig{
			Hops: []domain.JumpHop{
				{ID: existingID, Host: "bastion", Port: 22, Username: "user", Auth: domain.AuthMethodKey},
			},
		},
	}

	if err := r.Save(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if conn.JumpChain.Hops[0].ID != existingID {
		t.Fatalf("hop id changed on first save: got %q want %q", conn.JumpChain.Hops[0].ID, existingID)
	}

	conn.Host = "target-updated"
	if err := r.Save(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if conn.JumpChain.Hops[0].ID != existingID {
		t.Fatalf("hop id changed on second save: got %q want %q", conn.JumpChain.Hops[0].ID, existingID)
	}

	saved, err := r.GetByID(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.JumpChain.Hops[0].ID != existingID {
		t.Fatalf("saved hop id: got %q want %q", saved.JumpChain.Hops[0].ID, existingID)
	}
}
