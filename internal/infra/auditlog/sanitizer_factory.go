package auditlog

import "xquakshell/internal/domain"

func SanitizerFactory() domain.AuditInputSanitizerFactory {
	return func() domain.AuditInputSanitizer {
		return NewSanitizer()
	}
}

var _ domain.AuditInputSanitizer = (*Sanitizer)(nil)
