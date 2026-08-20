package wails

import (
	"errors"

	"xquakshell/internal/domain"
)

var errLocalesUnavailable = errors.New("locale catalog unavailable")

// LocaleInfoDTO is one selectable language in the Appearance settings.
type LocaleInfoDTO struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
}

// LocaleMessagesDTO is a language's full message catalogue, already merged with English.
type LocaleMessagesDTO struct {
	Code     string            `json:"code"`
	Name     string            `json:"name"`
	Messages map[string]string `json:"messages"`
}

// SetLocaleCatalog wires the language catalogue. Passing nil leaves the interface English.
func (a *AppAPI) SetLocaleCatalog(catalog domain.LocaleCatalog) {
	if a == nil {
		return
	}
	a.locales = catalog
}

// ListLocales returns the languages this installation can display.
//
// It deliberately does not touch the vault, so the language list is available on the unlock screen
// — the one screen a user who cannot read English most needs translated.
func (a *AppAPI) ListLocales() []LocaleInfoDTO {
	if a == nil || a.locales == nil {
		return []LocaleInfoDTO{}
	}
	infos := a.locales.Available()
	dtos := make([]LocaleInfoDTO, 0, len(infos))
	for _, info := range infos {
		dtos = append(dtos, LocaleInfoDTO{Code: info.Code, Name: info.Name, Builtin: info.Builtin})
	}
	return dtos
}

// GetLocaleMessages returns one language's catalogue with the English fallback already applied, so
// the interface never has to decide what to show for a key a translation omits.
//
// An unknown or malformed code falls back to English rather than erroring: this is called during
// startup, before anything can show an error, and a vault written by a build that knows more
// languages than this one is a normal thing to open.
func (a *AppAPI) GetLocaleMessages(code string) (LocaleMessagesDTO, error) {
	if a == nil || a.locales == nil {
		return LocaleMessagesDTO{}, errLocalesUnavailable
	}
	pack, err := a.locales.Pack(code)
	if err != nil {
		pack, err = a.locales.Pack(domain.DefaultLocale)
		if err != nil {
			return LocaleMessagesDTO{}, errLocalesUnavailable
		}
	}
	return LocaleMessagesDTO{Code: pack.Code, Name: pack.Name, Messages: pack.Messages}, nil
}
