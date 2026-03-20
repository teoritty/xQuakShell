package domain

import "time"

// LockoutSettings configures the automatic session lockout behaviour.
type LockoutSettings struct {
	Enabled        bool          `json:"enabled"`
	IdleTimeout    time.Duration `json:"idleTimeout"`
	LockOnMinimize bool          `json:"lockOnMinimize"`
}

// DefaultLockoutSettings returns sensible defaults (enabled, 5 min idle, lock on minimize).
func DefaultLockoutSettings() LockoutSettings {
	return LockoutSettings{
		Enabled:        false,
		IdleTimeout:    5 * time.Minute,
		LockOnMinimize: false,
	}
}
