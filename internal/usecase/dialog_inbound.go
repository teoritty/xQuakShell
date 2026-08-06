package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Dialog RPC method names, in one place so the gate, the dispatcher and the tests cannot drift.
const (
	MethodDialogOpen     = "dialog.open"
	MethodDialogSetError = "dialog.setError"
	MethodDialogClose    = "dialog.close"
)

type dialogOpenParams struct {
	Kind        string                    `json:"kind"`
	Title       string                    `json:"title"`
	SubmitLabel string                    `json:"submitLabel,omitempty"`
	Sections    []domainplugin.FieldGroup `json:"sections"`
	Values      map[string]string         `json:"values,omitempty"`
}

type dialogErrorParams struct {
	DialogID    string            `json:"dialogId"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}

type dialogIDParams struct {
	DialogID string `json:"dialogId"`
}

// Handle dispatches one dialog.* call: decode, delegate, nothing else.
func (s *DialogService) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	case MethodDialogOpen:
		var req dialogOpenParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return s.open(pluginID, req)
	case MethodDialogSetError:
		var req dialogErrorParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := s.setError(pluginID, req); err != nil {
			return nil, err
		}
	case MethodDialogClose:
		var req dialogIDParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		s.closeByPlugin(pluginID, req.DialogID)
	default:
		return nil, fmt.Errorf("%w: %s", domainplugin.ErrNotImplemented, method)
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// open handles dialog.open. It returns the id immediately: a dialog is open for as long as a
// person is reading it, and an RPC that stayed open for that would blow the 5 s timeout on every
// dialog a user thinks about (ADR-015 §2).
func (s *DialogService) open(pluginID string, req dialogOpenParams) (json.RawMessage, error) {
	caps := s.capsFor(pluginID)
	if caps == nil || !caps.Dialogs {
		return nil, fmt.Errorf("%w: dialogs not declared", domainplugin.ErrCapabilityDenied)
	}
	kind, err := domainplugin.ParseDialogKind(req.Kind)
	if err != nil {
		return nil, err
	}
	dialog := domainplugin.Dialog{
		ID:          newDialogID(),
		PluginID:    pluginID,
		Kind:        kind,
		Title:       sanitizeDialogTitle(req.Title),
		SubmitLabel: sanitizeDialogTitle(req.SubmitLabel),
		// Labels, descriptions, placeholders and option labels are drawn next to a button the user
		// is about to press, so they are cleaned exactly like the title above.
		Sections: sanitizeFieldGroups(req.Sections),
		Values:   req.Values,
	}
	if err := validateDialogSections(dialog); err != nil {
		return nil, err
	}
	if err := s.registry.Add(dialog); err != nil {
		return nil, err
	}
	s.presenter.DialogOpened(dialog)
	return json.Marshal(map[string]string{"dialogId": dialog.ID})
}

// setError shows a plugin-supplied failure on an open dialog — "docker refused this name" — which
// is the whole reason a form does not close on submit until its owner says so.
func (s *DialogService) setError(pluginID string, req dialogErrorParams) error {
	dialog, err := s.registry.Get(req.DialogID, pluginID)
	if err != nil {
		return err
	}
	s.presenter.DialogError(dialog.ID, sanitizeMessage(req.Message), sanitizeMessages(req.FieldErrors))
	return nil
}

// closeByPlugin closes a dialog the plugin itself asked to close. Idempotent, and silent about a
// dialog that is already gone: the user may have cancelled it a moment earlier.
func (s *DialogService) closeByPlugin(pluginID, dialogID string) {
	if _, err := s.registry.Get(dialogID, pluginID); err != nil {
		return
	}
	if _, taken := s.registry.Take(dialogID); !taken {
		return
	}
	s.presenter.DialogClosed(dialogID)
}

func (s *DialogService) capsFor(pluginID string) *domainplugin.UICaps {
	if s.caps == nil {
		return nil
	}
	return s.caps(pluginID)
}

// validateDialogSections enforces the declaration rules a dialog's fields must satisfy before the
// modal is shown (ADR-015 §2).
//
// Everything a dialog and a node details panel share — ids, types, the ban on secrets, widths,
// select options, dependsOn, and compiling validation.pattern so a submit can be checked against
// it — lives in domainplugin.ValidateWireFields. What stays here is the one rule only a dialog
// has: a modal with nothing to show is not a question.
func validateDialogSections(dialog domainplugin.Dialog) error {
	if dialog.CountFields() == 0 {
		return fmt.Errorf("dialog: at least one field is required")
	}
	return domainplugin.ValidateWireFields(dialog.Sections, "dialog")
}
