package persistence

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"ssh-client/internal/domain"
)

func TestVaultRepo_GetDataReturnsSnapshot(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)
	if err := repo.Unlock(context.Background(), "test-pass"); err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateData(context.Background(), func(data *domain.VaultData) error {
		data.Connections = append(data.Connections, domain.Connection{
			ID:   "c1",
			Name: "original",
			Host: "host",
			Port: 22,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := repo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(snapshot.Connections))
	}
	snapshot.Connections[0].Name = "mutated"

	again, err := repo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if again.Connections[0].Name != "original" {
		t.Fatalf("snapshot mutation leaked into vault: got %q", again.Connections[0].Name)
	}
}

func TestVaultRepo_UpdateDataSerializesConcurrentMutations(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)
	if err := repo.Unlock(context.Background(), "test-pass"); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("conn-%d", i)
			_ = repo.UpdateData(context.Background(), func(data *domain.VaultData) error {
				data.Connections = append(data.Connections, domain.Connection{
					ID:   name,
					Name: name,
					Host: "host",
					Port: 22,
				})
				return nil
			})
		}()
	}
	wg.Wait()

	data, err := repo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Connections) != n {
		t.Fatalf("expected %d connections, got %d", n, len(data.Connections))
	}
	seen := make(map[string]struct{}, n)
	for _, c := range data.Connections {
		seen[c.ID] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("duplicate connection IDs: got %d unique of %d", len(seen), n)
	}
}

func TestVaultRepo_UpdateDataRejectsWhenLocked(t *testing.T) {
	repo := NewVaultRepo(filepath.Join(t.TempDir(), "vault"))
	err := repo.UpdateData(context.Background(), func(*domain.VaultData) error { return nil })
	if err != domain.ErrVaultLocked {
		t.Fatalf("expected ErrVaultLocked, got %v", err)
	}
}

func TestVaultRepo_GetDataRejectsWhenLocked(t *testing.T) {
	repo := NewVaultRepo(filepath.Join(t.TempDir(), "vault"))
	_, err := repo.GetData()
	if err != domain.ErrVaultLocked {
		t.Fatalf("expected ErrVaultLocked, got %v", err)
	}
}
