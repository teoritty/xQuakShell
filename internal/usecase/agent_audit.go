package usecase

import (
	"context"
	"log/slog"
	"strings"

	"xquakshell/internal/domain"
)

// Agent forwarding audit events (ADR-022 §7).
const (
	AgentEventForwardStarted = "agent.forward.started"
	AgentEventSignAllowed    = "agent.sign.allowed"
	AgentEventSignDenied     = "agent.sign.denied"
	AgentEventOpRefused      = "agent.op.refused"
)

// AgentAuditEvent is one forwarded-agent decision. Reason and Operation are fixed tokens chosen
// by this package; the remaining fields are the user's own data or validated remote input.
type AgentAuditEvent struct {
	Event          string
	Session        AgentSessionInfo
	KeyFingerprint string
	RemoteUser     string
	Destination    string
	Verified       bool
	Reason         string
}

// AgentAuditRecorder writes forwarded-agent decisions to the audit log.
//
// It records who asked, for which key and where the signature goes — fingerprints only. Never the
// data that was signed, never the signature: either would let anyone reading the log replay or
// study what the agent produced.
type AgentAuditRecorder struct {
	repo domain.AuditLogRepository
}

// NewAgentAuditRecorder returns a recorder writing to repo. A nil repo drops events, for the same
// reason as KeyAuditRecorder: a degraded log must not disable the feature it describes.
func NewAgentAuditRecorder(repo domain.AuditLogRepository) *AgentAuditRecorder {
	return &AgentAuditRecorder{repo: repo}
}

// Record appends one event; a failed write is logged and swallowed so the decision it describes
// still stands.
func (r *AgentAuditRecorder) Record(ctx context.Context, e AgentAuditEvent) {
	if r == nil || r.repo == nil {
		return
	}
	entry := domain.AuditEntry{
		Category:       domain.AuditCategorySystem,
		SessionID:      e.Session.SessionID,
		ConnectionID:   e.Session.ConnectionID,
		ConnectionName: e.Session.ConnectionName,
		Host:           e.Session.Host,
		Username:       e.Session.Username,
		Input:          agentAuditLine(e),
	}
	if err := r.repo.Append(ctx, entry); err != nil {
		slog.Warn("agent audit write failed", "event", e.Event, "session", e.Session.SessionID, "err", err)
	}
}

func agentAuditLine(e AgentAuditEvent) string {
	parts := []string{e.Event}
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	add("key", e.KeyFingerprint)
	add("remote-user", e.RemoteUser)
	add("destination", e.Destination)
	if e.Destination != "" || e.Event == AgentEventSignAllowed || e.Event == AgentEventSignDenied {
		if e.Verified {
			parts = append(parts, "verified")
		} else {
			parts = append(parts, "unverified")
		}
	}
	add("reason", e.Reason)
	return strings.Join(parts, " ")
}
