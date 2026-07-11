package plugin

import "time"

// ChannelAuditEntry records a channel.open/channel.close event without secret values. Target
// carries the raw hint in Stage 3 (no real backends exist yet); Stage 6 replaces it with the
// post-validation canonical target for tcp-relay so raw plugin input never reaches the audit log.
type ChannelAuditEntry struct {
	Timestamp       time.Time
	PluginID        string
	Action          string // "channel.open" | "channel.close"
	ChannelID       uint32
	Purpose         string
	ParentSessionID string
	Target          string
	Success         bool
	Error           string
}

// ChannelAuditRecorder records channel bus audit events.
type ChannelAuditRecorder func(entry ChannelAuditEntry)
