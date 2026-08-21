package logwindow

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

// fakeCatalog serves one language and refuses everything else, so the fallback arm is reachable
// without a real pack directory.
type fakeCatalog struct {
	packs map[string]domain.LocalePack
}

func (c fakeCatalog) Available() []domain.LocaleInfo { return nil }

func (c fakeCatalog) Pack(code string) (domain.LocalePack, error) {
	pack, ok := c.packs[code]
	if !ok {
		return domain.LocalePack{}, domain.ErrLocaleNotFound
	}
	return pack, nil
}

func testCatalog() fakeCatalog {
	return fakeCatalog{packs: map[string]domain.LocalePack{
		domain.DefaultLocale: {Code: domain.DefaultLocale, Name: "English", Messages: map[string]string{"logviewer.title": "Debug log"}},
		"ru":                 {Code: "ru", Name: "Русский", Messages: map[string]string{"logviewer.title": "Журнал отладки"}},
	}}
}

// The parent passes the language on the command line. Nothing else in this process knows it: the
// setting lives in the vault, which the viewer never unlocks, and the localStorage mirror belongs
// to the main window's WebView.
func TestParseViewerOptionsReadsTheLocaleFlag(t *testing.T) {
	opts := ParseViewerOptions([]string{"xqs.exe", flagLogViewer, flagAddr + "127.0.0.1:1", flagLocale + "ru"})
	if opts.Locale != "ru" {
		t.Fatalf("Locale = %q, want %q", opts.Locale, "ru")
	}
	if opts.Addr != "127.0.0.1:1" {
		t.Fatalf("Addr = %q, want the address to survive alongside the locale flag", opts.Addr)
	}
}

func TestLaunchLocaleFallsBackToEnglishWhenTheParentSaidNothing(t *testing.T) {
	app := &LogViewerApp{locales: testCatalog()}
	if got := app.LaunchLocale(); got != domain.DefaultLocale {
		t.Fatalf("LaunchLocale() = %q, want %q", got, domain.DefaultLocale)
	}
}

func TestLaunchLocaleReportsWhatTheParentPassed(t *testing.T) {
	app := &LogViewerApp{locales: testCatalog(), locale: "ru"}
	if got := app.LaunchLocale(); got != "ru" {
		t.Fatalf("LaunchLocale() = %q, want %q", got, "ru")
	}
}

func TestGetLocaleMessagesServesTheRequestedLanguage(t *testing.T) {
	app := &LogViewerApp{locales: testCatalog(), locale: "ru"}
	pack, err := app.GetLocaleMessages("ru")
	if err != nil {
		t.Fatalf("GetLocaleMessages: %v", err)
	}
	if pack.Code != "ru" || pack.Messages["logviewer.title"] != "Журнал отладки" {
		t.Fatalf("pack = %+v, want the Russian catalogue", pack)
	}
}

func TestGetLocaleMessagesFallsBackToEnglishForAnUnknownLanguage(t *testing.T) {
	app := &LogViewerApp{locales: testCatalog()}
	pack, err := app.GetLocaleMessages("not-a-language")
	if err != nil {
		t.Fatalf("GetLocaleMessages: %v", err)
	}
	if pack.Code != domain.DefaultLocale {
		t.Fatalf("pack.Code = %q, want the English fallback %q", pack.Code, domain.DefaultLocale)
	}
}

// A viewer with no catalogue must say so rather than answer with an empty catalogue. An empty one
// is indistinguishable from a language whose every string is missing, and the frontend renders a
// missing string as its own key — which is exactly the raw-key window this binding exists to fix.
func TestGetLocaleMessagesWithoutACatalogReportsUnavailableRatherThanEmpty(t *testing.T) {
	app := &LogViewerApp{}
	pack, err := app.GetLocaleMessages(domain.DefaultLocale)
	if !errors.Is(err, errLocalesUnavailable) {
		t.Fatalf("err = %v, want errLocalesUnavailable", err)
	}
	if len(pack.Messages) != 0 {
		t.Fatalf("pack carried %d messages alongside the error", len(pack.Messages))
	}
}
