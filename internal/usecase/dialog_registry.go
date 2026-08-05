package usecase

import (
	"fmt"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// DialogRegistry holds the open dialogs and owns the only mutex behind them, the same discipline
// SurfaceRegistry follows: no outbound call of any kind happens in this file, so "no lock is held
// across an RPC" stays checkable by reading it.
//
// At most one dialog per plugin (ADR-015 §Limits). Two modals from one plugin would stack, and the
// user would answer a question without seeing which one they were answering.
type DialogRegistry struct {
	mu       sync.Mutex
	byID     map[string]domainplugin.Dialog
	byPlugin map[string]string
}

// NewDialogRegistry creates an empty registry.
func NewDialogRegistry() *DialogRegistry {
	return &DialogRegistry{
		byID:     make(map[string]domainplugin.Dialog),
		byPlugin: make(map[string]string),
	}
}

// Add registers a dialog, refusing a second one for the same plugin.
func (r *DialogRegistry) Add(d domainplugin.Dialog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, open := r.byPlugin[d.PluginID]; open {
		return fmt.Errorf("%w: plugin already has an open dialog", domainplugin.ErrRateLimited)
	}
	r.byID[d.ID] = d
	r.byPlugin[d.PluginID] = d.ID
	return nil
}

// Get returns the dialog if pluginID owns it. As with surfaces, a dialog belonging to another
// plugin and one that does not exist return the same error, so an id cannot be probed.
func (r *DialogRegistry) Get(dialogID, pluginID string) (domainplugin.Dialog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[dialogID]
	if !ok || d.PluginID != pluginID {
		return domainplugin.Dialog{}, fmt.Errorf("%w: unknown dialog", domainplugin.ErrCapabilityDenied)
	}
	return d, nil
}

// peek reads a dialog without an ownership check and without consuming it.
//
// It exists for one caller: a submit that must validate before it closes anything, so a value the
// user still has to fix does not take the dialog down with it. Ownership is not checked because
// the caller is the UI, which addresses the dialog it is showing.
func (r *DialogRegistry) peek(dialogID string) (domainplugin.Dialog, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[dialogID]
	return d, ok
}

// Take removes a dialog and returns it.
//
// Removal happens BEFORE the answer is delivered (see dialog_service.go), and that ordering is
// what makes "exactly one answer" true without a second flag to keep in sync: whoever takes it
// first is the one answer, and everyone after finds nothing.
func (r *DialogRegistry) Take(dialogID string) (domainplugin.Dialog, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[dialogID]
	if !ok {
		return domainplugin.Dialog{}, false
	}
	delete(r.byID, dialogID)
	if r.byPlugin[d.PluginID] == dialogID {
		delete(r.byPlugin, d.PluginID)
	}
	return d, true
}

// TakeByPlugin removes a plugin's dialog, if it has one.
func (r *DialogRegistry) TakeByPlugin(pluginID string) (domainplugin.Dialog, bool) {
	r.mu.Lock()
	id, ok := r.byPlugin[pluginID]
	r.mu.Unlock()
	if !ok {
		return domainplugin.Dialog{}, false
	}
	return r.Take(id)
}
