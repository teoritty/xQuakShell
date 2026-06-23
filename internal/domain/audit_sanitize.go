package domain

// AuditInputSanitizer redacts password-like terminal input based on recent output context.
type AuditInputSanitizer interface {
	FeedOutput(output string)
	SanitizeInput(input string) (string, bool)
}

// AuditInputSanitizerFactory creates a per-session audit input sanitizer.
type AuditInputSanitizerFactory func() AuditInputSanitizer
