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
	// AllowRemoteGateway opts a remote forward out of the loopback requirement on the SSH server.
	//
	// A remote forward bound to 0.0.0.0 exposes whatever TargetHost:TargetPort resolves to on THIS
	// machine to every host that can reach the server - and TargetHost is resolved locally, so
	// "127.0.0.1:22" means the user's own SSH daemon, published to the server's network. That can
	// be what someone wants, which is why it is a flag and not a prohibition, but it was reachable
	// by writing one string into a saved connection with nothing to acknowledge and nothing shown.
	//
	// Default false, so a rule injected through SaveConnection is refused by Validate rather than
	// quietly listening. Whether the server honours a non-loopback bind at all is its own
	// GatewayPorts setting; this decides only what this client is willing to ask for.
	AllowRemoteGateway bool `json:"allowRemoteGateway,omitempty"`
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
	if r.Kind == ForwardRuleLocal || r.Kind == ForwardRuleDynamic {
		if !IsLoopbackBind(r.BindAddress) {
			return fmt.Errorf("%w: %s forward bind address must be loopback", ErrInvalidForwardRule, r.Kind)
		}
	}
	// A remote forward is the one kind that can legitimately want a non-loopback bind, so it is
	// gated rather than refused - but it has to be asked for. Without this a rule written straight
	// into a saved connection published a local service to the server's whole network, with
	// nothing to acknowledge and nothing shown.
	if r.Kind == ForwardRuleRemote && !IsLoopbackBind(r.BindAddress) && !r.AllowRemoteGateway {
		return fmt.Errorf("%w: remote forward to %s exposes it beyond the server's loopback and must set allowRemoteGateway",
			ErrInvalidForwardRule, r.BindAddress)
	}
	return nil
}
