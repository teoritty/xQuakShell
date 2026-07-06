package domain

import (
	"errors"
	"fmt"
)

type ForwardRuleKind string

const (
	ForwardRuleLocal   ForwardRuleKind = "local"
	ForwardRuleRemote  ForwardRuleKind = "remote"
	ForwardRuleDynamic ForwardRuleKind = "dynamic"
)

// ErrInvalidForwardRule indicates a forward rule failed validation.
var ErrInvalidForwardRule = errors.New("invalid forward rule")

// ForwardRule configures one port-forwarding tunnel attached to a Connection.
type ForwardRule struct {
	ID          string          `json:"id"`
	Kind        ForwardRuleKind `json:"kind"`
	BindAddress string          `json:"bindAddress"`
	BindPort    int             `json:"bindPort"`
	TargetHost  string          `json:"targetHost,omitempty"`
	TargetPort  int             `json:"targetPort,omitempty"`
	PluginID    string          `json:"pluginId,omitempty"`
	ProviderID  string          `json:"providerId,omitempty"`
	Enabled     bool            `json:"enabled"`
}

func (r ForwardRule) Validate() error {
	if r.BindPort <= 0 || r.BindPort > 65535 {
		return fmt.Errorf("%w: invalid bind port %d", ErrInvalidForwardRule, r.BindPort)
	}
	switch r.Kind {
	case ForwardRuleLocal, ForwardRuleRemote:
		if r.TargetHost == "" || r.TargetPort <= 0 {
			return fmt.Errorf("%w: local/remote forward requires targetHost/targetPort", ErrInvalidForwardRule)
		}
	case ForwardRuleDynamic:
		if r.PluginID == "" || r.ProviderID == "" {
			return fmt.Errorf("%w: dynamic forward requires pluginId/providerId", ErrInvalidForwardRule)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidForwardRule, r.Kind)
	}
	return nil
}
