package wails

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"ssh-client/internal/domain"
	"ssh-client/internal/usecase"
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
	if a.forwardRules != nil {
		if err := a.forwardRules.ValidateRule(rule); err != nil {
			return err
		}
	}
	if rule.ID == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return fmt.Errorf("generate forward rule id: %w", err)
		}
		rule.ID = hex.EncodeToString(buf)
	}
	if a.forwardRules != nil {
		if err := a.forwardRules.ValidateRuleIDUnique(context.Background(), rule.ID, connectionID); err != nil {
			return err
		}
	}
	rule.BindAddress = domain.EffectiveBindAddress(rule.BindAddress)
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
	if err := usecase.ValidateConnectionRules(conn.ForwardRules); err != nil {
		return err
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
