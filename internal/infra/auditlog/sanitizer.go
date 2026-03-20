package auditlog

import (
	"regexp"
	"strings"
)

var passwordPromptRe = regexp.MustCompile(`(?i)(password|passphrase|pass|secret|token)\s*[:\s]`)

const redactedPlaceholder = "***REDACTED***"

// Sanitizer checks terminal context and redacts password-like inputs.
type Sanitizer struct {
	recentOutput string
}

// NewSanitizer creates a Sanitizer with empty context.
func NewSanitizer() *Sanitizer {
	return &Sanitizer{}
}

// FeedOutput appends terminal output to the sanitizer's context buffer.
// Only the last 512 bytes are retained to keep memory bounded.
func (s *Sanitizer) FeedOutput(output string) {
	s.recentOutput += output
	if len(s.recentOutput) > 512 {
		s.recentOutput = s.recentOutput[len(s.recentOutput)-512:]
	}
}

// SanitizeInput checks whether recent terminal output contained a password prompt.
// If so, the input is replaced with a redaction marker.
// Returns the (potentially redacted) input and whether redaction occurred.
func (s *Sanitizer) SanitizeInput(input string) (string, bool) {
	if passwordPromptRe.MatchString(s.recentOutput) {
		trimmed := strings.TrimRight(input, "\r\n")
		if len(trimmed) > 0 {
			s.recentOutput = ""
			return redactedPlaceholder, true
		}
	}
	return input, false
}
