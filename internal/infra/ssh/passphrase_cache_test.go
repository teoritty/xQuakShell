package ssh

import (
	"testing"
	"time"
)

// at builds a cache whose clock the test drives, so expiry is asserted without sleeping.
func at(start time.Time) (*PassphraseCache, *time.Time) {
	now := start
	c := NewPassphraseCache()
	c.now = func() time.Time { return now }
	return c, &now
}

func TestEntryWithoutTTLSurvivesUntilCleared(t *testing.T) {
	c, now := at(time.Unix(0, 0))
	c.Set("k1", "secret")

	*now = now.Add(24 * time.Hour)
	if got, ok := c.Get("k1"); !ok || got != "secret" {
		t.Errorf("Get after a day = (%q, %v), want (\"secret\", true); the default policy holds until the vault locks", got, ok)
	}

	c.Clear()
	if _, ok := c.Get("k1"); ok {
		t.Error("Get after Clear returned an entry; locking the vault must drop every passphrase")
	}
}

func TestEntryWithTTLStopsWorkingWhenItExpires(t *testing.T) {
	c, now := at(time.Unix(0, 0))
	c.SetWithTTL("k1", "secret", time.Minute)

	*now = now.Add(59 * time.Second)
	if _, ok := c.Get("k1"); !ok {
		t.Error("entry vanished before its TTL elapsed")
	}

	*now = now.Add(2 * time.Second)
	if _, ok := c.Get("k1"); ok {
		t.Error("entry survived past its TTL; an unattended unlocked app must stop authenticating on its own")
	}
}

func TestExpiryIsExclusiveAtTheBoundary(t *testing.T) {
	c, now := at(time.Unix(0, 0))
	c.SetWithTTL("k1", "secret", time.Minute)

	*now = now.Add(time.Minute)
	if _, ok := c.Get("k1"); ok {
		t.Error("entry still valid exactly at its expiry instant; the TTL must be an upper bound, not an inclusive one")
	}
}

func TestReadingAnExpiredEntryRemovesIt(t *testing.T) {
	c, now := at(time.Unix(0, 0))
	c.SetWithTTL("k1", "secret", time.Minute)
	*now = now.Add(2 * time.Minute)

	c.Get("k1")

	c.mu.RLock()
	_, present := c.cache["k1"]
	c.mu.RUnlock()
	if present {
		t.Error("expired entry is still in the map after a read; the plaintext must not outlive the TTL that was set to bound it")
	}
}

func TestZeroTTLStoresNothing(t *testing.T) {
	c, _ := at(time.Unix(0, 0))
	c.Set("k1", "old")
	c.SetWithTTL("k1", "secret", 0)

	if got, ok := c.Get("k1"); ok {
		t.Errorf("Get = (%q, %v), want no entry; a never-cache policy must not fall through to caching forever, and must drop what was there", got, ok)
	}
}

func TestForgetDropsOnlyTheNamedKey(t *testing.T) {
	c, _ := at(time.Unix(0, 0))
	c.Set("k1", "one")
	c.Set("k2", "two")

	c.Forget("k1")

	if _, ok := c.Get("k1"); ok {
		t.Error("Forget left the named entry in place; a superseded passphrase must not be retried against the re-wrapped key")
	}
	if got, ok := c.Get("k2"); !ok || got != "two" {
		t.Errorf("k2 = (%q, %v) after forgetting k1, want (\"two\", true)", got, ok)
	}
}

func TestForgetAndGetTolerateAMissingKey(t *testing.T) {
	c, _ := at(time.Unix(0, 0))
	c.Forget("never-stored")
	if _, ok := c.Get("never-stored"); ok {
		t.Error("Get invented an entry for a key that was never stored")
	}
}

func TestConcurrentUseDoesNotRace(t *testing.T) {
	c := NewPassphraseCache()
	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 200 {
				c.Set("k", "v")
				c.SetWithTTL("k", "v", time.Minute)
				c.Get("k")
				c.Forget("k")
				c.Clear()
			}
		}()
	}
	for range 8 {
		<-done
	}
}
