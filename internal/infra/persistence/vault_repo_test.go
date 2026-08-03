package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"xquakshell/internal/domain"
)

func TestVaultRepo_UnlockMissingVaultReturnsErrVaultNotFound(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)

	err := repo.Unlock(context.Background(), "test-pass")
	if !errors.Is(err, domain.ErrVaultNotFound) {
		t.Fatalf("expected ErrVaultNotFound, got %v", err)
	}
	if repo.IsUnlocked() {
		t.Error("repo must stay locked after a failed unlock")
	}
	// The whole point of the strict unlock: a typo must not become the master password.
	if _, statErr := os.Stat(filepath.Join(dir, "vault.age")); !os.IsNotExist(statErr) {
		t.Error("unlock must not create vault.age")
	}
}

func TestVaultRepo_ExistsFalseBeforeCreateTrueAfter(t *testing.T) {
	repo := NewVaultRepo(t.TempDir())

	if repo.Exists() {
		t.Error("expected Exists false before Create")
	}
	if err := repo.Create(context.Background(), "test-pass"); err != nil {
		t.Fatal(err)
	}
	if !repo.Exists() {
		t.Error("expected Exists true after Create")
	}
}

func TestVaultRepo_CreateWritesFileSynchronously(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)

	if err := repo.Create(context.Background(), "test-pass"); err != nil {
		t.Fatal(err)
	}

	// Deliberately no sleep: this fails if Create ever routes through the
	// debounced flush instead of writing straight to disk.
	if _, err := os.Stat(filepath.Join(dir, "vault.age")); err != nil {
		t.Fatalf("vault.age must exist the moment Create returns: %v", err)
	}
}

func TestVaultRepo_CreateLeavesVaultUnlockedWithDefaults(t *testing.T) {
	repo := NewVaultRepo(t.TempDir())

	if err := repo.Create(context.Background(), "test-pass"); err != nil {
		t.Fatal(err)
	}
	if !repo.IsUnlocked() {
		t.Fatal("expected the vault to be unlocked right after Create")
	}

	data, err := repo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if data.Settings == nil {
		t.Error("expected default settings to be populated on create")
	}
	if data.Version != domain.CurrentVaultVersion {
		t.Errorf("expected version %d, got %d", domain.CurrentVaultVersion, data.Version)
	}
}

func TestVaultRepo_CreateRejectsShortPassword(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)

	short := "1234567" // 7 runes, one below domain.MinMasterPasswordLength
	err := repo.Create(context.Background(), short)
	if !errors.Is(err, domain.ErrMasterPasswordTooShort) {
		t.Fatalf("expected ErrMasterPasswordTooShort, got %v", err)
	}
	if repo.IsUnlocked() {
		t.Error("a rejected password must not unlock the repo")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "vault.age")); !os.IsNotExist(statErr) {
		t.Error("a rejected password must not touch the disk")
	}
}

func TestVaultRepo_CreateOnExistingVaultDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)
	if err := repo.Create(context.Background(), "test-pass"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "vault.age")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Create(context.Background(), "another-pass"); !errors.Is(err, domain.ErrVaultAlreadyExists) {
		t.Fatalf("expected ErrVaultAlreadyExists, got %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a rejected Create must leave the existing ciphertext untouched")
	}
}

func TestVaultRepo_CreateLockUnlockRoundtrip(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)
	ctx := context.Background()

	if err := repo.Create(ctx, "test-pass"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateData(ctx, func(data *domain.VaultData) error {
		data.Connections = append(data.Connections, domain.Connection{
			ID: "c1", Name: "kept", Host: "host", Port: 22,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	repo.Lock() // flushes pending changes

	if err := repo.Unlock(ctx, "wrong-pass"); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("expected ErrVaultDecryptFailed, got %v", err)
	}
	if err := repo.Unlock(ctx, "test-pass"); err != nil {
		t.Fatalf("unlock with the creating password: %v", err)
	}

	data, err := repo.GetData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Connections) != 1 || data.Connections[0].Name != "kept" {
		t.Fatalf("roundtrip lost data: %+v", data.Connections)
	}
}

func TestVaultRepo_ConcurrentCreateOnlyOneSucceeds(t *testing.T) {
	repo := NewVaultRepo(t.TempDir())

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = repo.Create(context.Background(), "test-pass")
		}()
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrVaultAlreadyExists):
		default:
			t.Fatalf("unexpected error from a concurrent Create: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("expected exactly one successful Create, got %d", succeeded)
	}
}

func TestVaultRepo_GetDataReturnsSnapshot(t *testing.T) {
	dir := t.TempDir()
	repo := NewVaultRepo(dir)
	if err := repo.Create(context.Background(), "test-pass"); err != nil {
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
	if err := repo.Create(context.Background(), "test-pass"); err != nil {
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
