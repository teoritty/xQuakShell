package usecase

import (
	"context"
	"errors"
	"testing"

	"ssh-client/internal/domain"
)

type forwardVaultStubConnRepo struct {
	conns map[string]domain.Connection
}

func newForwardVaultStubConnRepo(conns ...domain.Connection) *forwardVaultStubConnRepo {
	m := make(map[string]domain.Connection, len(conns))
	for _, c := range conns {
		m[c.ID] = c
	}
	return &forwardVaultStubConnRepo{conns: m}
}

func (s *forwardVaultStubConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return nil, nil
}
func (s *forwardVaultStubConnRepo) SaveFolder(context.Context, *domain.ConnectionFolder) error { return nil }
func (s *forwardVaultStubConnRepo) DeleteFolder(context.Context, string) error                   { return nil }
func (s *forwardVaultStubConnRepo) GetAllConnections(ctx context.Context) ([]domain.Connection, error) {
	out := make([]domain.Connection, 0, len(s.conns))
	for _, c := range s.conns {
		out = append(out, c)
	}
	return out, nil
}
func (s *forwardVaultStubConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}
func (s *forwardVaultStubConnRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	c, ok := s.conns[id]
	if !ok {
		return nil, domain.ErrConnectionNotFound
	}
	cp := c
	return &cp, nil
}
func (s *forwardVaultStubConnRepo) Save(_ context.Context, c *domain.Connection) error {
	cp := *c
	s.conns[c.ID] = cp
	return nil
}
func (s *forwardVaultStubConnRepo) Delete(context.Context, string) error { return nil }
func (s *forwardVaultStubConnRepo) MoveToFolder(context.Context, []string, string) error {
	return nil
}
func (s *forwardVaultStubConnRepo) MoveFolder(context.Context, string, string) error { return nil }
func (s *forwardVaultStubConnRepo) ReorderConnections(context.Context, []string, string) error {
	return nil
}
func (s *forwardVaultStubConnRepo) ReorderFolders(context.Context, []string, string) error { return nil }

func TestForwardRuleVaultService_ListReturnsRules(t *testing.T) {
	repo := newForwardVaultStubConnRepo(domain.Connection{
		ID: "conn-1",
		ForwardRules: []domain.ForwardRule{
			{ID: "r1", Kind: domain.ForwardRuleLocal, BindPort: 8080, TargetHost: "h", TargetPort: 80, Enabled: true},
		},
	})
	svc := NewForwardRuleVaultService(repo, nil)

	rules, err := svc.List(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "r1" {
		t.Fatalf("unexpected rules: %#v", rules)
	}
}

func TestForwardRuleVaultService_SaveGeneratesID(t *testing.T) {
	repo := newForwardVaultStubConnRepo(domain.Connection{ID: "conn-1"})
	svc := NewForwardRuleVaultService(repo, nil)

	saved, err := svc.Save(context.Background(), "conn-1", domain.ForwardRule{
		Kind: domain.ForwardRuleLocal, BindPort: 8080, TargetHost: "h", TargetPort: 80, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected generated rule id")
	}
	conn, _ := repo.GetByID(context.Background(), "conn-1")
	if len(conn.ForwardRules) != 1 || conn.ForwardRules[0].ID != saved.ID {
		t.Fatalf("connection rules = %#v, want id %q", conn.ForwardRules, saved.ID)
	}
}

func TestForwardRuleVaultService_SaveRejectsCrossConnectionDuplicateID(t *testing.T) {
	repo := newForwardVaultStubConnRepo(
		domain.Connection{
			ID: "other",
			ForwardRules: []domain.ForwardRule{
				{ID: "dup", Kind: domain.ForwardRuleLocal, BindPort: 8080, TargetHost: "h", TargetPort: 80},
			},
		},
		domain.Connection{ID: "conn-1"},
	)
	v := NewForwardRuleValidator(repo, nil, nil)
	svc := NewForwardRuleVaultService(repo, v)

	_, err := svc.Save(context.Background(), "conn-1", domain.ForwardRule{
		ID: "dup", Kind: domain.ForwardRuleLocal, BindPort: 8081, TargetHost: "h", TargetPort: 80,
	})
	if err == nil || !errors.Is(err, domain.ErrInvalidForwardRule) {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestForwardRuleVaultService_SaveRejectsDynamicWithoutGrant(t *testing.T) {
	repo := newForwardVaultStubConnRepo(domain.Connection{ID: "conn-1"})
	v := NewForwardRuleValidator(repo, stubForwardManifestLookup{hasProvider: true, providerOK: true}, stubForwardGrantReader{granted: false})
	svc := NewForwardRuleVaultService(repo, v)

	_, err := svc.Save(context.Background(), "conn-1", domain.ForwardRule{
		ID: "dyn-1", Kind: domain.ForwardRuleDynamic, BindPort: 1080, PluginID: "p1", ProviderID: "socks5",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestForwardRuleVaultService_DeleteMissingRule(t *testing.T) {
	repo := newForwardVaultStubConnRepo(domain.Connection{ID: "conn-1"})
	svc := NewForwardRuleVaultService(repo, nil)

	err := svc.Delete(context.Background(), "conn-1", "missing")
	if !errors.Is(err, domain.ErrForwardRuleNotFound) {
		t.Fatalf("expected ErrForwardRuleNotFound, got %v", err)
	}
}

func TestForwardRuleVaultService_SetEnabledMissingRule(t *testing.T) {
	repo := newForwardVaultStubConnRepo(domain.Connection{ID: "conn-1"})
	svc := NewForwardRuleVaultService(repo, nil)

	err := svc.SetEnabled(context.Background(), "conn-1", "missing", true)
	if !errors.Is(err, domain.ErrForwardRuleNotFound) {
		t.Fatalf("expected ErrForwardRuleNotFound, got %v", err)
	}
}

func TestForwardRuleVaultService_ConnectionNotFound(t *testing.T) {
	svc := NewForwardRuleVaultService(newForwardVaultStubConnRepo(), nil)

	_, err := svc.List(context.Background(), "missing")
	if !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Fatalf("expected ErrConnectionNotFound, got %v", err)
	}
}
