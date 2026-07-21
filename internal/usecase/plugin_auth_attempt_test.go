package usecase

import (
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
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
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("expected ErrSessionNotBound, got %v", err)
	}
}

func TestPluginAuthAttemptRegistry_AuthorizeIDOR(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	attempt, err := r.Begin("plugin-a", "conn-1", "otp")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		pluginID     string
		attemptID    string
		authMethodID string
		connectionID string
	}{
		{"unknown attempt", "plugin-a", "missing", "otp", ""},
		{"foreign plugin", "plugin-b", attempt.ID, "otp", ""},
		{"wrong auth method", "plugin-a", attempt.ID, "other", ""},
		{"wrong connection", "plugin-a", attempt.ID, "otp", "conn-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := r.Authorize(tc.pluginID, tc.attemptID, tc.authMethodID, tc.connectionID)
			if err == nil {
				t.Fatal("expected ErrSessionNotBound")
			}
			if !errors.Is(err, domainplugin.ErrSessionNotBound) {
				t.Fatalf("expected ErrSessionNotBound, got %v", err)
			}
		})
	}

	if err := r.Authorize("plugin-a", attempt.ID, "otp", "conn-1"); err != nil {
		t.Fatalf("valid ownership: %v", err)
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
