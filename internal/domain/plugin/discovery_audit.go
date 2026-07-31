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
	Timestamp time.Time
	// Action names the PHASE, not just the verb: an invocation produces one entry when it is
	// dispatched and a second when its outcome is known, and the two are otherwise identical in
	// every field a reader could sort on. Without the phase, a failed action looks like two
	// contradictory records of one event rather than a dispatch and its result.
	Action       string
	PluginID     string
	ConnectionID string
	SessionID    string
	NodeIDs      []string
	ActionID     string
	Success      bool
	Error        string
}

// Discovery audit actions. The dispatch entry is written BEFORE the plugin is called: an action
// that arrived and then timed out has still been dispatched, and an entry written only on success
// would omit exactly the invocations an incident review is looking for.
const (
	DiscoveryAuditDispatch = "discovery.invokeAction.dispatch"
	DiscoveryAuditResult   = "discovery.invokeAction.result"
)

// DiscoveryAuditRecorder records discovery audit events.
type DiscoveryAuditRecorder func(entry DiscoveryAuditEntry)
