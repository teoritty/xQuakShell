package plugin

import "time"

// SurfaceAuditEntry records one surface.open (ADR-015 §Security model).
//
// Only open is audited. It is the moment a plugin claims a tab in the user's window on the
// authority of a session it borrowed, and it is the one surface verb whose absence from the log
// would leave no trace that the tab was ever the plugin's doing. Writes are not audited for the
// reason discovery does not audit publishes: they are a stream, and burying the entries that
// describe a claim under the ones that describe traffic helps nobody reading afterwards.
type SurfaceAuditEntry struct {
	Timestamp       time.Time
	PluginID        string
	SurfaceID       string
	ParentSessionID string
	ConnectionID    string
	Kind            string
	Success         bool
	Error           string
}

// SurfaceAuditOpen names the audited action, matching the discovery vocabulary's shape.
const SurfaceAuditOpen = "surface.open"

// SurfaceAuditRecorder records surface audit events.
type SurfaceAuditRecorder func(entry SurfaceAuditEntry)
