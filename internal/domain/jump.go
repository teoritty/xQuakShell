package domain

import "fmt"

// JumpHop describes a single intermediate SSH bastion host in a jump chain.
type JumpHop struct {
	ID       string         `json:"id,omitempty"`
	Host     string         `json:"host"`
	Port     int            `json:"port"`
	Username string         `json:"username"`
	Auth     AuthMethodType `json:"authMethod"`
	KeyAuth  *KeyAuthConfig      `json:"keyAuth,omitempty"`
	PassAuth *PasswordAuthConfig `json:"passAuth,omitempty"`
}

// Validate checks that the hop has valid host, port, username, and auth configuration.
// It is used for connect-time strict validation via JumpChainConfig.ValidateStrict.
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
	switch h.Auth {
	case AuthMethodKey:
		if h.KeyAuth == nil || len(h.KeyAuth.IdentityIDs) == 0 {
			return fmt.Errorf("jump hop key auth requires at least one identity: %w", ErrInvalidConnectionConfig)
		}
	case AuthMethodPassword:
		if h.PassAuth == nil || h.PassAuth.PasswordID == "" {
			return fmt.Errorf("jump hop password auth requires a password ID: %w", ErrInvalidConnectionConfig)
		}
	default:
		return fmt.Errorf("jump hop unknown auth method %q: %w", h.Auth, ErrInvalidConnectionConfig)
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

// ValidateUniqueHopIDs rejects duplicate non-empty hop IDs within the chain.
func (c *JumpChainConfig) ValidateUniqueHopIDs() error {
	seen := make(map[string]struct{}, len(c.Hops))
	for i, hop := range c.Hops {
		if hop.ID == "" {
			continue
		}
		if _, dup := seen[hop.ID]; dup {
			return fmt.Errorf("jump hop %d duplicate id %q: %w", i, hop.ID, ErrInvalidConnectionConfig)
		}
		seen[hop.ID] = struct{}{}
	}
	return nil
}

// ValidateStrict checks every hop is ready for connect (host, user, auth).
func (c *JumpChainConfig) ValidateStrict() error {
	for i := range c.Hops {
		if err := c.Hops[i].Validate(); err != nil {
			return fmt.Errorf("jump hop %d: %w", i, err)
		}
	}
	return nil
}
