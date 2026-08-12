package ssh

import (
	"sync"
	"time"

	"xquakshell/internal/domain"
)

// PassphraseCache stores passphrases for encrypted keys in memory.
// It is safe for concurrent use.
//
// Entries can carry an expiry so an unlocked application left unattended stops being able to
// authenticate on its own. An entry with no expiry lives until the vault locks, which is what
// this cache did before expiry existed and remains the default policy.
type PassphraseCache struct {
	mu    sync.RWMutex
	cache map[string]cacheEntry
	now   func() time.Time
}

type cacheEntry struct {
	passphrase string
	// expiresAt is the zero time for an entry that lives until the vault locks.
	expiresAt time.Time
}

var _ domain.PassphraseCache = (*PassphraseCache)(nil)

// NewPassphraseCache creates a new empty passphrase cache.
func NewPassphraseCache() *PassphraseCache {
	return &PassphraseCache{cache: make(map[string]cacheEntry), now: time.Now}
}

// Get retrieves a cached passphrase for the given identity ID.
//
// An expired entry is removed on the way out rather than merely reported as absent: leaving it in
// the map would keep the plaintext passphrase alive in memory for as long as the vault stays
// unlocked, which is the exact lifetime the expiry was set to cut short.
func (c *PassphraseCache) Get(identityID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[identityID]
	if !ok {
		return "", false
	}
	if !entry.expiresAt.IsZero() && !c.now().Before(entry.expiresAt) {
		c.wipe(identityID)
		return "", false
	}
	return entry.passphrase, true
}

// Set stores a passphrase that lives until the vault locks.
func (c *PassphraseCache) Set(identityID, passphrase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[identityID] = cacheEntry{passphrase: passphrase}
}

// SetWithTTL stores a passphrase that expires after ttl. A ttl of zero or less stores nothing at
// all, so a "never cache" policy needs no separate call path and cannot be expressed by accident
// as "cache forever".
func (c *PassphraseCache) SetWithTTL(identityID, passphrase string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ttl <= 0 {
		c.wipe(identityID)
		return
	}
	c.cache[identityID] = cacheEntry{passphrase: passphrase, expiresAt: c.now().Add(ttl)}
}

// Forget removes one identity's passphrase. It is called when a key's passphrase changes, so the
// superseded one cannot be tried against the re-wrapped blob and reported as a wrong passphrase.
func (c *PassphraseCache) Forget(identityID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wipe(identityID)
}

// Clear removes all cached passphrases from memory.
func (c *PassphraseCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.cache {
		c.wipe(k)
	}
}

// wipe overwrites an entry's passphrase before dropping it. The caller must hold the write lock.
//
// Overwriting a Go string does not scrub the original bytes — the header is replaced, the backing
// array is left for the GC. It is kept because it costs nothing and removes the reference that
// would otherwise keep that array reachable through the map until the next rehash.
func (c *PassphraseCache) wipe(identityID string) {
	if _, ok := c.cache[identityID]; ok {
		c.cache[identityID] = cacheEntry{}
	}
	delete(c.cache, identityID)
}
