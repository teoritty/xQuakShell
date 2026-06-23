package usecase

import (
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ActivationKind identifies why a plugin is being activated.
type ActivationKind string

const (
	ActivationStartup  ActivationKind = "startup"
	ActivationProtocol ActivationKind = "protocol"
	ActivationCommand  ActivationKind = "command"
	ActivationManual   ActivationKind = "manual"
	ActivationView     ActivationKind = "view"
)

// ActivationTrigger describes an activation event to match against manifest rules.
type ActivationTrigger struct {
	Kind  ActivationKind
	Value string
}

// MatchesActivation reports whether manifest activationEvents include the trigger.
func MatchesActivation(events []string, trigger ActivationTrigger) bool {
	for _, e := range events {
		switch trigger.Kind {
		case ActivationStartup:
			if e == "onStartup" {
				return true
			}
		case ActivationProtocol:
			if e == "onProtocol:"+trigger.Value {
				return true
			}
		case ActivationCommand:
			if e == "onCommand:"+trigger.Value {
				return true
			}
		case ActivationManual:
			if e == "onManual" {
				return true
			}
		case ActivationView:
			if e == "onView:*" {
				return true
			}
			if strings.HasPrefix(e, "onView:") && strings.TrimPrefix(e, "onView:") == trigger.Value {
				return true
			}
		}
	}
	return false
}

// PluginsForActivation returns installed plugins whose activationEvents match.
func (r *PluginRegistry) PluginsForActivation(trigger ActivationTrigger) []domainplugin.InstalledPlugin {
	var out []domainplugin.InstalledPlugin
	for _, p := range r.List() {
		if MatchesActivation(p.Manifest.ActivationEvents, trigger) {
			out = append(out, p)
		}
	}
	return out
}
