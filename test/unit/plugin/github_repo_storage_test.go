package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infrapersistence "ssh-client/internal/infra/persistence"
)

func TestFileGitHubRepositoryStorage_AddListRemove(t *testing.T) {
	dir := t.TempDir()
	storage, err := infrapersistence.NewFileGitHubRepositoryStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	repo := domainplugin.GitHubRepository{
		URL:         "https://github.com/user/repo",
		Owner:       "user",
		Repo:        "repo",
		DisplayName: "user/repo",
		AddedAt:     time.Now(),
		Trusted:     true,
	}
	if err := storage.Add(ctx, repo); err != nil {
		t.Fatal(err)
	}

	list, err := storage.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v err=%v", list, err)
	}

	got, err := storage.Get(ctx, repo.URL)
	if err != nil || got == nil || !got.Trusted {
		t.Fatalf("get: %v err=%v", got, err)
	}

	if err := storage.Remove(ctx, repo.URL); err != nil {
		t.Fatal(err)
	}
	list, err = storage.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("expected empty list, got %v err=%v", list, err)
	}

	if _, err := os.Stat(filepath.Join(dir, "github-repos.json")); err != nil {
		t.Fatalf("expected persisted file: %v", err)
	}
}
