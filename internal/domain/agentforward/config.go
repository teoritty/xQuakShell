package agentforward

import (
	"fmt"
	"slices"
	"time"
)

// ConfirmPolicy decides whether the confirmation dialog may offer a time window.
type ConfirmPolicy string

const (
	// ConfirmEach asks for every signature and offers no window. It is the default.
	ConfirmEach ConfirmPolicy = "each"
	// ConfirmSessionWindow asks too, but lets the user approve one key for one verified
	// destination for WindowMinutes, for scripts that sign repeatedly.
	ConfirmSessionWindow ConfirmPolicy = "session-window"
)

// Config is a connection's agent forwarding settings, stored in the vault.
//
// IdentityIDs is the connection's half of the two-sided consent; the key's AllowForwarding flag is
// the other half, and a key is offered only when both hold. The list is independent of the keys the
// connection logs in with: being able to reach a server is no reason to lend it a key.
type Config struct {
	Enabled       bool          `json:"enabled"`
	IdentityIDs   []string      `json:"identityIds,omitempty"`
	Confirm       ConfirmPolicy `json:"confirm,omitempty"`
	WindowMinutes int           `json:"windowMinutes,omitempty"`
}

// Validate checks structure only, so a draft — forwarding switched on before any key is picked —
// can still be saved. ValidateForConnect is the strict check.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	switch c.Confirm {
	case "", ConfirmEach, ConfirmSessionWindow:
	default:
		return fmt.Errorf("%w: confirm policy %q", ErrInvalidConfig, c.Confirm)
	}
	if c.WindowMinutes < 0 || c.WindowMinutes > MaxWindowMinutes {
		return fmt.Errorf("%w: window of %d minutes", ErrInvalidConfig, c.WindowMinutes)
	}
	if len(c.IdentityIDs) > MaxForwardedKeys {
		return fmt.Errorf("%w: %d keys, at most %d", ErrInvalidConfig, len(c.IdentityIDs), MaxForwardedKeys)
	}
	seen := make(map[string]bool, len(c.IdentityIDs))
	for _, id := range c.IdentityIDs {
		if id == "" || seen[id] {
			return fmt.Errorf("%w: empty or duplicate key id", ErrInvalidConfig)
		}
		seen[id] = true
	}
	return nil
}

// ValidateForConnect additionally requires an enabled configuration to name at least one key.
func (c *Config) ValidateForConnect() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c != nil && c.Enabled && len(c.IdentityIDs) == 0 {
		return fmt.Errorf("%w: forwarding is enabled but no key is selected", ErrInvalidConfig)
	}
	return nil
}

// Active reports whether a session for this connection gets an agent at all.
func (c *Config) Active() bool {
	return c != nil && c.Enabled && len(c.IdentityIDs) > 0
}

// Lists reports whether the connection forwards the key with this id.
func (c *Config) Lists(identityID string) bool {
	return c.Active() && slices.Contains(c.IdentityIDs, identityID)
}

// Clone returns a copy sharing no memory with c.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	out := *c
	out.IdentityIDs = slices.Clone(c.IdentityIDs)
	return &out
}

// Window is how long an "allow for this session" approval lasts, or zero when the connection
// does not offer one.
func (c *Config) Window() time.Duration {
	if c == nil || c.Confirm != ConfirmSessionWindow {
		return 0
	}
	minutes := c.WindowMinutes
	if minutes == 0 {
		minutes = DefaultWindowMinutes
	}
	return time.Duration(minutes) * time.Minute
}
