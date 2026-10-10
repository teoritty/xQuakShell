package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain/agentforward"
)

func sshConnectionForAgentTest() Connection {
	return Connection{
		ID: "c1", Host: "bastion", Port: 22,
		Users: []ConnectionUser{{ID: "u1", Username: "ops", Auth: AuthMethodKey,
			KeyAuth: &KeyAuthConfig{IdentityIDs: []string{"login-key"}}}},
		DefaultUserID: "u1",
	}
}

func TestConnectionValidateRefusesMalformedAgentForwarding(t *testing.T) {
	c := sshConnectionForAgentTest()
	c.AgentForwarding = &agentforward.Config{Enabled: true, IdentityIDs: []string{"k", "k"}}
	err := c.Validate()
	if !errors.Is(err, ErrInvalidConnectionConfig) || !errors.Is(err, agentforward.ErrInvalidConfig) {
		t.Fatalf("err = %v; a malformed forwarding config must fail as both a connection and a forwarding error", err)
	}
}

func TestConnectionValidateSavesAForwardingDraft(t *testing.T) {
	c := sshConnectionForAgentTest()
	c.AgentForwarding = &agentforward.Config{Enabled: true}
	if err := c.Validate(); err != nil {
		t.Errorf("forwarding switched on before a key is picked is a draft and must save: %v", err)
	}
	if err := c.ValidateForConnect(); !errors.Is(err, agentforward.ErrInvalidConfig) {
		t.Errorf("but it must not connect: err = %v", err)
	}
}

func TestConnectionValidateForConnectAcceptsCompleteForwarding(t *testing.T) {
	c := sshConnectionForAgentTest()
	c.AgentForwarding = &agentforward.Config{Enabled: true, IdentityIDs: []string{"fwd-key"}}
	if err := c.ValidateForConnect(); err != nil {
		t.Fatal(err)
	}
}

func TestCloneConnectionDoesNotShareAgentForwarding(t *testing.T) {
	c := sshConnectionForAgentTest()
	c.AgentForwarding = &agentforward.Config{Enabled: true, IdentityIDs: []string{"fwd-key"}}
	clone := CloneConnection(c)
	clone.AgentForwarding.IdentityIDs[0] = "other"
	clone.AgentForwarding.Enabled = false
	if c.AgentForwarding.IdentityIDs[0] != "fwd-key" || !c.AgentForwarding.Enabled {
		t.Fatal("editing a clone's forwarding settings changed the original")
	}
}

// The fields are additive and omitempty, so a vault written by this build without forwarding is
// byte-for-byte what the previous build wrote, and the schema version need not move (ADR-017).
func TestAgentForwardingFieldsAreOmittedWhenUnset(t *testing.T) {
	raw, err := json.Marshal(sshConnectionForAgentTest())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "agentForwarding") {
		t.Errorf("an unset forwarding config must not be persisted: %s", raw)
	}
	raw, err = json.Marshal(SSHIdentity{ID: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "allowForwarding") {
		t.Errorf("an unset key flag must not be persisted: %s", raw)
	}
}
