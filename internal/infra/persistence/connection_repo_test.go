package persistence

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
)

type memVault struct {
	data *domain.VaultData
}

func (m *memVault) Exists() bool                                       { return true }
func (m *memVault) Create(context.Context, string) error               { return nil }
func (m *memVault) Unlock(context.Context, string) error               { return nil }
func (m *memVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (m *memVault) Lock()                                              {}
func (m *memVault) IsUnlocked() bool                                   { return true }
func (m *memVault) GetData() (*domain.VaultData, error)                { return domain.CloneVaultData(m.data), nil }
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

// A new folder belongs at the top of its level. Stored with the zero Order it arrives with, it
// landed wherever the tree's stable sort placed a tie - in practice straight after the first
// folder, because that one usually has order zero too. Appearing in the middle of a list for no
// reason the user can see is the worst of the three places it could go.
func TestANewFolderSortsAheadOfItsSiblings(t *testing.T) {
	d := domain.NewVaultData()
	d.Folders = []domain.ConnectionFolder{
		{ID: "a", Name: "A", ParentID: "", Order: 0},
		{ID: "b", Name: "B", ParentID: "", Order: 1},
		{ID: "c", Name: "C", ParentID: "", Order: 2},
	}
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	fresh := &domain.ConnectionFolder{Name: "New folder"}
	if err := r.SaveFolder(context.Background(), fresh); err != nil {
		t.Fatalf("save: %v", err)
	}

	for _, existing := range d.Folders {
		if existing.ID == fresh.ID {
			continue
		}
		if fresh.Order >= existing.Order {
			t.Fatalf("new folder order %d does not sort ahead of %q at %d",
				fresh.Order, existing.Name, existing.Order)
		}
	}
	if d.Folders[len(d.Folders)-1].Order != fresh.Order {
		t.Error("the stored folder did not keep the order that was assigned to it")
	}
}

// Creating several in a row must keep putting each one first, or the second click lands the folder
// behind the one the first click made and the rule holds only once.
func TestEachNewFolderGoesAheadOfTheLastOne(t *testing.T) {
	v := &memVault{data: domain.NewVaultData()}
	r := NewConnectionRepo(v)

	var previous int
	for i := range 3 {
		f := &domain.ConnectionFolder{Name: "New folder"}
		if err := r.SaveFolder(context.Background(), f); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
		if i > 0 && f.Order >= previous {
			t.Fatalf("folder %d took order %d, which does not sort ahead of the previous %d", i, f.Order, previous)
		}
		previous = f.Order
	}
}

// Ordering is per level. A new subfolder must sort against its own siblings, not against folders
// under a different parent that happen to hold lower numbers.
func TestOrderingIsScopedToTheParent(t *testing.T) {
	d := domain.NewVaultData()
	d.Folders = []domain.ConnectionFolder{
		{ID: "root", Name: "Root", ParentID: "", Order: -50},
		{ID: "child", Name: "Child", ParentID: "root", Order: 3},
	}
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	fresh := &domain.ConnectionFolder{Name: "New folder", ParentID: "root"}
	if err := r.SaveFolder(context.Background(), fresh); err != nil {
		t.Fatalf("save: %v", err)
	}

	if fresh.Order >= 3 {
		t.Errorf("order = %d, want less than the sibling's 3", fresh.Order)
	}
	// The unrelated root folder at -50 must not drag the new subfolder down past it; only siblings
	// count, and the lowest sibling order here is 3.
	if fresh.Order < -1 {
		t.Errorf("order = %d; a folder under a different parent was counted as a sibling", fresh.Order)
	}
}

// Updating an existing folder must leave its order alone, or every rename would jump the folder to
// the top of the list.
func TestSavingAnExistingFolderKeepsItsOrder(t *testing.T) {
	d := domain.NewVaultData()
	d.Folders = []domain.ConnectionFolder{
		{ID: "a", Name: "A", ParentID: "", Order: 0},
		{ID: "b", Name: "B", ParentID: "", Order: 1},
	}
	v := &memVault{data: d}
	r := NewConnectionRepo(v)

	renamed := &domain.ConnectionFolder{ID: "b", Name: "B renamed", ParentID: "", Order: 1}
	if err := r.SaveFolder(context.Background(), renamed); err != nil {
		t.Fatalf("save: %v", err)
	}
	if renamed.Order != 1 {
		t.Errorf("order = %d after a rename, want 1", renamed.Order)
	}
	if d.Folders[1].Name != "B renamed" || d.Folders[1].Order != 1 {
		t.Errorf("stored folder = %+v, want the rename at order 1", d.Folders[1])
	}
}
