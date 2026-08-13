package domain

import (
	"errors"
	"testing"
)

func remoteRule(bindAddress string, allowGateway bool) ForwardRule {
	return ForwardRule{
		ID:                 "r1",
		Kind:               ForwardRuleRemote,
		BindAddress:        bindAddress,
		BindPort:           8080,
		TargetHost:         "127.0.0.1",
		TargetPort:         22,
		AllowRemoteGateway: allowGateway,
	}
}

// The attack this closes: a rule written into a saved connection with bindAddress "0.0.0.0" and a
// target of 127.0.0.1:22 publishes the user's own SSH daemon to every host that can reach the
// server. TargetHost is resolved on this machine, so "local" targets are local to the victim.
// Nothing acknowledged it and nothing showed it.
func TestForwardRuleRemoteGatewayNeedsAnExplicitOptIn(t *testing.T) {
	for _, bind := range []string{"0.0.0.0", "::", "10.0.0.5", "example.internal"} {
		t.Run(bind, func(t *testing.T) {
			err := remoteRule(bind, false).Validate()
			if !errors.Is(err, ErrInvalidForwardRule) {
				t.Fatalf("Validate() = %v, want ErrInvalidForwardRule; %q reaches beyond the server's loopback", err, bind)
			}
		})
	}
}

// It is a gate, not a prohibition: publishing a service through a bastion is a real thing to want.
func TestForwardRuleRemoteGatewayIsAllowedWhenAskedFor(t *testing.T) {
	if err := remoteRule("0.0.0.0", true).Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil once allowRemoteGateway is set", err)
	}
}

// The common case must stay free of the flag, or every ordinary remote forward starts failing.
func TestForwardRuleRemoteLoopbackNeedsNoOptIn(t *testing.T) {
	for _, bind := range []string{"", "127.0.0.1", "localhost", "::1"} {
		t.Run(bind, func(t *testing.T) {
			if err := remoteRule(bind, false).Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil; %q is loopback", err, bind)
			}
		})
	}
}

// AllowRemoteGateway is scoped to remote forwards. A local or dynamic forward stays loopback-only
// whatever the flag says - those listen on the user's own machine, and there is no second party
// whose policy could make a wildcard bind reasonable.
func TestForwardRuleGatewayFlagDoesNotUnlockLocalOrDynamic(t *testing.T) {
	local := ForwardRule{
		ID: "r1", Kind: ForwardRuleLocal, BindAddress: "0.0.0.0", BindPort: 8080,
		TargetHost: "10.0.0.9", TargetPort: 80, AllowRemoteGateway: true,
	}
	if err := local.Validate(); !errors.Is(err, ErrInvalidForwardRule) {
		t.Errorf("local forward Validate() = %v, want ErrInvalidForwardRule", err)
	}

	dynamic := ForwardRule{
		ID: "r2", Kind: ForwardRuleDynamic, BindAddress: "0.0.0.0", BindPort: 1080,
		PluginID: "p", ProviderID: "prov", AllowRemoteGateway: true,
	}
	if err := dynamic.Validate(); !errors.Is(err, ErrInvalidForwardRule) {
		t.Errorf("dynamic forward Validate() = %v, want ErrInvalidForwardRule", err)
	}
}
