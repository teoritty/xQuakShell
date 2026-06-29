package wails

import (
	"testing"

	"ssh-client/internal/domain"
)

func TestConnectionDTO_JumpHopIDRoundTrip(t *testing.T) {
	conn := domain.Connection{
		ID:   "conn-1",
		Name: "test",
		Host: "target.example.com",
		Port: 22,
		JumpChain: domain.JumpChainConfig{
			Hops: []domain.JumpHop{
				{
					ID:       "hop-abc",
					Host:     "bastion.example.com",
					Port:     22,
					Username: "jumpuser",
					Auth:     domain.AuthMethodKey,
					KeyAuth:  &domain.KeyAuthConfig{IdentityIDs: []string{"key-1"}},
				},
			},
		},
	}

	dto := ConnectionToDTO(conn)
	if len(dto.JumpChain) != 1 {
		t.Fatalf("jump chain length: got %d want 1", len(dto.JumpChain))
	}
	if dto.JumpChain[0].ID != "hop-abc" {
		t.Fatalf("hop id in DTO: got %q want hop-abc", dto.JumpChain[0].ID)
	}

	back := DTOToConnection(dto)
	if len(back.JumpChain.Hops) != 1 {
		t.Fatalf("round-trip hop count: got %d want 1", len(back.JumpChain.Hops))
	}
	if back.JumpChain.Hops[0].ID != "hop-abc" {
		t.Fatalf("hop id after round-trip: got %q want hop-abc", back.JumpChain.Hops[0].ID)
	}
	if back.JumpChain.Hops[0].Host != "bastion.example.com" {
		t.Fatalf("hop host: got %q", back.JumpChain.Hops[0].Host)
	}
}
