package wails

import (
	"context"
	"fmt"
)

// ListForwardRules returns forward rules for a connection.
func (a *AppAPI) ListForwardRules(connectionID string) ([]ForwardRuleDTO, error) {
	if a.forwardRuleVault == nil {
		return nil, fmt.Errorf("forward rule service unavailable")
	}
	rules, err := a.forwardRuleVault.List(context.Background(), connectionID)
	if err != nil {
		return nil, err
	}
	out := make([]ForwardRuleDTO, 0, len(rules))
	for _, r := range rules {
		out = append(out, forwardRuleToDTO(r))
	}
	return out, nil
}

// SaveForwardRule creates or updates a forward rule on a connection.
func (a *AppAPI) SaveForwardRule(connectionID string, rule ForwardRuleDTO) error {
	if a.forwardRuleVault == nil {
		return fmt.Errorf("forward rule service unavailable")
	}
	_, err := a.forwardRuleVault.Save(context.Background(), connectionID, forwardRuleFromDTO(rule))
	return err
}

// DeleteForwardRule removes a forward rule from a connection.
func (a *AppAPI) DeleteForwardRule(connectionID, ruleID string) error {
	if a.forwardRuleVault == nil {
		return fmt.Errorf("forward rule service unavailable")
	}
	return a.forwardRuleVault.Delete(context.Background(), connectionID, ruleID)
}

// SetForwardRuleEnabled toggles a forward rule without removing it.
func (a *AppAPI) SetForwardRuleEnabled(connectionID, ruleID string, enabled bool) error {
	if a.forwardRuleVault == nil {
		return fmt.Errorf("forward rule service unavailable")
	}
	return a.forwardRuleVault.SetEnabled(context.Background(), connectionID, ruleID, enabled)
}
