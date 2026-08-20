package locale

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"xquakshell/internal/domain"
)

// Catalog serves the languages this installation can display, combining the packs compiled into
// the binary with any the user has dropped into <dataRoot>/locales.
type Catalog struct {
	builtin map[string]domain.LocalePack
	disk    *diskReader
}

// NewCatalog builds the catalogue rooted at a portable data directory.
//
// dataRoot may point at a directory that does not exist or cannot be read; that yields a catalogue
// of the built-in languages rather than an error, because a portable copy on read-only media is a
// supported way to run this application.
func NewCatalog(dataRoot string, log *slog.Logger) (*Catalog, error) {
	builtin, err := loadBuiltins()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	root := ""
	if dataRoot != "" {
		root = filepath.Join(dataRoot, DirName)
	}
	return &Catalog{builtin: builtin, disk: &diskReader{root: root, log: log}}, nil
}

// Available lists every selectable language, built-in first and then alphabetically by code so the
// order does not depend on map iteration.
func (c *Catalog) Available() []domain.LocaleInfo {
	onDisk := c.disk.list()
	infos := make([]domain.LocaleInfo, 0, len(c.builtin)+len(onDisk))

	// A user pack for a built-in language may not rename it (overlay refuses that), so the name
	// listed here is the built-in one — the same name the merged pack reports.
	for code, pack := range c.builtin {
		infos = append(infos, domain.LocaleInfo{Code: code, Name: pack.Name, Builtin: true})
	}
	for code, pack := range onDisk {
		if _, isBuiltin := c.builtin[code]; isBuiltin {
			continue
		}
		infos = append(infos, domain.LocaleInfo{Code: code, Name: pack.Name, Builtin: false})
	}

	slices.SortFunc(infos, func(a, b domain.LocaleInfo) int {
		if a.Builtin != b.Builtin {
			if a.Builtin {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Code, b.Code)
	})
	return infos
}

// Pack returns the requested language already merged with English, so a caller never has to
// implement a fallback of its own.
//
// The layering is English, then the built-in pack for this language, then the on-disk pack. Only
// the first two are trusted with the security namespace.
func (c *Catalog) Pack(code string) (domain.LocalePack, error) {
	if !ValidCode(code) {
		return domain.LocalePack{}, fmt.Errorf("%q: %w", code, domain.ErrLocaleCodeInvalid)
	}

	merged := c.builtin[domain.DefaultLocale]
	merged.Code = code
	merged.Name = ""

	found := false
	if pack, ok := c.builtin[code]; ok {
		merged = overlay(merged, pack, true)
		found = true
	}
	if pack, ok := c.disk.list()[code]; ok {
		merged = overlay(merged, pack, false)
		found = true
	}
	if !found {
		return domain.LocalePack{}, fmt.Errorf("%q: %w", code, domain.ErrLocaleNotFound)
	}
	return merged, nil
}
