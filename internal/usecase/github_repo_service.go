package usecase

import (
	"context"
	"fmt"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// GitHubRepositoryService handles GitHub repository registration and management.
type GitHubRepositoryService struct {
	storage domainplugin.GitHubRepositoryStorage
	cache   domainplugin.GitHubCache
}

// NewGitHubRepositoryService creates a new repository service.
func NewGitHubRepositoryService(
	storage domainplugin.GitHubRepositoryStorage,
	cache domainplugin.GitHubCache,
) *GitHubRepositoryService {
	return &GitHubRepositoryService{
		storage: storage,
		cache:   cache,
	}
}

// ListRepositories returns all registered GitHub repositories.
func (s *GitHubRepositoryService) ListRepositories(ctx context.Context) ([]domainplugin.GitHubRepository, error) {
	return s.storage.List(ctx)
}

// GetRepository returns a registered repository by URL.
func (s *GitHubRepositoryService) GetRepository(ctx context.Context, repoURL string) (*domainplugin.GitHubRepository, error) {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return nil, err
	}
	return s.storage.Get(ctx, normalizedURL)
}

// AddRepository registers a new GitHub repository.
func (s *GitHubRepositoryService) AddRepository(ctx context.Context, repoURL string, trusted bool) error {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return err
	}

	if existing, err := s.storage.Get(ctx, normalizedURL); err == nil && existing != nil {
		return fmt.Errorf("repository already registered: %s", normalizedURL)
	}

	owner, repo, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return err
	}

	repository := domainplugin.GitHubRepository{
		URL:         normalizedURL,
		Owner:       owner,
		Repo:        repo,
		DisplayName: owner + "/" + repo,
		AddedAt:     time.Now(),
		Trusted:     trusted,
	}
	if err := repository.Validate(); err != nil {
		return err
	}

	return s.storage.Add(ctx, repository)
}

// RemoveRepository unregisters a GitHub repository.
func (s *GitHubRepositoryService) RemoveRepository(ctx context.Context, repoURL string) error {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return err
	}
	_ = s.cache.Delete(ctx, "metadata:"+normalizedURL)
	return s.storage.Remove(ctx, normalizedURL)
}

// SetRepositoryTrust marks a repository as trusted or untrusted.
func (s *GitHubRepositoryService) SetRepositoryTrust(ctx context.Context, repoURL string, trusted bool) error {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return err
	}
	return s.storage.SetTrusted(ctx, normalizedURL, trusted)
}
