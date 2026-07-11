package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// ExecChannelRequest is the JSON shape carried in a channel.open hint for the exec purpose:
// which declared argv template to run (index into capabilities.channel.execCommands) and the
// placeholder values to substitute into it. The plugin never supplies a free argv or command
// string — only a template selection and named parameter values.
type ExecChannelRequest struct {
	Template int               `json:"template"`
	Params   map[string]string `json:"params"`
}

// MatchExecCommand matches hint (a JSON-encoded ExecChannelRequest) against the manifest's
// declared exec argv templates and returns the concrete argv to run. Literal argv elements pass
// through unchanged; {placeholder} elements are substituted from the request's params only after
// the value is checked against the template's params regex for that placeholder — the same
// regex channel_caps_validate.go already proved is compilable and ReDoS-safe at manifest-load
// time; this is its first application to an actual runtime value. Returns ErrCapabilityDenied if
// the request selects no declared template, omits a required param, or a param value fails its
// regex — never a partial/best-effort argv.
func MatchExecCommand(templates []ExecCommandTemplate, hint string) ([]string, error) {
	var req ExecChannelRequest
	if err := json.Unmarshal([]byte(hint), &req); err != nil {
		return nil, fmt.Errorf("%w: invalid exec channel request: %v", ErrCapabilityDenied, err)
	}
	if req.Template < 0 || req.Template >= len(templates) {
		return nil, fmt.Errorf("%w: exec template %d not declared", ErrCapabilityDenied, req.Template)
	}
	tmpl := templates[req.Template]

	argv := make([]string, len(tmpl.Argv))
	for i, arg := range tmpl.Argv {
		placeholder, ok := extractPlaceholder(arg)
		if !ok {
			argv[i] = arg
			continue
		}
		value, present := req.Params[placeholder]
		if !present {
			return nil, fmt.Errorf("%w: missing param %q", ErrCapabilityDenied, placeholder)
		}
		pattern, ok := tmpl.Params[placeholder]
		if !ok {
			// Manifest-load validation (channel_caps_validate.go) guarantees every placeholder
			// has a params regex; this only guards against that invariant being violated.
			return nil, fmt.Errorf("%w: placeholder %q has no validator", ErrCapabilityDenied, placeholder)
		}
		matched, err := matchesFullString(pattern, value)
		if err != nil || !matched {
			return nil, fmt.Errorf("%w: param %q failed validation", ErrCapabilityDenied, placeholder)
		}
		argv[i] = value
	}
	return argv, nil
}

// matchesFullString reports whether value matches pattern over its entire length, not merely a
// substring — a template regex like "^[a-z]+$" already anchors, but a runtime match must not
// trust every manifest author to anchor correctly, so the match is checked to span [0, len(value))
// regardless of whether the pattern itself is anchored.
func matchesFullString(pattern, value string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	loc := re.FindStringIndex(value)
	if loc == nil {
		return false, nil
	}
	return loc[0] == 0 && loc[1] == len(value), nil
}
