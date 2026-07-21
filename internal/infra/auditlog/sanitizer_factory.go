package auditlog

import "xquakshell/internal/domain"

// SanitizerFactory returns a factory for domain audit input sanitizers.
func SanitizerFactory() domain.AuditInputSanitizerFactory {
	return func() domain.AuditInputSanitizer {
		return NewSanitizer()
	}
}

var _ domain.AuditInputSanitizer = (*Sanitizer)(nil)
