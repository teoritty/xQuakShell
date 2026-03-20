package unit

import (
	"context"
	"errors"
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/persistence"
)

func setupPasswordRepo(t *testing.T) (*persistence.PasswordRepo, context.Context) {
	t.Helper()
	tmpDir := t.TempDir()
	vaultRepo := persistence.NewVaultRepo(tmpDir)
	ctx := context.Background()
	if err := vaultRepo.Unlock(ctx, "test-pass"); err != nil {
		t.Fatalf("unlock vault: %v", err)
	}
	passwordRepo := persistence.NewPasswordRepo(vaultRepo)
	return passwordRepo, ctx
}

func TestPasswordImportAndGet(t *testing.T) {
	repo, ctx := setupPasswordRepo(t)

	password := []byte("secret-password-123")
	label := "Test Password"
	id, err := repo.Import(ctx, password, label)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if id == "" {
		t.Fatal("import returned empty ID")
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(password) {
		t.Errorf("get: got %q, want %q", string(got), string(password))
	}
}

func TestPasswordDelete(t *testing.T) {
	repo, ctx := setupPasswordRepo(t)

	password := []byte("to-delete")
	id, err := repo.Import(ctx, password, "Delete Me")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	err = repo.Delete(ctx, id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = repo.Get(ctx, id)
	if err == nil {
		t.Fatal("get after delete: expected error, got nil")
	}
	if !errors.Is(err, domain.ErrPasswordNotFound) {
		t.Errorf("get after delete: expected ErrPasswordNotFound, got %v", err)
	}
}

func TestPasswordList(t *testing.T) {
	repo, ctx := setupPasswordRepo(t)

	_, err := repo.Import(ctx, []byte("pass1"), "Label One")
	if err != nil {
		t.Fatalf("import first: %v", err)
	}
	_, err = repo.Import(ctx, []byte("pass2"), "Label Two")
	if err != nil {
		t.Fatalf("import second: %v", err)
	}

	entries, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("list: got %d entries, want 2", len(entries))
	}
}
