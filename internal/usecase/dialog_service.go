package usecase

import (
	"fmt"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
)

// DialogPresenter shows and hides a plugin's modal. Implemented in internal/presentation/wails.
type DialogPresenter interface {
	DialogOpened(d domainplugin.Dialog)
	DialogClosed(dialogID string)
	DialogError(dialogID, message string, fieldErrors map[string]string)
}

// DialogService is the use case behind the dialog.* verbs (ADR-015 §2).
//
// Like SurfaceService it holds no lock of its own: DialogRegistry owns the state, and every method
// here takes what it needs, releases, then calls out to the presenter or the plugin.
type DialogService struct {
	registry  *DialogRegistry
	presenter DialogPresenter
	outbound  domainplugin.DialogOutboundPort
	caps      SurfaceCapabilityLookup
}

// NewDialogService wires the dialog use case. presenter may be nil at construction and pushed in
// later with SetPresenter, for the same composition-order reason SurfaceService allows it.
func NewDialogService(
	registry *DialogRegistry,
	presenter DialogPresenter,
	outbound domainplugin.DialogOutboundPort,
	caps SurfaceCapabilityLookup,
) *DialogService {
	if presenter == nil {
		presenter = noopDialogPresenter{}
	}
	return &DialogService{registry: registry, presenter: presenter, outbound: outbound, caps: caps}
}

// SetPresenter late-binds the UI side.
func (s *DialogService) SetPresenter(presenter DialogPresenter) {
	if presenter != nil {
		s.presenter = presenter
	}
}

// Submit delivers a form's answer. Called from the UI.
func (s *DialogService) Submit(dialogID string, values map[string]string) error {
	// Peeked before it is taken, because a submit can fail validation and a failed submit must
	// leave the dialog open for the user to correct.
	dialog, ok := s.registry.peek(dialogID)
	if !ok {
		return fmt.Errorf("dialog: no such dialog")
	}
	if dialog.Kind != domainplugin.DialogKindForm {
		return fmt.Errorf("dialog: a detail dialog has no answer to submit")
	}
	accepted, err := validateDialogValues(dialog, values)
	if err != nil {
		return err
	}
	// Only now is it consumed: taking first would close the dialog on a value the user still has
	// to fix.
	if _, taken := s.registry.Take(dialogID); !taken {
		return fmt.Errorf("dialog: no such dialog")
	}
	s.presenter.DialogClosed(dialogID)
	if s.outbound != nil {
		s.outbound.Submitted(dialog.PluginID, dialogID, accepted)
	}
	return nil
}

// Cancel closes a dialog without an answer. Called from the UI.
func (s *DialogService) Cancel(dialogID string) error {
	dialog, ok := s.registry.Take(dialogID)
	if !ok {
		return fmt.Errorf("dialog: no such dialog")
	}
	s.presenter.DialogClosed(dialogID)
	if s.outbound != nil {
		s.outbound.Cancelled(dialog.PluginID, dialogID)
	}
	return nil
}

// CancelForPlugin closes a stopped plugin's dialog. The plugin is not told: its process is gone,
// and a modal whose owner cannot answer must not stay on screen.
func (s *DialogService) CancelForPlugin(pluginID string) {
	dialog, ok := s.registry.TakeByPlugin(pluginID)
	if !ok {
		return
	}
	s.presenter.DialogClosed(dialog.ID)
}

// validateDialogValues keeps only declared, currently visible fields and checks each against its
// declaration.
//
// An undeclared key is DROPPED rather than refused: the frontend sends what it rendered, and a
// stale extra key is an ordinary UI race. A declared field with an invalid value is REFUSED, since
// that is the plugin's own rule being broken and the user can fix it.
//
// A field whose dependsOn is off is not part of this answer at all: its value is dropped and its
// required flag does not apply. That is the rule SavePluginFields already follows for connection
// fields and the one the renderer follows when it decides what to draw — a third opinion here
// would refuse a form the user has no way to complete, because the field it is asking for is not
// on screen.
func validateDialogValues(dialog domainplugin.Dialog, values map[string]string) (map[string]string, error) {
	accepted := make(map[string]string, len(values))
	for id, value := range values {
		field, declared := dialog.FieldByID(id)
		if !declared || !domainplugin.IsFieldVisible(field, values) {
			continue
		}
		if err := validateFieldValue(value, &field); err != nil {
			return nil, fmt.Errorf("field %q: %w", id, err)
		}
		accepted[id] = value
	}
	// A required field the user left empty is the plugin's rule, enforced here rather than trusted
	// to the frontend, which is not a trust boundary.
	for _, group := range dialog.Sections {
		for _, field := range group.Fields {
			if !field.Required || !domainplugin.IsFieldVisible(field, values) {
				continue
			}
			if accepted[field.ID] == "" {
				return nil, fmt.Errorf("field %q is required", field.ID)
			}
		}
	}
	return accepted, nil
}

// sanitizeDialogTitle applies the same treatment a surface title gets: stripped of control
// characters and bidirectional overrides, then bounded.
func sanitizeDialogTitle(title string) string {
	clean := discovery.SanitizeText(title)
	runes := []rune(clean)
	if len(runes) > domainplugin.MaxSurfaceTitleLen {
		runes = runes[:domainplugin.MaxSurfaceTitleLen]
	}
	return string(runes)
}

type noopDialogPresenter struct{}

func (noopDialogPresenter) DialogOpened(domainplugin.Dialog)              {}
func (noopDialogPresenter) DialogClosed(string)                           {}
func (noopDialogPresenter) DialogError(string, string, map[string]string) {}

var _ domainplugin.DialogInboundPort = (*DialogService)(nil)
var _ domainplugin.DialogPluginCloser = (*DialogService)(nil)
