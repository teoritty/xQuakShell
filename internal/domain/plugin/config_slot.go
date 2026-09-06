package plugin

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// ErrInvalidPluginConfig indicates a user-supplied configuration value the core refuses to use.
//
// It is separate from ErrInvalidManifest because the two are answered by different people: a
// malformed manifest is the plugin author's problem and stops the install, while this is something
// the user typed and can correct in the settings panel.
var ErrInvalidPluginConfig = errors.New("invalid plugin configuration value")

// SlotKindEndpoint is the only slot kind: a network destination the user supplies.
//
// There is deliberately no free-text kind. Every value stored here becomes part of a security
// decision - what the plugin is allowed to dial - and a kind the core cannot validate would be a
// value it cannot reason about.
const SlotKindEndpoint = "endpoint"

// endpointSchemePorts is the closed set of schemes a slot may declare, with the port each implies
// when the user does not type one. A scheme with no known default port could not produce a dial
// pattern, so the set and the defaults are the same table.
var endpointSchemePorts = map[string]int{
	"http":  80,
	"https": 443,
	"ws":    80,
	"wss":   443,
}

// ConfigCaps declares the configuration a plugin needs from the user (ADR-022).
type ConfigCaps struct {
	Slots []ConfigSlot `json:"slots,omitempty"`
}

// ConfigSlot declares a value the user must supply, never the value itself.
//
// The address of a self-hosted server cannot be known when a bundle is built, so without slots a
// plugin whose destination only its user knows has to request allowArbitraryOutbound - the widest
// grant in the system, demanded by precisely the plugins that handle credentials.
type ConfigSlot struct {
	ID string `json:"id"`
	// Label is shown beside the field. It is plugin-authored text and is sanitized before display.
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Schemes lists what the plugin accepts. It is required for an endpoint: the scheme decides the
	// default port, and "whatever the user types" is not a protocol a plugin author can support.
	Schemes  []string `json:"schemes,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// PluginConfig holds the values a user supplied, keyed by slot id.
//
// It lives in the vault beside the rest of the settings and never inside a bundle. A server address
// is configuration and is visible; a token for that server is a secret and belongs in
// VaultData.PluginSecrets instead. Different lifetimes, different visibility, different storage.
type PluginConfig struct {
	Values map[string]string `json:"values,omitempty"`
}

// validateConfigCaps checks the slot declarations a manifest carries.
//
// Slots are validated when the manifest loads rather than when a value is typed: a plugin whose
// declarations are malformed cannot be configured correctly by anyone, so it must not install.
func (m *Manifest) validateConfigCaps() error {
	if m.Capabilities.Config == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(m.Capabilities.Config.Slots))
	for _, slot := range m.Capabilities.Config.Slots {
		if err := validateConfigSlot(slot); err != nil {
			return err
		}
		if _, duplicate := seen[slot.ID]; duplicate {
			return fmt.Errorf("%w: duplicate configuration slot %q", ErrInvalidManifest, slot.ID)
		}
		seen[slot.ID] = struct{}{}
	}
	return nil
}

func validateConfigSlot(slot ConfigSlot) error {
	if strings.TrimSpace(slot.ID) == "" {
		return fmt.Errorf("%w: configuration slot needs an id", ErrInvalidManifest)
	}
	if strings.TrimSpace(slot.Label) == "" {
		return fmt.Errorf("%w: configuration slot %q needs a label", ErrInvalidManifest, slot.ID)
	}
	if slot.Kind != SlotKindEndpoint {
		return fmt.Errorf("%w: configuration slot %q has unknown kind %q", ErrInvalidManifest, slot.ID, slot.Kind)
	}
	if len(slot.Schemes) == 0 {
		return fmt.Errorf("%w: endpoint slot %q declares no scheme", ErrInvalidManifest, slot.ID)
	}
	for _, scheme := range slot.Schemes {
		if _, known := endpointSchemePorts[scheme]; !known {
			return fmt.Errorf("%w: endpoint slot %q declares unsupported scheme %q", ErrInvalidManifest, slot.ID, scheme)
		}
	}
	return nil
}

// EffectiveOutbound returns the dial allowlist: the manifest's own patterns plus the endpoints the
// user filled in.
//
// The result feeds the existing network gate unchanged, so it inherits resolve-then-dial and the
// explicit consent private and loopback addresses already require - the slot adds a destination,
// it does not add a way around the checks that destination is subject to.
//
// A value for a slot the manifest no longer declares is ignored rather than honoured: a plugin
// update that drops a slot must stop reaching the host that slot used to name.
func EffectiveOutbound(m *Manifest, cfg PluginConfig) ([]string, error) {
	if m == nil {
		return nil, nil
	}
	var patterns []string
	if m.Capabilities.Network != nil {
		patterns = append(patterns, m.Capabilities.Network.Outbound...)
	}
	if m.Capabilities.Config == nil {
		return patterns, nil
	}
	for _, slot := range m.Capabilities.Config.Slots {
		value, filled := cfg.Values[slot.ID]
		if !filled || strings.TrimSpace(value) == "" {
			continue
		}
		pattern, err := endpointPattern(slot, value)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// endpointPattern turns one user-supplied endpoint into the tcp:host:port form the gate parses.
//
// A refusal here is deliberate rather than a silent skip. A value that cannot become a pattern is a
// destination the plugin will fail to reach, and failing to dial for a reason nobody can see is
// worse than saying which value is wrong.
func endpointPattern(slot ConfigSlot, value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("%w: %q is not a URL", ErrInvalidPluginConfig, value)
	}
	if !slices.Contains(slot.Schemes, parsed.Scheme) {
		return "", fmt.Errorf("%w: slot %q does not accept scheme %q", ErrInvalidPluginConfig, slot.ID, parsed.Scheme)
	}
	// Credentials in a configuration value would be shown in the settings panel, written to the
	// audit trail and carried in every log line naming the endpoint. A secret belongs in
	// PluginSecrets, and refusing here is what keeps the two apart.
	if parsed.User != nil {
		return "", fmt.Errorf("%w: slot %q must not carry credentials", ErrInvalidPluginConfig, slot.ID)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("%w: slot %q has no host", ErrInvalidPluginConfig, slot.ID)
	}
	// The pattern format splits on its first colon, so it cannot express an IPv6 literal at all.
	// Deriving one anyway would produce a pattern that parses into a different host than the user
	// typed, which is the one outcome a network allowlist must never have.
	if strings.Contains(host, ":") {
		return "", fmt.Errorf("%w: slot %q cannot take an IPv6 address", ErrInvalidPluginConfig, slot.ID)
	}
	port, err := endpointPort(parsed, slot)
	if err != nil {
		return "", err
	}
	return "tcp:" + host + ":" + strconv.Itoa(port), nil
}

func endpointPort(parsed *url.URL, slot ConfigSlot) (int, error) {
	if parsed.Port() == "" {
		return endpointSchemePorts[parsed.Scheme], nil
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%w: slot %q has an invalid port %q", ErrInvalidPluginConfig, slot.ID, parsed.Port())
	}
	return port, nil
}
