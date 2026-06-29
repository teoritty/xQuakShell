package domain

import (
	"errors"
	"testing"
)

func TestJumpHop_Validate_RejectsMissingKeyIdentities(t *testing.T) {
	h := JumpHop{
		Host:     "bastion",
		Port:     22,
		Username: "jump",
		Auth:     AuthMethodKey,
	}
	err := h.Validate()
	if err == nil {
		t.Fatal("expected key auth error")
	}
	if !errors.Is(err, ErrInvalidConnectionConfig) {
		t.Fatalf("expected ErrInvalidConnectionConfig, got %v", err)
	}
}

func TestJumpHop_Validate_RejectsMissingPasswordID(t *testing.T) {
	h := JumpHop{
		Host:     "bastion",
		Port:     22,
		Username: "jump",
		Auth:     AuthMethodPassword,
	}
	if err := h.Validate(); err == nil {
		t.Fatal("expected password auth error")
	}
}

func TestJumpHop_Validate_AcceptsKeyAuth(t *testing.T) {
	h := JumpHop{
		Host:     "bastion",
		Port:     22,
		Username: "jump",
		Auth:     AuthMethodKey,
		KeyAuth:  &KeyAuthConfig{IdentityIDs: []string{"id-1"}},
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("expected valid hop: %v", err)
	}
}

func TestJumpHop_Validate_AcceptsPasswordAuth(t *testing.T) {
	h := JumpHop{
		Host:     "bastion",
		Port:     22,
		Username: "jump",
		Auth:     AuthMethodPassword,
		PassAuth: &PasswordAuthConfig{PasswordID: "pw-1"},
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("expected valid hop: %v", err)
	}
}

func TestJumpChainConfig_ValidateUniqueHopIDs_RejectsDuplicates(t *testing.T) {
	chain := JumpChainConfig{
		Hops: []JumpHop{
			{ID: "same", Port: 22},
			{ID: "same", Port: 2222},
		},
	}
	if err := chain.ValidateUniqueHopIDs(); err == nil {
		t.Fatal("expected duplicate hop id error")
	}
}

func TestJumpChainConfig_ValidateUniqueHopIDs_AllowsBlankIDs(t *testing.T) {
	chain := JumpChainConfig{
		Hops: []JumpHop{
			{Port: 22},
			{Port: 2222},
		},
	}
	if err := chain.ValidateUniqueHopIDs(); err != nil {
		t.Fatalf("blank hop ids should be allowed: %v", err)
	}
}
