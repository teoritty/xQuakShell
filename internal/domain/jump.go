package domain

import "fmt"

// JumpHop describes a single intermediate SSH bastion host in a jump chain.
type JumpHop struct {
	Host     string         `json:"host"`
	Port     int            `json:"port"`
	Username string         `json:"username"`
	Auth     AuthMethodType `json:"authMethod"`
	KeyAuth  *KeyAuthConfig      `json:"keyAuth,omitempty"`
	PassAuth *PasswordAuthConfig `json:"passAuth,omitempty"`
}

// Validate checks that the hop has valid host, port and auth configuration.
func (h *JumpHop) Validate() error {
	if h.Host == "" {
		return fmt.Errorf("jump hop host must not be empty: %w", ErrInvalidConnectionConfig)
	}
	if h.Port < MinPort || h.Port > MaxPort {
		return fmt.Errorf("jump hop port %d out of range: %w", h.Port, ErrInvalidConnectionConfig)
	}
	if h.Username == "" {
		return fmt.Errorf("jump hop username must not be empty: %w", ErrInvalidConnectionConfig)
	}
	return nil
}

// JumpChainConfig describes an ordered list of hops to reach the target host.
// Hops are traversed in order: Client → Hop[0] → Hop[1] → … → Target.
type JumpChainConfig struct {
	Hops []JumpHop `json:"hops,omitempty"`
}

// IsEmpty returns true when no intermediate hops are configured.
func (c *JumpChainConfig) IsEmpty() bool {
	return len(c.Hops) == 0
}
