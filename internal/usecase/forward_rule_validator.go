package usecase

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

type ForwardRuleManifestLookup interface {
	HasTunnelProvider(pluginID string) (bool, error)
	TunnelProviderExists(pluginID, providerID string) (bool, error)
}

type ForwardRuleGrantReader interface {
	IsTunnelProviderGranted(pluginID string) bool
}

type ForwardRuleValidator struct {
	connRepo domain.ConnectionRepository
	lookup   ForwardRuleManifestLookup
	grants   ForwardRuleGrantReader
}

func NewForwardRuleValidator(connRepo domain.ConnectionRepository, lookup ForwardRuleManifestLookup, grants ForwardRuleGrantReader) *ForwardRuleValidator {
	return &ForwardRuleValidator{connRepo: connRepo, lookup: lookup, grants: grants}
}

// ValidateRule checks structural, manifest, and consent constraints for one rule.
func (v *ForwardRuleValidator) ValidateRule(rule domain.ForwardRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	if rule.Kind != domain.ForwardRuleDynamic {
		return nil
	}
	if v.lookup == nil {
		return fmt.Errorf("forward rule validation unavailable")
	}
	hasProvider, err := v.lookup.HasTunnelProvider(rule.PluginID)
	if err != nil {
		return fmt.Errorf("forward rule plugin %q: %w", rule.PluginID, err)
	}
	if !hasProvider {
		return fmt.Errorf("%w: plugin %q is not a tunnel provider", domain.ErrInvalidForwardRule, rule.PluginID)
	}
	ok, err := v.lookup.TunnelProviderExists(rule.PluginID, rule.ProviderID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: unknown tunnel provider %q for plugin %q", domain.ErrInvalidForwardRule, rule.ProviderID, rule.PluginID)
	}
	if v.grants != nil && !v.grants.IsTunnelProviderGranted(rule.PluginID) {
		return fmt.Errorf("tunnel provider access not granted for plugin %q", rule.PluginID)
	}
	return nil
}

// ValidateRuleIDUnique ensures ruleID is not used by another connection in the vault.
func (v *ForwardRuleValidator) ValidateRuleIDUnique(ctx context.Context, ruleID, connectionID string) error {
	if v == nil || v.connRepo == nil || ruleID == "" {
		return nil
	}
	conns, err := v.connRepo.GetAllConnections(ctx)
	if err != nil {
		return err
	}
	for _, conn := range conns {
		if conn.ID == connectionID {
			continue
		}
		for _, r := range conn.ForwardRules {
			if r.ID == ruleID {
				return fmt.Errorf("%w: forward rule id %q already used by connection %q", domain.ErrInvalidForwardRule, ruleID, conn.ID)
			}
		}
	}
	return nil
}

// ValidateConnectionRules checks duplicate rule IDs within one connection.
func ValidateConnectionRules(rules []domain.ForwardRule) error {
	seen := make(map[string]struct{}, len(rules))
	for _, r := range rules {
		if r.ID == "" {
			continue
		}
		if _, dup := seen[r.ID]; dup {
			return fmt.Errorf("%w: duplicate forward rule id %q", domain.ErrInvalidForwardRule, r.ID)
		}
		seen[r.ID] = struct{}{}
	}
	return nil
}

// ValidateRuleForConnect validates an enabled rule before session start.
func (v *ForwardRuleValidator) ValidateRuleForConnect(rule domain.ForwardRule) error {
	if err := v.ValidateRule(rule); err != nil {
		return err
	}
	if rule.Kind == domain.ForwardRuleDynamic && v.grants == nil {
		return domainplugin.ErrCapabilityDenied
	}
	return nil
}
