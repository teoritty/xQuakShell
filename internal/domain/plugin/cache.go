package plugin

import (
	"context"
	"time"
)

// GitHubCache defines the interface for caching GitHub API responses.
type GitHubCache interface {
	Get(ctx context.Context, key string) (interface{}, bool, error)
	Set(ctx context.Context, key string, value interface{}) error
	SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// DefaultCacheTTL is the default GitHub API response cache duration.
const DefaultCacheTTL = 3 * time.Hour
