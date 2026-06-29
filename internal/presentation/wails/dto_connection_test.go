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

func TestConnectionDTO_BlankJumpHopIDNotGenerated(t *testing.T) {
	dto := ConnectionDTO{
		ID:   "conn-1",
		Name: "test",
		Host: "target.example.com",
		Port: 22,
		JumpChain: []JumpHopDTO{
			{
				Host:     "bastion.example.com",
				Port:     22,
				Username: "jumpuser",
				Auth:     string(domain.AuthMethodKey),
			},
		},
	}

	conn := DTOToConnection(dto)
	if len(conn.JumpChain.Hops) != 1 {
		t.Fatalf("hop count: got %d want 1", len(conn.JumpChain.Hops))
	}
	if conn.JumpChain.Hops[0].ID != "" {
		t.Fatalf("DTO conversion must not generate hop id, got %q", conn.JumpChain.Hops[0].ID)
	}
}

func TestConnectionToDTO_DoesNotShareSlicesWithDomain(t *testing.T) {
	conn := domain.Connection{
		Tags:        []string{"prod"},
		IdentityIDs: []string{"k1"},
	}
	dto := ConnectionToDTO(conn)
	dto.Tags[0] = "changed"
	dto.IdentityIDs[0] = "changed"
	if conn.Tags[0] != "prod" {
		t.Fatal("tags shared with domain")
	}
	if conn.IdentityIDs[0] != "k1" {
		t.Fatal("identity ids shared with domain")
	}
}

func TestDTOToConnection_DoesNotShareSlicesWithDTO(t *testing.T) {
	dto := ConnectionDTO{
		Tags: []string{"prod"},
		Users: []ConnectionUserDTO{{
			ID:      "u1",
			KeyAuth: &KeyAuthConfigDTO{IdentityIDs: []string{"k1"}},
		}},
	}
	conn := DTOToConnection(dto)
	dto.Tags[0] = "changed"
	dto.Users[0].KeyAuth.IdentityIDs[0] = "changed"
	if conn.Tags[0] != "prod" {
		t.Fatal("tags shared with dto")
	}
	if conn.Users[0].KeyAuth.IdentityIDs[0] != "k1" {
		t.Fatal("user key ids shared with dto")
	}
}
