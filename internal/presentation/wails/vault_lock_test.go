package wails

import (
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// countingVault records that Lock reached the repository.
type countingVault struct {
	domain.VaultRepository
	locks int
}

func (v *countingVault) Lock() { v.locks++ }

// recordingPassphraseCache stands in for the SSH key passphrase cache, whose clearing is the part
// of the lock sequence that has no other observer: the keys it holds unlock private keys, and they
// live in memory until something empties them.
type recordingPassphraseCache struct {
	cleared int
}

func (c *recordingPassphraseCache) Get(string) (string, bool)                { return "", false }
func (c *recordingPassphraseCache) Set(string, string)                       {}
func (c *recordingPassphraseCache) SetWithTTL(string, string, time.Duration) {}
func (c *recordingPassphraseCache) Forget(string)                            {}
func (c *recordingPassphraseCache) Clear()                                   { c.cleared++ }

func lockableAPI(t *testing.T) (*AppAPI, *countingVault, *recordingPassphraseCache) {
	t.Helper()
	cache := &recordingPassphraseCache{}
	vault := &countingVault{}
	// ctx stays nil so the Wails event emit is skipped; every other step of the sequence runs.
	return &AppAPI{
		vaultRepo: vault,
		sessions:  usecase.NewSessionManager(usecase.SessionManagerConfig{PassphraseCache: cache}),
	}, vault, cache
}

// A vault gets locked two ways - the user asking, and the idle timer firing - and those were
// separate copies of the same sequence. The copy easiest to forget when a step is added is the
// timer's, and the timer is the one that runs when nobody is watching: an idle machine that locked
// its vault but kept the key passphrases in memory is the exact state the lockout exists to prevent.
//
// Both entry points are asserted against the same expectations on purpose. A test that covered only
// LockVault would pass for a build where the idle path had drifted.
func TestBothLockPathsRunTheWholeSequence(t *testing.T) {
	for _, tc := range []struct {
		name string
		lock func(*AppAPI)
	}{
		{"the user locks the vault", (*AppAPI).LockVault},
		{"the idle timer fires", (*AppAPI).onLockoutTriggered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, vault, cache := lockableAPI(t)
			tc.lock(api)

			if vault.locks != 1 {
				t.Errorf("vault Lock called %d times, want exactly 1", vault.locks)
			}
			if cache.cleared != 1 {
				t.Errorf("passphrase cache cleared %d times, want exactly 1; the key passphrases "+
					"outlive the lock on this path", cache.cleared)
			}
		})
	}
}

// Locking twice must stay safe: the idle timer can fire on a vault the user has just locked by hand.
func TestLockingTwiceIsHarmless(t *testing.T) {
	api, vault, cache := lockableAPI(t)
	api.LockVault()
	api.onLockoutTriggered()

	if vault.locks != 2 || cache.cleared != 2 {
		t.Errorf("second lock: locks=%d cleared=%d, want 2 and 2", vault.locks, cache.cleared)
	}
}
