package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// FileGitHubRepositoryStorage stores repository registrations in a JSON file.
type FileGitHubRepositoryStorage struct {
	filePath string
	mu       sync.RWMutex
}

// NewFileGitHubRepositoryStorage creates storage backed by a JSON file.
func NewFileGitHubRepositoryStorage(dataDir string) (*FileGitHubRepositoryStorage, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &FileGitHubRepositoryStorage{
		filePath: filepath.Join(dataDir, "github-repos.json"),
	}, nil
}

type repoFile struct {
	Repositories []domainplugin.GitHubRepository `json:"repositories"`
}

// List returns all registered repositories.
func (s *FileGitHubRepositoryStorage) List(_ context.Context) ([]domainplugin.GitHubRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadReposLocked()
}

// Add registers a new repository.
func (s *FileGitHubRepositoryStorage) Add(_ context.Context, repo domainplugin.GitHubRepository) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repos, err := s.loadReposLocked()
	if err != nil {
		return err
	}
	for _, r := range repos {
		if r.URL == repo.URL {
			return fmt.Errorf("repository already exists: %s", repo.URL)
		}
	}
	repos = append(repos, repo)
	return s.saveReposLocked(repos)
}

// Remove unregisters a repository.
func (s *FileGitHubRepositoryStorage) Remove(_ context.Context, repoURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repos, err := s.loadReposLocked()
	if err != nil {
		return err
	}
	filtered := make([]domainplugin.GitHubRepository, 0, len(repos))
	for _, r := range repos {
		if r.URL != repoURL {
			filtered = append(filtered, r)
		}
	}
	return s.saveReposLocked(filtered)
}

// Get retrieves a specific repository.
func (s *FileGitHubRepositoryStorage) Get(_ context.Context, repoURL string) (*domainplugin.GitHubRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos, err := s.loadReposLocked()
	if err != nil {
		return nil, err
	}
	for i := range repos {
		if repos[i].URL == repoURL {
			copy := repos[i]
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("repository not found: %s", repoURL)
}

// UpdateFetchedAt updates the last fetch timestamp.
func (s *FileGitHubRepositoryStorage) UpdateFetchedAt(_ context.Context, repoURL string, fetchedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repos, err := s.loadReposLocked()
	if err != nil {
		return err
	}
	for i := range repos {
		if repos[i].URL == repoURL {
			repos[i].LastFetchedAt = &fetchedAt
			return s.saveReposLocked(repos)
		}
	}
	return fmt.Errorf("repository not found: %s", repoURL)
}

// SetTrusted marks a repository as trusted or untrusted.
func (s *FileGitHubRepositoryStorage) SetTrusted(_ context.Context, repoURL string, trusted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	repos, err := s.loadReposLocked()
	if err != nil {
		return err
	}
	for i := range repos {
		if repos[i].URL == repoURL {
			repos[i].Trusted = trusted
			return s.saveReposLocked(repos)
		}
	}
	return fmt.Errorf("repository not found: %s", repoURL)
}

func (s *FileGitHubRepositoryStorage) loadReposLocked() ([]domainplugin.GitHubRepository, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []domainplugin.GitHubRepository{}, nil
		}
		return nil, err
	}
	var file repoFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return file.Repositories, nil
}

func (s *FileGitHubRepositoryStorage) saveReposLocked(repos []domainplugin.GitHubRepository) error {
	file := repoFile{Repositories: repos}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0o600)
}

var _ domainplugin.GitHubRepositoryStorage = (*FileGitHubRepositoryStorage)(nil)

// EnsureGitHubReposFile creates an empty github-repos.json if missing.
func EnsureGitHubReposFile(dataDir string) error {
	path := filepath.Join(dataDir, "github-repos.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	file := repoFile{Repositories: []domainplugin.GitHubRepository{}}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
