package plugin

import "time"

// ChannelAuditEntry records a channel.open/channel.close event without secret values. Target
// carries the post-validation canonical target for tcp-relay, never the raw hint, so unvalidated
// plugin input cannot reach the audit log.
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
