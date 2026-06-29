package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestConnection_Validate_AllowsDraftWithIncompleteHops(t *testing.T) {
	c := Connection{
		Port: 22,
		JumpChain: JumpChainConfig{
			Hops: []JumpHop{
				{Port: 22},
			},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("draft validate: %v", err)
	}
}

func TestConnection_Validate_RejectsInvalidConnectionPort(t *testing.T) {
	c := Connection{Port: 0}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected invalid connection port error")
	}
	if !errors.Is(err, ErrInvalidConnectionConfig) {
		t.Fatalf("expected ErrInvalidConnectionConfig, got %v", err)
	}
}

func TestConnection_Validate_RejectsInvalidJumpHopPort(t *testing.T) {
	c := Connection{
		Port: 22,
		JumpChain: JumpChainConfig{
			Hops: []JumpHop{{Port: 0}},
		},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid hop port error")
	}
}

func TestConnection_Validate_RejectsDuplicateNonEmptyHopIDs(t *testing.T) {
	c := Connection{
		Port: 22,
		JumpChain: JumpChainConfig{
			Hops: []JumpHop{
				{ID: "hop-1", Port: 22},
				{ID: "hop-1", Port: 2222},
			},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected duplicate hop id error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate in error, got %v", err)
	}
}

func TestConnection_Validate_IgnoresBlankHopIDsForUniqueness(t *testing.T) {
	c := Connection{
		Port: 22,
		JumpChain: JumpChainConfig{
			Hops: []JumpHop{
				{Port: 22},
				{Port: 2222},
			},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("blank hop ids should not fail uniqueness: %v", err)
	}
}

func TestConnection_Validate_RejectsTagLongerThanMax(t *testing.T) {
	c := Connection{
		Port: 22,
		Tags: []string{strings.Repeat("a", MaxTagLength+1)},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected tag length error")
	}
}

func TestConnection_ValidateForConnect_RejectsHopKeyAuthWithoutIdentities(t *testing.T) {
	c := sshConnectionReady(t)
	c.JumpChain = JumpChainConfig{
		Hops: []JumpHop{
			{
				ID:       "hop-1",
				Host:     "bastion",
				Port:     22,
				Username: "jump",
				Auth:     AuthMethodKey,
			},
		},
	}
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected hop key auth validation error")
	}
}

func TestConnection_ValidateForConnect_RejectsHopPasswordAuthWithoutPasswordID(t *testing.T) {
	c := sshConnectionReady(t)
	c.JumpChain = JumpChainConfig{
		Hops: []JumpHop{
			{
				ID:       "hop-1",
				Host:     "bastion",
				Port:     22,
				Username: "jump",
				Auth:     AuthMethodPassword,
			},
		},
	}
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected hop password auth validation error")
	}
}

func TestConnection_ValidateForConnect_RejectsUnknownHopAuthMethod(t *testing.T) {
	c := sshConnectionReady(t)
	c.JumpChain = JumpChainConfig{
		Hops: []JumpHop{
			{
				ID:       "hop-1",
				Host:     "bastion",
				Port:     22,
				Username: "jump",
				Auth:     AuthMethodType("token"),
			},
		},
	}
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected unknown hop auth method error")
	}
}

func TestConnection_ValidateForConnect_AcceptsConfiguredJumpHops(t *testing.T) {
	c := sshConnectionReady(t)
	c.JumpChain = JumpChainConfig{
		Hops: []JumpHop{
			{
				ID:       "hop-key",
				Host:     "bastion1",
				Port:     22,
				Username: "jump1",
				Auth:     AuthMethodKey,
				KeyAuth:  &KeyAuthConfig{IdentityIDs: []string{"id-1"}},
			},
			{
				ID:       "hop-pass",
				Host:     "bastion2",
				Port:     2222,
				Username: "jump2",
				Auth:     AuthMethodPassword,
				PassAuth: &PasswordAuthConfig{PasswordID: "pw-1"},
			},
		},
	}
	if err := c.ValidateForConnect(); err != nil {
		t.Fatalf("expected valid jump chain: %v", err)
	}
}

func TestConnection_ValidateForConnect_RejectsMissingDefaultUser(t *testing.T) {
	c := sshConnectionReady(t)
	c.DefaultUserID = "missing"
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected missing default user error")
	}
}

func TestConnection_ValidateForConnect_RejectsDefaultKeyUserWithoutIdentities(t *testing.T) {
	c := sshConnectionReady(t)
	c.Users[0].KeyAuth = nil
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected key auth error")
	}
}

func TestConnection_ValidateForConnect_RejectsDefaultPasswordUserWithoutPasswordID(t *testing.T) {
	c := sshConnectionReady(t)
	c.Users[0].Auth = AuthMethodPassword
	c.Users[0].KeyAuth = nil
	c.Users[0].PassAuth = nil
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected password auth error")
	}
}

func TestConnection_ValidateForConnect_RejectsDuplicateUserIDs(t *testing.T) {
	c := sshConnectionReady(t)
	c.Users = append(c.Users, ConnectionUser{
		ID:       "user-1",
		Username: "bob",
		Auth:     AuthMethodKey,
		KeyAuth:  &KeyAuthConfig{IdentityIDs: []string{"key-2"}},
	})
	if err := c.ValidateForConnect(); err == nil {
		t.Fatal("expected duplicate user id error")
	}
}

func TestConnection_ValidateForConnect_PluginSkipsJumpChainStrict(t *testing.T) {
	c := Connection{
		Protocol: "rdp",
		Host:     "target.example.com",
		Port:     3389,
		JumpChain: JumpChainConfig{
			Hops: []JumpHop{{Host: "bastion", Port: 22}},
		},
	}
	if err := c.ValidateForConnect(); err != nil {
		t.Fatalf("plugin connect should not validate jump hops: %v", err)
	}
}

func sshConnectionReady(t *testing.T) Connection {
	t.Helper()
	return Connection{
		Host: "target.example.com",
		Port: 22,
		Users: []ConnectionUser{
			{
				ID:       "user-1",
				Username: "alice",
				Auth:     AuthMethodKey,
				KeyAuth:  &KeyAuthConfig{IdentityIDs: []string{"key-1"}},
			},
		},
		DefaultUserID: "user-1",
	}
}
