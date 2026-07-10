package usecase

import (
	"context"

	"ssh-client/internal/domain"
)

// ForwardRuleVaultService manages forward rules persisted on vault connections.
type ForwardRuleVaultService struct {
	connRepo  domain.ConnectionRepository
	validator *ForwardRuleValidator
}

// NewForwardRuleVaultService creates a forward rule vault service.
func NewForwardRuleVaultService(connRepo domain.ConnectionRepository, validator *ForwardRuleValidator) *ForwardRuleVaultService {
	if connRepo == nil {
		panic("usecase: ForwardRuleVaultService requires ConnectionRepository")
	}
	return &ForwardRuleVaultService{connRepo: connRepo, validator: validator}
}

// List returns forward rules for a connection.
func (s *ForwardRuleVaultService) List(ctx context.Context, connectionID string) ([]domain.ForwardRule, error) {
	conn, err := s.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	return conn.ForwardRules, nil
}

// Save creates or updates a forward rule on a connection.
func (s *ForwardRuleVaultService) Save(ctx context.Context, connectionID string, rule domain.ForwardRule) (domain.ForwardRule, error) {
	prepared, err := prepareForwardRuleForSave(ctx, s.validator, connectionID, rule)
	if err != nil {
		return domain.ForwardRule{}, err
	}
	rule = prepared

	conn, err := s.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return domain.ForwardRule{}, err
	}
	replaced := false
	for i := range conn.ForwardRules {
		if conn.ForwardRules[i].ID == rule.ID {
			conn.ForwardRules[i] = rule
			replaced = true
			break
		}
	}
	if !replaced {
		conn.ForwardRules = append(conn.ForwardRules, rule)
	}
	if err := ValidateConnectionRules(conn.ForwardRules); err != nil {
		return domain.ForwardRule{}, err
	}
	if err := s.connRepo.Save(ctx, conn); err != nil {
		return domain.ForwardRule{}, err
	}
	return rule, nil
}

// Delete removes a forward rule from a connection.
func (s *ForwardRuleVaultService) Delete(ctx context.Context, connectionID, ruleID string) error {
	conn, err := s.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return err
	}
	next := conn.ForwardRules[:0]
	found := false
	for _, r := range conn.ForwardRules {
		if r.ID == ruleID {
			found = true
			continue
		}
		next = append(next, r)
	}
	if !found {
		return domain.ErrForwardRuleNotFound
	}
	conn.ForwardRules = next
	return s.connRepo.Save(ctx, conn)
}

// SetEnabled toggles a forward rule without removing it.
func (s *ForwardRuleVaultService) SetEnabled(ctx context.Context, connectionID, ruleID string, enabled bool) error {
	conn, err := s.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return err
	}
	found := false
	for i := range conn.ForwardRules {
		if conn.ForwardRules[i].ID == ruleID {
			conn.ForwardRules[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		return domain.ErrForwardRuleNotFound
	}
	return s.connRepo.Save(ctx, conn)
}
