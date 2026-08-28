package wails

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

type fakeCatalog struct {
	infos []domain.LocaleInfo
	packs map[string]domain.LocalePack
}

func (f *fakeCatalog) Available() []domain.LocaleInfo { return f.infos }

func (f *fakeCatalog) Pack(code string) (domain.LocalePack, error) {
	pack, ok := f.packs[code]
	if !ok {
		return domain.LocalePack{}, domain.ErrLocaleNotFound
	}
	return pack, nil
}

func newFakeCatalogAPI() *AppAPI {
	api := &AppAPI{}
	api.SetLocaleCatalog(&fakeCatalog{
		infos: []domain.LocaleInfo{
			{Code: "en", Name: "English", Builtin: true},
			{Code: "de", Name: "Deutsch"},
		},
		packs: map[string]domain.LocalePack{
			"en": {Code: "en", Name: "English", Messages: map[string]string{"k": "English text"}},
			"de": {Code: "de", Name: "Deutsch", Messages: map[string]string{"k": "Deutscher Text"}},
		},
	})
	return api
}

func TestListLocalesCarriesTheBuiltinFlagThrough(t *testing.T) {
	dtos := newFakeCatalogAPI().ListLocales()

	if len(dtos) != 2 {
		t.Fatalf("ListLocales() = %d entries, want 2", len(dtos))
	}
	if !dtos[0].Builtin || dtos[0].Code != "en" {
		t.Errorf("first entry = %+v, want the built-in en", dtos[0])
	}
	if dtos[1].Builtin || dtos[1].Name != "Deutsch" {
		t.Errorf("second entry = %+v, want a non-built-in Deutsch", dtos[1])
	}
}

// An unrecognised language must not fail startup: the setting may have been written by a build that
// knows more languages than this one, and there is no interface yet in which to report an error.
func TestGetLocaleMessagesFallsBackToEnglishForAnUnknownLanguage(t *testing.T) {
	dto, err := newFakeCatalogAPI().GetLocaleMessages("xx")
	if err != nil {
		t.Fatalf("GetLocaleMessages(xx): %v", err)
	}
	if dto.Code != "en" || dto.Messages["k"] != "English text" {
		t.Errorf("got %+v, want the English catalogue", dto)
	}
}

func TestGetLocaleMessagesReturnsTheRequestedLanguage(t *testing.T) {
	dto, err := newFakeCatalogAPI().GetLocaleMessages("de")
	if err != nil {
		t.Fatalf("GetLocaleMessages(de): %v", err)
	}
	if dto.Code != "de" || dto.Messages["k"] != "Deutscher Text" {
		t.Errorf("got %+v, want the German catalogue", dto)
	}
}

func TestLocaleHandlersAreInertWithoutACatalog(t *testing.T) {
	api := &AppAPI{}
	if got := api.ListLocales(); len(got) != 0 {
		t.Errorf("ListLocales() = %v, want an empty list rather than nil-panicking", got)
	}
	if _, err := api.GetLocaleMessages("en"); !errors.Is(err, errLocalesUnavailable) {
		t.Errorf("GetLocaleMessages error = %v, want errLocalesUnavailable", err)
	}
}
