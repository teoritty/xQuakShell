package usecase

import (
	"context"
	"errors"
	"testing"

	"ssh-client/internal/domain"
)

type stubForwardManifestLookup struct {
	hasProvider bool
	providerOK  bool
}

func (s stubForwardManifestLookup) HasTunnelProvider(_ string) (bool, error) {
	return s.hasProvider, nil
}

func (s stubForwardManifestLookup) TunnelProviderExists(_, _ string) (bool, error) {
	return s.providerOK, nil
}

type stubForwardGrantReader struct {
	granted bool
}

func (s stubForwardGrantReader) IsTunnelProviderGranted(_ string) bool {
	return s.granted
}

func TestForwardRuleValidator_RejectsInvalidProviderID(t *testing.T) {
	v := NewForwardRuleValidator(nil, stubForwardManifestLookup{hasProvider: true, providerOK: false}, stubForwardGrantReader{granted: true})
	err := v.ValidateRule(domain.ForwardRule{
		ID: "r1", Kind: domain.ForwardRuleDynamic, BindPort: 1080,
		PluginID: "p1", ProviderID: "missing",
	})
	if err == nil || !errors.Is(err, domain.ErrInvalidForwardRule) {
		t.Fatalf("expected invalid forward rule, got %v", err)
	}
}

func TestForwardRuleValidator_RejectsCrossConnectionDuplicateID(t *testing.T) {
	repo := &validatorStubConnRepo{conns: []domain.Connection{
		{ID: "other", ForwardRules: []domain.ForwardRule{{ID: "dup-id"}}},
	}}
	v := NewForwardRuleValidator(repo, nil, nil)
	err := v.ValidateRuleIDUnique(context.Background(), "dup-id", "mine")
	if err == nil || !errors.Is(err, domain.ErrInvalidForwardRule) {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestValidateConnectionRules_RejectsDuplicateIDs(t *testing.T) {
	err := ValidateConnectionRules([]domain.ForwardRule{
		{ID: "same", Kind: domain.ForwardRuleLocal, BindPort: 8080, TargetHost: "h", TargetPort: 80},
		{ID: "same", Kind: domain.ForwardRuleLocal, BindPort: 8081, TargetHost: "h", TargetPort: 80},
	})
	if err == nil || !errors.Is(err, domain.ErrInvalidForwardRule) {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

type validatorStubConnRepo struct {
	conns []domain.Connection
}

func (s *validatorStubConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return nil, nil
}
func (s *validatorStubConnRepo) SaveFolder(context.Context, *domain.ConnectionFolder) error { return nil }
func (s *validatorStubConnRepo) DeleteFolder(context.Context, string) error                   { return nil }
func (s *validatorStubConnRepo) GetAllConnections(context.Context) ([]domain.Connection, error) {
	return s.conns, nil
}
func (s *validatorStubConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}
func (s *validatorStubConnRepo) GetByID(context.Context, string) (*domain.Connection, error) {
	return nil, domain.ErrConnectionNotFound
}
func (s *validatorStubConnRepo) Save(context.Context, *domain.Connection) error { return nil }
func (s *validatorStubConnRepo) Delete(context.Context, string) error         { return nil }
func (s *validatorStubConnRepo) MoveToFolder(context.Context, []string, string) error {
	return nil
}
func (s *validatorStubConnRepo) MoveFolder(context.Context, string, string) error { return nil }
func (s *validatorStubConnRepo) ReorderConnections(context.Context, []string, string) error {
	return nil
}
func (s *validatorStubConnRepo) ReorderFolders(context.Context, []string, string) error { return nil }
