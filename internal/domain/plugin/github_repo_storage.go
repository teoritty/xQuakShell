package plugin

import (
	"context"
	"time"
)

// GitHubRepositoryStorage defines the interface for storing GitHub repository registrations.
type GitHubRepositoryStorage interface {
	List(ctx context.Context) ([]GitHubRepository, error)
	Add(ctx context.Context, repo GitHubRepository) error
	Remove(ctx context.Context, repoURL string) error
	Get(ctx context.Context, repoURL string) (*GitHubRepository, error)
	UpdateFetchedAt(ctx context.Context, repoURL string, fetchedAt time.Time) error
	SetTrusted(ctx context.Context, repoURL string, trusted bool) error
}
