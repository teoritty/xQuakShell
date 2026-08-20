package usecase

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

type recordedNotify struct {
	pluginID string
	method   string
	locale   string
}

type fakeLocaleNotifier struct {
	mu   sync.Mutex
	sent []recordedNotify
}

func (f *fakeLocaleNotifier) Notify(_ context.Context, pluginID, method string, params json.RawMessage) error {
	var body struct {
		Locale string `json:"locale"`
	}
	_ = json.Unmarshal(params, &body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, recordedNotify{pluginID: pluginID, method: method, locale: body.Locale})
	return nil
}

func (f *fakeLocaleNotifier) records() []recordedNotify {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedNotify(nil), f.sent...)
}

type fakeI18nPlugins struct{ ids []string }

func (f fakeI18nPlugins) I18nPlugins() []string { return f.ids }

func TestSetLocaleReachesEveryPluginThatAskedForIt(t *testing.T) {
	notifier := &fakeLocaleNotifier{}
	observer := NewLocaleObserver(fakeI18nPlugins{ids: []string{"a", "b"}}, notifier)

	observer.SetLocale("ru")

	sent := notifier.records()
	if len(sent) != 2 {
		t.Fatalf("sent %d notifications, want one per plugin", len(sent))
	}
	for _, rec := range sent {
		if rec.method != localeChangedMethod {
			t.Errorf("method = %q, want %q", rec.method, localeChangedMethod)
		}
		if rec.locale != "ru" {
			t.Errorf("%s got locale %q, want %q", rec.pluginID, rec.locale, "ru")
		}
	}
}

// Saving settings writes the whole struct, so a change to the ping interval would otherwise wake
// every translated plugin to tell it a language it already has.
func TestSetLocaleIsSilentWhenTheLanguageDidNotChange(t *testing.T) {
	notifier := &fakeLocaleNotifier{}
	observer := NewLocaleObserver(fakeI18nPlugins{ids: []string{"a"}}, notifier)

	observer.SetLocale("ru")
	observer.SetLocale("ru")
	observer.SetLocale("ru")

	if got := len(notifier.records()); got != 1 {
		t.Errorf("sent %d notifications for one language, want 1", got)
	}
}

func TestSetLocaleSendsAgainWhenTheLanguageActuallyChanges(t *testing.T) {
	notifier := &fakeLocaleNotifier{}
	observer := NewLocaleObserver(fakeI18nPlugins{ids: []string{"a"}}, notifier)

	observer.SetLocale("ru")
	observer.SetLocale("en")

	sent := notifier.records()
	if len(sent) != 2 || sent[1].locale != "en" {
		t.Errorf("got %+v, want a second notification carrying en", sent)
	}
}

// The level-triggered contract: a plugin that restarts after a language change would otherwise go
// on writing in the language it first started under, with nothing to tell it otherwise.
func TestPluginStartedRepeatsTheCurrentLanguage(t *testing.T) {
	notifier := &fakeLocaleNotifier{}
	observer := NewLocaleObserver(fakeI18nPlugins{ids: []string{"a"}}, notifier)
	observer.SetLocale("ru")

	observer.PluginStarted("b")

	sent := notifier.records()
	if len(sent) != 2 {
		t.Fatalf("sent %d notifications, want the broadcast plus the restart", len(sent))
	}
	if sent[1].pluginID != "b" || sent[1].locale != "ru" {
		t.Errorf("restart notification = %+v, want b told ru", sent[1])
	}
}

// Plugins start before the vault opens, so there is a window where the host has no answer. Sending
// an empty language would be worse than sending nothing: a plugin cannot tell it apart from a
// deliberate reset to no language at all.
func TestPluginStartedSaysNothingBeforeALanguageIsKnown(t *testing.T) {
	notifier := &fakeLocaleNotifier{}
	observer := NewLocaleObserver(fakeI18nPlugins{ids: []string{"a"}}, notifier)

	observer.PluginStarted("a")

	if got := len(notifier.records()); got != 0 {
		t.Errorf("sent %d notifications with no language known, want none", got)
	}
}

func TestLocaleIsWhatTheHandshakeWillCarry(t *testing.T) {
	observer := NewLocaleObserver(fakeI18nPlugins{}, &fakeLocaleNotifier{})
	if got := observer.Locale(); got != "" {
		t.Errorf("Locale() = %q before any change, want empty", got)
	}
	observer.SetLocale("pt-BR")
	if got := observer.Locale(); got != "pt-BR" {
		t.Errorf("Locale() = %q, want the language last broadcast", got)
	}
}

// A plugin that granted nothing must not be addressed, and a nil observer must not panic: both are
// ordinary states, not errors.
func TestLocaleObserverIsInertWithoutPluginsOrItself(t *testing.T) {
	notifier := &fakeLocaleNotifier{}
	NewLocaleObserver(fakeI18nPlugins{}, notifier).SetLocale("ru")
	if got := len(notifier.records()); got != 0 {
		t.Errorf("sent %d notifications with no plugins granting i18n, want none", got)
	}

	var nilObserver *LocaleObserver
	nilObserver.SetLocale("ru")
	nilObserver.PluginStarted("a")
	if got := nilObserver.Locale(); got != "" {
		t.Errorf("nil observer Locale() = %q, want empty", got)
	}
}
