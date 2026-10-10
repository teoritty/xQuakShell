package agentforward

import (
	"fmt"
	"sync"
	"time"
)

// Grant names what one "allow for this session" approval covers: one session, one key, one
// destination host key. Anything that differs in any field is a new question for the user.
type Grant struct {
	SessionID   string
	IdentityID  string
	Destination string
}

// ApprovalWindow holds the approvals a user granted for a limited time.
//
// A grant needs a verified destination. Without one the user would be approving "whatever host
// asks next", which is the blanket permission the confirmation exists to prevent.
type ApprovalWindow struct {
	mu     sync.Mutex
	now    func() time.Time
	grants map[Grant]time.Time
}

// NewApprovalWindow returns an empty window reading time from now.
func NewApprovalWindow(now func() time.Time) *ApprovalWindow {
	return &ApprovalWindow{now: now, grants: map[Grant]time.Time{}}
}

// Allow records a grant lasting ttl.
func (w *ApprovalWindow) Allow(g Grant, ttl time.Duration) error {
	if g.SessionID == "" || g.IdentityID == "" || g.Destination == "" {
		return fmt.Errorf("%w: a window needs a session, a key and a verified destination", ErrInvalidConfig)
	}
	if ttl <= 0 || ttl > MaxWindowMinutes*time.Minute {
		return fmt.Errorf("%w: window of %s", ErrInvalidConfig, ttl)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	w.pruneLocked(now)
	if len(w.grants) >= maxGrants {
		return fmt.Errorf("%w: too many open windows", ErrInvalidConfig)
	}
	w.grants[g] = now.Add(ttl)
	return nil
}

// Allows reports whether g is covered by a grant that has not expired. A grant ends at its
// expiry instant, not after it.
func (w *ApprovalWindow) Allows(g Grant) bool {
	if g.Destination == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	expiry, ok := w.grants[g]
	return ok && w.now().Before(expiry)
}

// ForgetSession drops every grant of one session. It runs when the session closes, so an
// approval never outlives the connection it was given for — not even across a reconnect.
func (w *ApprovalWindow) ForgetSession(sessionID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for g := range w.grants {
		if g.SessionID == sessionID {
			delete(w.grants, g)
		}
	}
}

func (w *ApprovalWindow) pruneLocked(now time.Time) {
	for g, expiry := range w.grants {
		if !now.Before(expiry) {
			delete(w.grants, g)
		}
	}
}
