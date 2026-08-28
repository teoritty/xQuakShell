package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

// localeChangedMethod is the host->plugin notification carrying the interface language.
//
// A notification rather than a request: the language has already changed on screen, and there is
// nothing a plugin could usefully answer. A plugin that cannot apply it keeps rendering whatever it
// rendered before, which is exactly what happens today.
const localeChangedMethod = "i18n.localeChanged"

// I18nPluginLookup lists the installed plugins that declared capabilities.i18n.
// Satisfied by *PluginRegistry.
type I18nPluginLookup interface {
	I18nPlugins() []string
}

// LocaleObserver tells plugins which language the interface is in.
//
// Like the discovery observer it is level-triggered: it holds the current language rather than a
// history of changes, sends the whole answer every time, and repeats it to a plugin that has just
// started. Without that repeat a plugin restarted after a language change would go on writing in
// the language that was in force when it first launched, with nothing to tell it otherwise — and
// unlike a missed expand event, nobody would think to toggle the setting twice to fix it.
type LocaleObserver struct {
	plugins  I18nPluginLookup
	notifier DiscoveryNotifier

	mu     sync.RWMutex
	locale string
}

// NewLocaleObserver wires the broadcast. The notifier is DiscoveryNotifier because that interface
// already means exactly "send a host->plugin notification"; giving it a second name would suggest
// the two differ.
func NewLocaleObserver(plugins I18nPluginLookup, notifier DiscoveryNotifier) *LocaleObserver {
	return &LocaleObserver{plugins: plugins, notifier: notifier}
}

// SetLocale records the language and tells every plugin that asked to know.
//
// A repeat of the language already in force is dropped rather than resent: saving settings writes
// the whole struct, so an unrelated change to the ping interval would otherwise wake every
// translated plugin for nothing.
func (o *LocaleObserver) SetLocale(code string) {
	if o == nil || code == "" {
		return
	}
	o.mu.Lock()
	if o.locale == code {
		o.mu.Unlock()
		return
	}
	o.locale = code
	o.mu.Unlock()

	if o.plugins == nil {
		return
	}
	for _, pluginID := range o.plugins.I18nPlugins() {
		o.send(pluginID, code)
	}
}

// Locale returns the language last broadcast, for the initialize handshake to carry.
func (o *LocaleObserver) Locale() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.locale
}

// PluginStarted tells one plugin the current language, right after it comes up.
//
// The initialize handshake already carries it, so this is the belt to that braces: a plugin that
// restarts while the host is running gets the same answer through the same path as a language
// change, which is one code path for the plugin to implement rather than two.
func (o *LocaleObserver) PluginStarted(pluginID string) {
	if o == nil {
		return
	}
	if code := o.Locale(); code != "" {
		o.send(pluginID, code)
	}
}

// send marshals and dispatches, logging rather than returning failures.
//
// There is nothing a caller could do with the error. The language is a fact about the interface,
// the plugin may be mid-restart, and PluginStarted will tell it again when it comes up.
func (o *LocaleObserver) send(pluginID, code string) {
	if o.notifier == nil {
		return
	}
	params, err := json.Marshal(map[string]string{"locale": code})
	if err != nil {
		slog.Warn("i18n: marshal locale notification failed", "err", err)
		return
	}
	// Detached for the reason above: nothing is awaited, so there is no deadline to inherit and no
	// caller to disappoint.
	if err := o.notifier.Notify(context.Background(), pluginID, localeChangedMethod, params); err != nil {
		slog.Debug("i18n: locale notify failed", "pluginId", pluginID, "err", err)
	}
}
