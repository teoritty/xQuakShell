package wails_test

import (
	"context"
	"errors"
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/presentation/wails"
	"ssh-client/internal/usecase"
)

func newForwardRulesTestAPI(t *testing.T, conns ...domain.Connection) *wails.AppAPI {
	t.Helper()
	repo := newForwardVaultStubConnRepo(conns...)
	svc := usecase.NewForwardRuleVaultService(repo, nil)
	api := &wails.AppAPI{}
	api.SetForwardRuleVaultService(svc)
	return api
}

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

func TestListForwardRules_ReturnsConnectionRules(t *testing.T) {
	api := newForwardRulesTestAPI(t, domain.Connection{
		ID: "conn-1",
		ForwardRules: []domain.ForwardRule{
			{ID: "r1", Kind: domain.ForwardRuleLocal, BindAddress: "127.0.0.1", BindPort: 8080, TargetHost: "h", TargetPort: 80, Enabled: true},
		},
	})

	rules, err := api.ListForwardRules("conn-1")
	if err != nil {
		t.Fatalf("ListForwardRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "r1" {
		t.Fatalf("unexpected rules: %#v", rules)
	}
}

func TestSaveForwardRule_PersistsNewRule(t *testing.T) {
	api := newForwardRulesTestAPI(t, domain.Connection{ID: "conn-1"})

	err := api.SaveForwardRule("conn-1", wails.ForwardRuleDTO{
		Kind:        string(domain.ForwardRuleLocal),
		BindAddress: "127.0.0.1",
		BindPort:    8080,
		TargetHost:  "h",
		TargetPort:  80,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveForwardRule: %v", err)
	}

	rules, err := api.ListForwardRules("conn-1")
	if err != nil {
		t.Fatalf("ListForwardRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID == "" {
		t.Fatalf("expected one saved rule with generated id, got %#v", rules)
	}
}

func TestDeleteForwardRule_NotFound(t *testing.T) {
	api := newForwardRulesTestAPI(t, domain.Connection{ID: "conn-1"})

	err := api.DeleteForwardRule("conn-1", "missing")
	if !errors.Is(err, domain.ErrForwardRuleNotFound) {
		t.Fatalf("expected ErrForwardRuleNotFound, got %v", err)
	}
}

func TestSetForwardRuleEnabled_NotFound(t *testing.T) {
	api := newForwardRulesTestAPI(t, domain.Connection{ID: "conn-1"})

	err := api.SetForwardRuleEnabled("conn-1", "missing", true)
	if !errors.Is(err, domain.ErrForwardRuleNotFound) {
		t.Fatalf("expected ErrForwardRuleNotFound, got %v", err)
	}
}

func TestListForwardRules_ConnectionNotFound(t *testing.T) {
	api := newForwardRulesTestAPI(t)

	_, err := api.ListForwardRules("missing")
	if !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Fatalf("expected ErrConnectionNotFound, got %v", err)
	}
}
