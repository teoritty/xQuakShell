package usecase

import (
	"context"
	"errors"
	"testing"

	"ssh-client/internal/domain"
)

func newTestVaultServiceWithForwardRules(conn domain.ConnectionRepository, validator *ForwardRuleValidator) *VaultService {
	return NewVaultService(VaultServiceConfig{
		ConnRepo:     conn,
		PasswordRepo: &memoryPasswordRepo{},
		IdentRepo:    &memoryIdentityRepo{},
	}).SetForwardRuleValidatorForTest(validator)
}

// SetForwardRuleValidatorForTest wires validation in tests without a separate setter call.
func (s *VaultService) SetForwardRuleValidatorForTest(v *ForwardRuleValidator) *VaultService {
	s.SetForwardRuleValidator(v)
	return s
}

func TestVaultServiceSaveConnection_WithValidLocalForwardRule(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	svc := newTestVaultServiceWithForwardRules(connRepo, NewForwardRuleValidator(connRepo, nil, nil))

	conn := &domain.Connection{
		ID:   "c1",
		Name: "Server",
		Host: "10.0.0.1",
		Port: 22,
		ForwardRules: []domain.ForwardRule{{
			Kind:        domain.ForwardRuleLocal,
			BindPort:    8080,
			TargetHost:  "db.internal",
			TargetPort:  5432,
			BindAddress: "127.0.0.1",
			Enabled:     true,
		}},
	}
	saved, err := svc.SaveConnection(ctx, conn, nil)
	if err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}
	if len(saved.ForwardRules) != 1 {
		t.Fatalf("forward rules: got %d, want 1", len(saved.ForwardRules))
	}
	if saved.ForwardRules[0].ID == "" {
		t.Fatal("expected generated forward rule id")
	}
	if saved.ForwardRules[0].TargetHost != "db.internal" {
		t.Fatalf("target host: got %q", saved.ForwardRules[0].TargetHost)
	}
}

func TestVaultServiceSaveConnection_RejectsDynamicWithoutConsent(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	validator := NewForwardRuleValidator(
		connRepo,
		stubForwardManifestLookup{hasProvider: true, providerOK: true},
		stubForwardGrantReader{granted: false},
	)
	svc := newTestVaultServiceWithForwardRules(connRepo, validator)

	conn := &domain.Connection{
		ID:   "c1",
		Name: "Server",
		Host: "10.0.0.1",
		Port: 22,
		ForwardRules: []domain.ForwardRule{{
			Kind:       domain.ForwardRuleDynamic,
			BindPort:   1080,
			PluginID:   "socks",
			ProviderID: "default",
			Enabled:    true,
		}},
	}
	_, err := svc.SaveConnection(ctx, conn, nil)
	if err == nil {
		t.Fatal("expected consent validation error")
	}
	if connRepo.saveCalls != 0 {
		t.Fatalf("connRepo.Save must not run on validation error, got %d calls", connRepo.saveCalls)
	}
}

func TestVaultServiceSaveConnection_RejectsDuplicateRuleIDs(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	svc := newTestVaultServiceWithForwardRules(connRepo, NewForwardRuleValidator(connRepo, nil, nil))

	conn := &domain.Connection{
		ID:   "c1",
		Name: "Server",
		Host: "10.0.0.1",
		Port: 22,
		ForwardRules: []domain.ForwardRule{
			{ID: "dup", Kind: domain.ForwardRuleLocal, BindPort: 8080, TargetHost: "a", TargetPort: 80},
			{ID: "dup", Kind: domain.ForwardRuleLocal, BindPort: 8081, TargetHost: "b", TargetPort: 80},
		},
	}
	_, err := svc.SaveConnection(ctx, conn, nil)
	if err == nil || !errors.Is(err, domain.ErrInvalidForwardRule) {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
	if connRepo.saveCalls != 0 {
		t.Fatalf("connRepo.Save must not run on validation error, got %d calls", connRepo.saveCalls)
	}
}

func TestVaultServiceSaveConnection_PreservesForwardRulesOnSubsequentSave(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	svc := newTestVaultServiceWithForwardRules(connRepo, NewForwardRuleValidator(connRepo, nil, nil))

	initial := &domain.Connection{
		ID:   "c1",
		Name: "Server",
		Host: "10.0.0.1",
		Port: 22,
		ForwardRules: []domain.ForwardRule{{
			Kind:       domain.ForwardRuleLocal,
			BindPort:   8080,
			TargetHost: "db.internal",
			TargetPort: 5432,
			Enabled:    true,
		}},
	}
	saved, err := svc.SaveConnection(ctx, initial, nil)
	if err != nil {
		t.Fatalf("initial SaveConnection: %v", err)
	}
	if len(saved.ForwardRules) != 1 {
		t.Fatalf("initial forward rules: got %d, want 1", len(saved.ForwardRules))
	}

	updated := *saved
	updated.Name = "Renamed server"
	updated.ForwardRules = saved.ForwardRules
	resaved, err := svc.SaveConnection(ctx, &updated, nil)
	if err != nil {
		t.Fatalf("second SaveConnection: %v", err)
	}
	if len(resaved.ForwardRules) != 1 {
		t.Fatalf("forward rules after rename: got %d, want 1", len(resaved.ForwardRules))
	}
	if resaved.ForwardRules[0].ID != saved.ForwardRules[0].ID {
		t.Fatalf("forward rule id changed: got %q want %q", resaved.ForwardRules[0].ID, saved.ForwardRules[0].ID)
	}
	if resaved.Name != "Renamed server" {
		t.Fatalf("name: got %q", resaved.Name)
	}
}

func TestVaultServiceSaveConnection_NilForwardRulesClearsRules(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	svc := newTestVaultServiceWithForwardRules(connRepo, NewForwardRuleValidator(connRepo, nil, nil))

	conn := &domain.Connection{
		ID:   "c1",
		Name: "Server",
		Host: "10.0.0.1",
		Port: 22,
		ForwardRules: []domain.ForwardRule{{
			Kind:       domain.ForwardRuleLocal,
			BindPort:   8080,
			TargetHost: "db.internal",
			TargetPort: 5432,
			Enabled:    true,
		}},
	}
	saved, err := svc.SaveConnection(ctx, conn, nil)
	if err != nil {
		t.Fatalf("initial SaveConnection: %v", err)
	}
	if len(saved.ForwardRules) != 1 {
		t.Fatal("expected one forward rule after initial save")
	}

	// Simulates legacy client payloads that omitted forwardRules entirely.
	cleared := *saved
	cleared.ForwardRules = nil
	resaved, err := svc.SaveConnection(ctx, &cleared, nil)
	if err != nil {
		t.Fatalf("second SaveConnection: %v", err)
	}
	if len(resaved.ForwardRules) != 0 {
		t.Fatalf("nil forwardRules must clear rules (legacy client path), got %d", len(resaved.ForwardRules))
	}
}
