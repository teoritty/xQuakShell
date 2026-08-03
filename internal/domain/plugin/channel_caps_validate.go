package plugin

import (
	"fmt"
	"strings"
)

func isKnownChannelPurpose(purpose string) bool {
	switch purpose {
	case PurposeExec, PurposeEmbedStream, PurposeTCPRelay, PurposeUDPRelay:
		return true
	default:
		return false
	}
}

func (m *Manifest) validateChannelCaps() error {
	c := m.Capabilities.Channel
	if c == nil {
		return nil
	}
	purposes := make(map[string]struct{}, len(c.Purposes))
	for _, p := range c.Purposes {
		p = strings.TrimSpace(p)
		if !isKnownChannelPurpose(p) {
			return fmt.Errorf("%w: unknown channel purpose %q", ErrInvalidManifest, p)
		}
		purposes[p] = struct{}{}
	}
	if len(c.ExecCommands) > 0 {
		if _, ok := purposes[PurposeExec]; !ok {
			return fmt.Errorf("%w: channel.execCommands requires exec in channel.purposes", ErrInvalidManifest)
		}
		for _, tmpl := range c.ExecCommands {
			if err := validateExecCommandTemplate(tmpl); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExecCommandTemplate(tmpl ExecCommandTemplate) error {
	if len(tmpl.Argv) == 0 {
		return fmt.Errorf("%w: channel.execCommands entry requires a non-empty argv", ErrInvalidManifest)
	}
	for _, arg := range tmpl.Argv {
		placeholder, ok := extractPlaceholder(arg)
		if !ok {
			continue
		}
		pattern, ok := tmpl.Params[placeholder]
		if !ok {
			return fmt.Errorf("%w: argv placeholder %q has no corresponding params regex", ErrInvalidManifest, placeholder)
		}
		if _, err := validateRegexPatternSafe(pattern); err != nil {
			return fmt.Errorf("%w: invalid params regex for %q: %v", ErrInvalidManifest, placeholder, err)
		}
	}
	return nil
}

// extractPlaceholder reports whether arg is exactly a "{name}" placeholder and returns name.
func extractPlaceholder(arg string) (string, bool) {
	if len(arg) < 3 || arg[0] != '{' || arg[len(arg)-1] != '}' {
		return "", false
	}
	return arg[1 : len(arg)-1], true
}
