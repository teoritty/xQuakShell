package plugin

import "time"

// DiscoveryAuditEntry records one discovery.invokeAction, the only discovery verb that changes
// anything on the remote host (ADR-014 "Security model").
//
// NodeIDs is the FULL selection, never a count and never a sample. An action is opaque to the core:
// once it has been relayed, the audit log is the only record of what a mass invocation was aimed
// at, and "stopped 200 nodes" is useless to whoever has to work out afterwards which 200.
//
// publish and observe are absent by design. A publish is data flowing inward, gated and rate
// limited and already visible as a tree; auditing every one would bury the entries that describe an
// actual effect on the user's machine under a stream that describes none.
type DiscoveryAuditEntry struct {
	Timestamp    time.Time
	PluginID     string
	ConnectionID string
	SessionID    string
	NodeIDs      []string
	ActionID     string
	Success      bool
	Error        string
}

// DiscoveryAuditRecorder records discovery audit events.
type DiscoveryAuditRecorder func(entry DiscoveryAuditEntry)
