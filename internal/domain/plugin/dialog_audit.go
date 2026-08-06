package plugin

import "time"

// DialogAuditEntry records one answered dialog (ADR-015 §Security model).
//
// A submit is audited and an open is not, which is the opposite of a surface: opening a modal
// shows the user a question, while submitting one hands a plugin an answer the user gave. The
// second is the point where something crosses from the user to the plugin, and it is the one an
// incident review needs.
//
// FieldIDs, never values. A form field holds whatever the user typed into it — a path, a name, a
// passphrase they were not supposed to put there — and §2.1 of the project's rules puts secrets
// out of the audit log unconditionally. Which fields were answered is enough to reconstruct what
// was asked; what was answered is the plugin's business and stays out of the log.
type DialogAuditEntry struct {
	Timestamp time.Time
	PluginID  string
	DialogID  string
	Kind      string
	FieldIDs  []string
	Success   bool
	Error     string
}

// DialogAuditSubmit names the audited action, matching the surface and discovery vocabulary.
const DialogAuditSubmit = "dialog.submit"

// DialogAuditRecorder records dialog audit events.
type DialogAuditRecorder func(entry DialogAuditEntry)
