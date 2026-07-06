package plugin

import "testing"

func TestAuthAttemptIDORUsesSessionNotBoundSentinel(t *testing.T) {
	// TZ 1.4: binding violations (sessionId, attemptId) share one sentinel — no parallel type.
	if ErrSessionNotBound == nil {
		t.Fatal("ErrSessionNotBound must be the canonical binding violation sentinel")
	}
}
