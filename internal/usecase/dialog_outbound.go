package usecase

import (
	"context"
	"encoding/json"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Host->plugin dialog notification methods (ADR-015 §2). Exactly one of the two arrives for any
// dialog the host opened, which is what lets a plugin await an answer without a timeout of its own.
const (
	dialogSubmitMethod = "dialog.submit"
	dialogCancelMethod = "dialog.cancel"
)

// DialogNotifier delivers a dialog's answer to its owning plugin.
type DialogNotifier struct {
	notifier DiscoveryNotifier
}

// NewDialogNotifier wires the outbound half. The parameter is DiscoveryNotifier because that
// interface already means "send a host->plugin notification" and is satisfied by *PluginManager.
func NewDialogNotifier(notifier DiscoveryNotifier) *DialogNotifier {
	return &DialogNotifier{notifier: notifier}
}

// Submitted delivers the values the user entered.
func (n *DialogNotifier) Submitted(pluginID, dialogID string, values map[string]string) {
	if values == nil {
		// An explicit empty map, never null: a plugin reading the payload must not have to tell
		// "no values" from "the field was missing".
		values = map[string]string{}
	}
	n.send(pluginID, dialogSubmitMethod, map[string]any{
		"dialogId": dialogID,
		"values":   values,
	})
}

// Cancelled reports that the user closed the dialog without answering.
func (n *DialogNotifier) Cancelled(pluginID, dialogID string) {
	n.send(pluginID, dialogCancelMethod, map[string]string{"dialogId": dialogID})
}

// send marshals and dispatches, logging rather than returning failures: the dialog is already
// closed on the host side, and there is nothing a caller could do with the error.
func (n *DialogNotifier) send(pluginID, method string, payload any) {
	if n.notifier == nil {
		return
	}
	params, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("dialog: marshal notification failed", "method", method, "err", err)
		return
	}
	// A dialog notification reports something the UI has already done. The caller
	// is not waiting on the plugin's answer and there is nothing to undo if the
	// plugin never reads it, so this fires under no deadline but the transport's.
	if err := n.notifier.Notify(context.Background(), pluginID, method, params); err != nil {
		slog.Debug("dialog: notify failed", "method", method, "pluginId", pluginID, "err", err)
	}
}

var _ domainplugin.DialogOutboundPort = (*DialogNotifier)(nil)
