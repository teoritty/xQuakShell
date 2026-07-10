package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"ssh-client/internal/domain"
)

func generateForwardRuleID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate forward rule id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func prepareForwardRuleForSave(ctx context.Context, validator *ForwardRuleValidator, connectionID string, rule domain.ForwardRule) (domain.ForwardRule, error) {
	if validator != nil {
		if err := validator.ValidateRule(rule); err != nil {
			return domain.ForwardRule{}, err
		}
	}
	if rule.ID == "" {
		id, err := generateForwardRuleID()
		if err != nil {
			return domain.ForwardRule{}, err
		}
		rule.ID = id
	}
	if validator != nil {
		if err := validator.ValidateRuleIDUnique(ctx, rule.ID, connectionID); err != nil {
			return domain.ForwardRule{}, err
		}
	}
	rule.BindAddress = domain.EffectiveBindAddress(rule.BindAddress)
	return rule, nil
}

func prepareConnectionForwardRules(ctx context.Context, validator *ForwardRuleValidator, connectionID string, rules []domain.ForwardRule) ([]domain.ForwardRule, error) {
	if len(rules) == 0 {
		return rules, nil
	}
	out := make([]domain.ForwardRule, 0, len(rules))
	for _, rule := range rules {
		prepared, err := prepareForwardRuleForSave(ctx, validator, connectionID, rule)
		if err != nil {
			return nil, err
		}
		out = append(out, prepared)
	}
	if err := ValidateConnectionRules(out); err != nil {
		return nil, err
	}
	return out, nil
}
