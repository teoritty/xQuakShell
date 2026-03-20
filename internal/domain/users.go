package domain

import "fmt"

// AuthMethodType distinguishes between key-based and password-based SSH authentication.
type AuthMethodType string

const (
	// AuthMethodKey indicates SSH public key authentication.
	AuthMethodKey AuthMethodType = "key"
	// AuthMethodPassword indicates SSH password authentication.
	AuthMethodPassword AuthMethodType = "password"
)

// KeyAuthConfig holds references to SSH identity entries for key-based authentication.
type KeyAuthConfig struct {
	IdentityIDs []string `json:"identityIds"`
}

// PasswordAuthConfig holds a reference to an encrypted password stored in the vault.
type PasswordAuthConfig struct {
	PasswordID string `json:"passwordId"`
}

// ConnectionUser represents one of potentially many users configured for a single connection.
// Each user has an independent authentication method (key or password).
type ConnectionUser struct {
	ID       string         `json:"id"`
	Username string         `json:"username"`
	Auth     AuthMethodType `json:"authMethod"`
	KeyAuth  *KeyAuthConfig      `json:"keyAuth,omitempty"`
	PassAuth *PasswordAuthConfig `json:"passAuth,omitempty"`
	Label    string         `json:"label,omitempty"`
}

// Validate checks that the user has a non-empty username and a valid auth configuration.
func (u *ConnectionUser) Validate() error {
	if u.Username == "" {
		return fmt.Errorf("username must not be empty: %w", ErrInvalidConnectionConfig)
	}
	switch u.Auth {
	case AuthMethodKey:
		if u.KeyAuth == nil || len(u.KeyAuth.IdentityIDs) == 0 {
			return fmt.Errorf("key auth requires at least one identity: %w", ErrInvalidConnectionConfig)
		}
	case AuthMethodPassword:
		if u.PassAuth == nil || u.PassAuth.PasswordID == "" {
			return fmt.Errorf("password auth requires a password ID: %w", ErrInvalidConnectionConfig)
		}
	default:
		return fmt.Errorf("unknown auth method %q: %w", u.Auth, ErrInvalidConnectionConfig)
	}
	return nil
}
