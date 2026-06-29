package domain

import "fmt"

const (
	// DefaultSSHPort is the standard SSH port.
	DefaultSSHPort = 22
	// MinPort is the minimum valid TCP port number.
	MinPort = 1
	// MaxPort is the maximum valid TCP port number.
	MaxPort = 65535
	// MaxTagLength is the maximum allowed length for a connection tag label.
	MaxTagLength = 30
)

// ProtocolType identifies the connection protocol.
const (
	ProtocolSSH = "ssh"
)

// Connection holds the configuration for a connection.
// Users holds one or more ConnectionUser references; DefaultUserID selects the active one.
// Legacy fields User and IdentityIDs are kept for vault v1 backward compatibility
// and are migrated into Users on vault upgrade.
// Vault JSON may still contain a legacy "proxy" key; encoding/json ignores unknown fields on unmarshal.
type Connection struct {
	ID       string `json:"id"`
	FolderID string `json:"folderId"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Order    int    `json:"order"`

	// Legacy (v1) — migrated to Users on vault v2 upgrade.
	User        string   `json:"user,omitempty"`
	IdentityIDs []string `json:"identityIds,omitempty"`

	// v2 fields
	Protocol      string           `json:"protocol,omitempty"` // ssh (default); other values require a plugin connector
	Users         []ConnectionUser `json:"users,omitempty"`
	DefaultUserID string           `json:"defaultUserId,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	JumpChain     JumpChainConfig  `json:"jumpChain,omitempty"`
}

// DefaultUser returns the ConnectionUser designated as default, or nil when none is set.
func (c *Connection) DefaultUser() *ConnectionUser {
	for i := range c.Users {
		if c.Users[i].ID == c.DefaultUserID {
			return &c.Users[i]
		}
	}
	return nil
}

// EffectiveUsername returns the username that should be used when opening a session.
// It prefers the default user; falls back to the legacy User field.
func (c *Connection) EffectiveUsername() string {
	if u := c.DefaultUser(); u != nil {
		return u.Username
	}
	return c.User
}

// GetProtocol returns the connection protocol, defaulting to ssh.
func (c *Connection) GetProtocol() string {
	if c.Protocol != "" {
		return c.Protocol
	}
	return ProtocolSSH
}

// EffectiveHost returns the host to use for ping/connect based on protocol.
func (c *Connection) EffectiveHost() string {
	if c.GetProtocol() == ProtocolSSH {
		return c.Host
	}
	return ""
}

// EffectivePort returns the port to use for ping/connect based on protocol.
func (c *Connection) EffectivePort() int {
	if c.GetProtocol() == ProtocolSSH {
		if c.Port > 0 {
			return c.Port
		}
		return DefaultSSHPort
	}
	return 0
}

// Validate checks structural constraints (port range, jump hop ports).
// It deliberately allows empty host/username so draft connections can be saved.
// Full readiness should be checked via ValidateForConnect before opening a session.
func (c *Connection) Validate() error {
	if c.Port < MinPort || c.Port > MaxPort {
		return fmt.Errorf("port %d out of range [%d-%d]: %w", c.Port, MinPort, MaxPort, ErrInvalidConnectionConfig)
	}
	for i := range c.JumpChain.Hops {
		if p := c.JumpChain.Hops[i].Port; p < MinPort || p > MaxPort {
			return fmt.Errorf("jump hop %d port %d out of range [%d-%d]: %w", i, p, MinPort, MaxPort, ErrInvalidConnectionConfig)
		}
	}
	for _, tag := range c.Tags {
		if len(tag) > MaxTagLength {
			return fmt.Errorf("tag %q exceeds max length %d: %w", tag, MaxTagLength, ErrInvalidConnectionConfig)
		}
	}
	if err := c.JumpChain.ValidateUniqueHopIDs(); err != nil {
		return err
	}
	return nil
}

func (c *Connection) validateHopsStrict() error {
	return c.JumpChain.ValidateStrict()
}

// ValidateForConnect performs strict validation required before opening a session.
func (c *Connection) ValidateForConnect() error {
	if c.Host == "" {
		return fmt.Errorf("host must not be empty: %w", ErrInvalidConnectionConfig)
	}
	if err := c.Validate(); err != nil {
		return err
	}
	switch c.GetProtocol() {
	case ProtocolSSH:
		return c.validateSSHForConnect()
	default:
		return c.validatePluginForConnect()
	}
}

func (c *Connection) validateSSHForConnect() error {
	if err := c.validateUsersForConnect(); err != nil {
		return err
	}
	return c.validateHopsStrict()
}

// validatePluginForConnect checks plugin-specific connect requirements only.
// Stored SSH users and jump hops are intentionally ignored here so protocol
// switching does not require clearing reversible draft data from the vault.
func (c *Connection) validatePluginForConnect() error {
	if c.Port < MinPort || c.Port > MaxPort {
		if c.Port != 0 {
			return fmt.Errorf("port %d out of range [%d-%d]: %w", c.Port, MinPort, MaxPort, ErrInvalidConnectionConfig)
		}
	}
	return nil
}

func (c *Connection) validateUsersForConnect() error {
	if len(c.Users) > 0 {
		if err := validateUniqueUserIDs(c.Users); err != nil {
			return err
		}
		if c.DefaultUserID == "" {
			return fmt.Errorf("default user must be selected: %w", ErrInvalidConnectionConfig)
		}
		defaultUser := c.DefaultUser()
		if defaultUser == nil {
			return fmt.Errorf("default user %q not found: %w", c.DefaultUserID, ErrInvalidConnectionConfig)
		}
		return defaultUser.Validate()
	}
	if c.User == "" {
		return fmt.Errorf("at least one user must be configured: %w", ErrInvalidConnectionConfig)
	}
	return nil
}

func validateUniqueUserIDs(users []ConnectionUser) error {
	seen := make(map[string]struct{}, len(users))
	for i, u := range users {
		if u.ID == "" {
			continue
		}
		if _, dup := seen[u.ID]; dup {
			return fmt.Errorf("user %d duplicate id %q: %w", i, u.ID, ErrInvalidConnectionConfig)
		}
		seen[u.ID] = struct{}{}
	}
	return nil
}

// WithDefaults fills in default values for optional fields (e.g., port).
func (c *Connection) WithDefaults() {
	if c.Port == 0 {
		c.Port = DefaultSSHPort
	}
}
