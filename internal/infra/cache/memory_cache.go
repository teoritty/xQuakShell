package cache

import (
	"context"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// MemoryCache implements an in-memory cache with TTL.
type MemoryCache struct {
	mu         sync.RWMutex
	items      map[string]*item
	defaultTTL time.Duration
}

type item struct {
	value     interface{}
	expiresAt time.Time
}

// NewMemoryCache creates a new in-memory cache.
func NewMemoryCache(defaultTTL time.Duration) *MemoryCache {
	c := &MemoryCache{
		items:      make(map[string]*item),
		defaultTTL: defaultTTL,
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a value from cache.
func (c *MemoryCache) Get(_ context.Context, key string) (interface{}, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.items[key]
	if !found {
		return nil, false, nil
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false, nil
	}
	return entry.value, true, nil
}

// Set stores a value with default TTL.
func (c *MemoryCache) Set(ctx context.Context, key string, value interface{}) error {
	return c.SetWithTTL(ctx, key, value, c.defaultTTL)
}

// SetWithTTL stores a value with custom TTL.
func (c *MemoryCache) SetWithTTL(_ context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &item{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Delete removes a value from cache.
func (c *MemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	return nil
}

// Clear removes all items from cache.
func (c *MemoryCache) Clear(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*item)
	return nil
}

func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.cleanup()
	}
}

func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.items {
		if now.After(entry.expiresAt) {
			delete(c.items, key)
		}
	}
}

var _ domainplugin.GitHubCache = (*MemoryCache)(nil)
