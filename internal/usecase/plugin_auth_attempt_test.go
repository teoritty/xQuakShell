package usecase

import (
	"errors"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestPluginAuthAttemptRegistry_AuthorizeChecksConnectionID(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	attempt, err := r.Begin("plugin-a", "conn-1", "otp")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Authorize("plugin-a", attempt.ID, "otp", "conn-1"); err != nil {
		t.Fatalf("authorize same connection: %v", err)
	}
	if err = r.Authorize("plugin-a", attempt.ID, "otp", "conn-2"); err == nil {
		t.Fatal("expected connection mismatch error")
	}
	if !errors.Is(err, domainplugin.ErrAuthAttemptNotBound) {
		t.Fatalf("expected ErrAuthAttemptNotBound, got %v", err)
	}
}

func TestPluginAuthAttemptRegistry_LookupReturnsAttempt(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	attempt, err := r.Begin("plugin-a", "conn-1", "otp")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.Lookup(attempt.ID)
	if !ok || got.ConnectionID != "conn-1" {
		t.Fatalf("lookup = %+v ok=%v", got, ok)
	}
}
