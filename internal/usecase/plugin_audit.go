package usecase

import (
	"context"
	"log"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginAuditWriter records plugin security events through the vault audit log port.
type PluginAuditWriter struct {
	repo domain.AuditLogRepository
}

// NewPluginAuditWriter creates a plugin audit writer.
func NewPluginAuditWriter(repo domain.AuditLogRepository) *PluginAuditWriter {
	return &PluginAuditWriter{repo: repo}
}

// RPCRecorder returns an audit callback for plugin→core RPC denials.
func (w *PluginAuditWriter) RPCRecorder() domainplugin.AuditRecorder {
	return func(pluginID, method string, denied bool, detail string) {
		w.append(formatPluginRPCAuditLine(pluginID, method, denied, detail))
	}
}

// StartFunc returns a start-authorization audit callback.
func (w *PluginAuditWriter) StartFunc() PluginStartAuditFunc {
	return func(pluginID, reason, detail string, denied bool) {
		w.append(formatPluginStartAuditLine(pluginID, reason, detail, denied))
	}
}

// OutboundAuthFunc returns an audit callback for host→plugin auth.* RPC calls.
func (w *PluginAuditWriter) OutboundAuthFunc() OutboundAuthAuditFunc {
	return func(pluginID, method, sanitizedParams string) {
		w.append(formatPluginOutboundAuthAuditLine(pluginID, method, sanitizedParams))
	}
}

func formatPluginOutboundAuthAuditLine(pluginID, method, sanitizedParams string) string {
	line := "[plugin] action=" + method + " pluginId=" + pluginID + " direction=outbound result=allowed"
	if sanitizedParams != "" && sanitizedParams != "null" {
		line += " detail=" + domainplugin.RedactAuditDetail(sanitizedParams)
	}
	return line
}

func (w *PluginAuditWriter) append(input string) {
	if w == nil || w.repo == nil || input == "" {
		return
	}
	entry := domain.AuditEntry{
		Timestamp: time.Now(),
		SessionID: "plugin",
		Input:     input,
	}
	if err := w.repo.Append(context.Background(), entry); err != nil {
		log.Printf("WARNING: plugin audit append failed: %v", err)
	}
}

func formatPluginRPCAuditLine(pluginID, method string, denied bool, detail string) string {
	flag := "allowed"
	if denied {
		flag = "denied"
	}
	line := "[plugin] action=" + method + " pluginId=" + pluginID + " result=" + flag
	if detail != "" {
		line += " detail=" + domainplugin.RedactAuditDetail(detail)
	}
	return line
}

func formatPluginStartAuditLine(pluginID, reason, detail string, denied bool) string {
	flag := "allowed"
	if denied {
		flag = "denied"
	}
	line := "[plugin] action=start pluginId=" + pluginID + " reason=" + reason + " result=" + flag
	if detail != "" {
		line += " detail=" + domainplugin.RedactAuditDetail(detail)
	}
	return line
}

// ChannelFunc returns a channel.open/channel.close audit callback.
func (w *PluginAuditWriter) ChannelFunc() domainplugin.ChannelAuditRecorder {
	return func(entry domainplugin.ChannelAuditEntry) {
		w.append(formatPluginChannelAuditLine(entry))
	}
}

func formatPluginChannelAuditLine(entry domainplugin.ChannelAuditEntry) string {
	flag := "allowed"
	if !entry.Success {
		flag = "denied"
	}
	line := "[plugin] action=" + entry.Action + " pluginId=" + entry.PluginID + " sessionId=" + entry.ParentSessionID + " purpose=" + entry.Purpose + " result=" + flag
	if entry.Target != "" {
		line += " target=" + domainplugin.RedactAuditDetail(entry.Target)
	}
	if entry.Error != "" {
		line += " detail=" + domainplugin.RedactAuditDetail(entry.Error)
	}
	return line
}

// SessionBindFunc returns a session bind/unbind audit callback.
func (w *PluginAuditWriter) SessionBindFunc() SessionBindAuditFunc {
	return func(pluginID, sessionID, action string, allowed bool, detail string) {
		w.append(formatPluginSessionBindAuditLine(pluginID, sessionID, action, allowed, detail))
	}
}

func formatPluginSessionBindAuditLine(pluginID, sessionID, action string, allowed bool, detail string) string {
	flag := "allowed"
	if !allowed {
		flag = "denied"
	}
	line := "[plugin] action=session." + action + " pluginId=" + pluginID + " sessionId=" + sessionID + " result=" + flag
	if detail != "" {
		line += " detail=" + domainplugin.RedactAuditDetail(detail)
	}
	return line
}
