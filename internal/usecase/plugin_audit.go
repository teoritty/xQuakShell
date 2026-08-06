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

func NewPluginAuditWriter(repo domain.AuditLogRepository) *PluginAuditWriter {
	return &PluginAuditWriter{repo: repo}
}

func (w *PluginAuditWriter) RPCRecorder() domainplugin.AuditRecorder {
	return func(pluginID, method string, denied bool, detail string) {
		w.append(formatPluginRPCAuditLine(pluginID, method, denied, detail))
	}
}

func (w *PluginAuditWriter) StartFunc() PluginStartAuditFunc {
	return func(pluginID, reason, detail string, denied bool) {
		w.append(formatPluginStartAuditLine(pluginID, reason, detail, denied))
	}
}

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
	// Audit writes are detached on purpose. This runs from RPC paths that have a
	// context, but an audit record is the evidence that the operation happened -
	// dropping it because the operation was cancelled would delete exactly the
	// entries an investigation cares about.
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

func (w *PluginAuditWriter) SurfaceFunc() domainplugin.SurfaceAuditRecorder {
	return func(entry domainplugin.SurfaceAuditEntry) {
		w.append(formatPluginSurfaceAuditLine(entry))
	}
}

func formatPluginSurfaceAuditLine(entry domainplugin.SurfaceAuditEntry) string {
	flag := "allowed"
	if !entry.Success {
		flag = "denied"
	}
	// The plugin-authored title is deliberately absent. It is free text a plugin chose, and an
	// audit line is not a place to carry one — the ids below are what an incident review needs.
	line := "[plugin] action=" + domainplugin.SurfaceAuditOpen + " pluginId=" + entry.PluginID +
		" sessionId=" + entry.ParentSessionID +
		" connectionId=" + entry.ConnectionID +
		" surfaceId=" + safeAuditValue(entry.SurfaceID) +
		" kind=" + safeAuditValue(entry.Kind) +
		" result=" + flag
	if entry.Error != "" {
		line += " detail=" + safeAuditValue(domainplugin.RedactAuditDetail(entry.Error))
	}
	return line
}

func (w *PluginAuditWriter) DialogFunc() domainplugin.DialogAuditRecorder {
	return func(entry domainplugin.DialogAuditEntry) {
		w.append(formatPluginDialogAuditLine(entry))
	}
}

func formatPluginDialogAuditLine(entry domainplugin.DialogAuditEntry) string {
	flag := "allowed"
	if !entry.Success {
		flag = "denied"
	}
	// Field ids, never their values: a form field holds whatever the user typed into it, and an
	// audit line is the last place that belongs. The ids are plugin-authored, so they go through
	// the same tokenizer node ids do.
	line := "[plugin] action=" + domainplugin.DialogAuditSubmit + " pluginId=" + entry.PluginID +
		" dialogId=" + safeAuditValue(entry.DialogID) +
		" kind=" + safeAuditValue(entry.Kind) +
		" fields=" + joinAuditTokens(entry.FieldIDs) +
		" result=" + flag
	if entry.Error != "" {
		line += " detail=" + safeAuditValue(entry.Error)
	}
	return line
}

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
		" actionId=" + safeAuditValue(entry.ActionID) +
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
		line += " detail=" + safeAuditValue(entry.Error)
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
// safeAuditValue makes one plugin-authored value fit to write into an audit field: secrets removed
// first, structure neutralized second.
//
// THE ORDER IS LOAD-BEARING AND MUST NOT BE SWAPPED. RedactAuditDetail finds secrets by pattern —
// (password|token|secret|apikey|...) followed by ':' or '=' and the value — so it needs the value's
// original punctuation to recognize one. Tokenizing first rewrites '=' to '_', the pattern stops
// matching, and a plugin error of `auth failed, token=AKIA...` is written to the audit log in full.
// That failure is silent: nothing errors, the secret is simply there.
//
// Running redaction first costs the forgery defence nothing. Its output introduces no separators —
// "[REDACTED]" and the truncation marker contain neither a space nor an '=' — so whatever structure
// the plugin smuggled in is still present for logfmtToken to neutralize afterwards.
//
// Both properties are pinned together, on one value, by TestDiscoveryAuditRedactsSecretsAndStill-
// ResistsForgery: testing them apart is what lets a later change satisfy one and quietly drop the
// other.
func safeAuditValue(value string) string {
	return logfmtToken(domainplugin.RedactAuditDetail(value))
}

func joinAuditTokens(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	safe := make([]string, 0, len(ids))
	for _, id := range ids {
		safe = append(safe, safeAuditValue(id))
	}
	return strings.Join(safe, ",")
}

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
