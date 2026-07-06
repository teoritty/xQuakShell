package wails

import (
	"context"
	"fmt"

	"ssh-client/internal/domain"
)

// ListForwardRules returns forward rules for a connection.
func (a *AppAPI) ListForwardRules(connectionID string) ([]domain.ForwardRule, error) {
	conn, err := a.vaultSvc.GetConnection(context.Background(), connectionID)
	if err != nil {
		return nil, err
	}
	return conn.ForwardRules, nil
}

// SaveForwardRule creates or updates a forward rule on a connection.
func (a *AppAPI) SaveForwardRule(connectionID string, rule domain.ForwardRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	conn, err := a.vaultSvc.GetConnection(context.Background(), connectionID)
	if err != nil {
		return err
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
	_, err = a.vaultSvc.SaveConnection(context.Background(), conn, conn.PluginFields)
	return err
}

// DeleteForwardRule removes a forward rule from a connection.
func (a *AppAPI) DeleteForwardRule(connectionID, ruleID string) error {
	conn, err := a.vaultSvc.GetConnection(context.Background(), connectionID)
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
		return fmt.Errorf("forward rule %q not found", ruleID)
	}
	conn.ForwardRules = next
	_, err = a.vaultSvc.SaveConnection(context.Background(), conn, conn.PluginFields)
	return err
}

// SetForwardRuleEnabled toggles a forward rule without removing it.
func (a *AppAPI) SetForwardRuleEnabled(connectionID, ruleID string, enabled bool) error {
	conn, err := a.vaultSvc.GetConnection(context.Background(), connectionID)
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
		return fmt.Errorf("forward rule %q not found", ruleID)
	}
	_, err = a.vaultSvc.SaveConnection(context.Background(), conn, conn.PluginFields)
	return err
}
