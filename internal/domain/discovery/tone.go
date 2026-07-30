package discovery

import "regexp"

// Tone is the semantic color category a plugin assigns to a node's Status. It is a closed
// vocabulary, not a free-form CSS color, so the frontend can theme it consistently (light/dark,
// accessible contrast) without trusting a plugin-supplied palette.
type Tone string

const (
	ToneOK      Tone = "ok"
	ToneWarn    Tone = "warn"
	ToneError   Tone = "error"
	ToneBusy    Tone = "busy"
	ToneNeutral Tone = "neutral"
	ToneUnknown Tone = "unknown"
)

// Status is the plugin's opinion about one node's health, rendered as a colored dot with an
// optional tooltip. It is filled ONLY by the plugin — see the provenance note on Node.Status
// and on Branch, its host-owned counterpart.
type Status struct {
	Tone    Tone   `json:"tone"`
	Color   string `json:"color,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// hexColorPattern matches a strict 6-digit hex color (#RRGGBB). Compiled once at package init
// rather than per call: ValidatePublish runs this over every node of every snapshot, and a
// plugin publishing hundreds of nodes at MaxPublishPerSecond makes per-call compilation a
// real, avoidable cost.
var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidTone reports whether t is one of the known Tone values.
func ValidTone(t Tone) bool {
	switch t {
	case ToneOK, ToneWarn, ToneError, ToneBusy, ToneNeutral, ToneUnknown:
		return true
	default:
		return false
	}
}

// ValidColor reports whether c is acceptable as Status.Color: empty (no override — the
// frontend picks a color from Tone) or a strict 6-digit hex color. Three-digit shorthand and
// alpha channels are deliberately not accepted: one canonical form keeps the frontend parser
// trivial and closes off CSS-injection-flavored surprises in a plugin-supplied string.
func ValidColor(c string) bool {
	if c == "" {
		return true
	}
	return hexColorPattern.MatchString(c)
}
