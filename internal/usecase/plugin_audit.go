package usecase

import (
	"context"
	"log"
	"strings"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
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
		Category:  domain.AuditCategorySystem,
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

// DiscoveryFunc returns a discovery.invokeAction audit callback.
func (w *PluginAuditWriter) DiscoveryFunc() domainplugin.DiscoveryAuditRecorder {
	return func(entry domainplugin.DiscoveryAuditEntry) {
		w.append(formatPluginDiscoveryAuditLine(entry))
	}
}

func formatPluginDiscoveryAuditLine(entry domainplugin.DiscoveryAuditEntry) string {
	flag := "allowed"
	if !entry.Success {
		flag = "denied"
	}
	line := "[plugin] action=" + entry.Action + " pluginId=" + entry.PluginID +
		" sessionId=" + entry.SessionID +
		" connectionId=" + entry.ConnectionID +
		" actionId=" + domainplugin.RedactAuditDetail(logfmtToken(entry.ActionID)) +
		" nodeIds=" + joinAuditTokens(entry.NodeIDs) +
		" result=" + flag
	if entry.Error != "" {
		// detail= gets the same treatment as the id fields, and for the same reason: the host's own
		// error strings interpolate plugin-chosen ids ("node %q has no action %q"), so a value that
		// never touched an id field can still arrive carrying one. The shared detail= convention in
		// channel_audit/session_audit is untouched — this formatter hardens its own field rather
		// than changing a form three call sites depend on.
		//
		// Sanitizing rather than relocating the field is the point: detail= happens to sit after
		// result= today, so a first-occurrence parser reads the host's verdict. That is field order,
		// not a defence — a logfmt reader where the last occurrence wins would read the forgery.
		line += " detail=" + domainplugin.RedactAuditDetail(logfmtToken(entry.Error))
	}
	return line
}

// joinAuditTokens renders plugin-chosen identifiers into one audit field without letting them forge
// a second field.
//
// Node and action IDs are entirely plugin-authored: discovery validation bounds their length and
// refuses empties, and SanitizeNode cleans Label and Tooltip — but not ID, because an ID is a key
// the plugin must be able to match against its own bookkeeping, not a display string. See
// logfmtToken for what an unfiltered one does to a `key=value` line.
//
// The whole list is still written — an incident review needs to know which 200 nodes an action hit,
// and truncating that to fit a line-length budget would defeat the entry's only purpose. Redaction
// runs per ID rather than over the join for the same reason: its 512-character cut would otherwise
// land in the middle of the list.
func joinAuditTokens(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	safe := make([]string, 0, len(ids))
	for _, id := range ids {
		safe = append(safe, domainplugin.RedactAuditDetail(logfmtToken(id)))
	}
	return strings.Join(safe, ",")
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
