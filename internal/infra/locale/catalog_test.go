package locale

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

func newTestCatalog(t *testing.T, dataRoot string) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(dataRoot, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return catalog
}

// writePack drops a raw pack file into <dataRoot>/locales.
func writePack(t *testing.T, dataRoot, name, body string) {
	t.Helper()
	dir := filepath.Join(dataRoot, DirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestPackFallsBackToEnglishKeyByKey(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "ru.json", `{"code":"ru","name":"Русский","messages":{"settings.tab.files":"Файлы"}}`)

	pack, err := newTestCatalog(t, root).Pack("ru")
	if err != nil {
		t.Fatalf("Pack(ru): %v", err)
	}

	if got := pack.Messages["settings.tab.files"]; got != "Файлы" {
		t.Errorf("translated key = %q, want %q; the disk pack must win for a key it carries", got, "Файлы")
	}
	if got := pack.Messages["settings.tab.network"]; got != "Сеть" {
		t.Errorf("key absent from the disk pack = %q, want the built-in Russian %q", got, "Сеть")
	}
}

func TestPackFallsBackToEnglishForALanguageWithNoBuiltin(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "de.json", `{"code":"de","name":"Deutsch","messages":{"settings.tab.files":"Dateien"}}`)

	pack, err := newTestCatalog(t, root).Pack("de")
	if err != nil {
		t.Fatalf("Pack(de): %v", err)
	}

	if got := pack.Messages["settings.tab.files"]; got != "Dateien" {
		t.Errorf("translated key = %q, want %q", got, "Dateien")
	}
	if got := pack.Messages["settings.tab.network"]; got != "Network" {
		t.Errorf("untranslated key = %q, want the English %q; a missing key must never render blank", got, "Network")
	}
}

// A disk pack rewriting a security warning is the attack this whole namespace exists to stop.
func TestDiskPackCannotOverrideABuiltinSecurityMessage(t *testing.T) {
	const key = domain.SecurityMessagePrefix + "hostkey.changed"
	root := t.TempDir()
	writePack(t, root, "ru.json",
		`{"code":"ru","name":"Русский","messages":{"`+key+`":"Всё в порядке, нажмите Принять"}}`)

	catalog := newTestCatalog(t, root)
	catalog.builtin["ru"] = overlay(catalog.builtin["ru"], domain.LocalePack{
		Messages: map[string]string{key: "Ключ хоста изменился"},
	}, true)

	pack, err := catalog.Pack("ru")
	if err != nil {
		t.Fatalf("Pack(ru): %v", err)
	}
	if got := pack.Messages[key]; got != "Ключ хоста изменился" {
		t.Errorf("%s = %q, want the built-in warning; a disk pack must not be able to rewrite it", key, got)
	}
}

// The protection is an override ban, not a translation ban: a language with no built-in pack must
// still be able to translate its warnings, or the rule would force English on every new locale.
func TestDiskPackMayAddASecurityMessageThatHasNoBuiltinValue(t *testing.T) {
	const key = domain.SecurityMessagePrefix + "plugin.consent"
	root := t.TempDir()
	writePack(t, root, "de.json",
		`{"code":"de","name":"Deutsch","messages":{"`+key+`":"Plugin installieren?"}}`)

	pack, err := newTestCatalog(t, root).Pack("de")
	if err != nil {
		t.Fatalf("Pack(de): %v", err)
	}
	if got := pack.Messages[key]; got != "Plugin installieren?" {
		t.Errorf("%s = %q, want the disk translation; nothing built in claims this key", key, got)
	}
}

func TestAvailableListsBuiltinsFirstAndThenDiskPacks(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "de.json", `{"code":"de","name":"Deutsch","messages":{}}`)

	infos := newTestCatalog(t, root).Available()

	codes := make([]string, len(infos))
	for i, info := range infos {
		codes[i] = info.Code
	}
	want := []string{"en", "ru", "de"}
	if strings.Join(codes, ",") != strings.Join(want, ",") {
		t.Errorf("Available() = %v, want %v; built-ins first, then disk packs, each sorted by code", codes, want)
	}
	if infos[len(infos)-1].Builtin {
		t.Error("a disk-only pack was reported as built-in")
	}
	if infos[len(infos)-1].Name != "Deutsch" {
		t.Errorf("disk pack name = %q, want %q", infos[len(infos)-1].Name, "Deutsch")
	}
}

func TestAvailableDoesNotListABuiltinTwiceWhenADiskPackShadowsIt(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "ru.json", `{"code":"ru","name":"Взломанный","messages":{}}`)

	for _, info := range newTestCatalog(t, root).Available() {
		if info.Code != "ru" {
			continue
		}
		if !info.Builtin {
			t.Error("ru lost its built-in flag because a disk pack shadowed it")
		}
		if info.Name != "Русский" {
			t.Errorf("ru name = %q, want %q; a disk pack must not rename a built-in language", info.Name, "Русский")
		}
	}
}

func TestPackRejectsAnInvalidCodeBeforeTouchingTheFilesystem(t *testing.T) {
	catalog := newTestCatalog(t, t.TempDir())
	for _, code := range []string{"../../evil", "en/../..", "e", "ENG", "en_US", "", "en-us", "en-USA"} {
		if _, err := catalog.Pack(code); !errors.Is(err, domain.ErrLocaleCodeInvalid) {
			t.Errorf("Pack(%q) error = %v, want ErrLocaleCodeInvalid", code, err)
		}
	}
}

func TestPackReportsNotFoundForAnUnknownLanguage(t *testing.T) {
	if _, err := newTestCatalog(t, t.TempDir()).Pack("fr"); !errors.Is(err, domain.ErrLocaleNotFound) {
		t.Errorf("Pack(fr) error = %v, want ErrLocaleNotFound", err)
	}
}

func TestBuiltinLanguagesSurviveAMissingDataRoot(t *testing.T) {
	catalog := newTestCatalog(t, filepath.Join(t.TempDir(), "does-not-exist"))
	if len(catalog.Available()) != 2 {
		t.Errorf("Available() = %d languages, want the 2 built-ins; a missing data root is not an error", len(catalog.Available()))
	}
	if _, err := catalog.Pack("ru"); err != nil {
		t.Errorf("Pack(ru) with no data root: %v", err)
	}
}

func TestMalformedPacksAreSkippedWithoutBreakingTheRest(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "de.json", `{"code":"de","name":"Deutsch","messages":{"k":`)
	writePack(t, root, "fr.json", `{"code":"es","name":"Español","messages":{}}`)
	writePack(t, root, "it.json", `{"code":"it","name":"Italiano","messages":{"k":{"nested":"no"}}}`)
	writePack(t, root, "pt.json", `{"code":"pt","name":"Português","messages":{"k":"ok"}}`)

	infos := newTestCatalog(t, root).Available()

	byCode := map[string]bool{}
	for _, info := range infos {
		byCode[info.Code] = true
	}
	for _, bad := range []string{"de", "fr", "es", "it"} {
		if byCode[bad] {
			t.Errorf("%q was listed; a pack that is malformed, nested, or lies about its code must be skipped", bad)
		}
	}
	if !byCode["pt"] {
		t.Error("pt was skipped; one bad pack must not take the good ones with it")
	}
}

func TestOversizedPackIsRejectedWhole(t *testing.T) {
	root := t.TempDir()
	filler := strings.Repeat("a", maxTextBytes+1)
	writePack(t, root, "de.json", `{"code":"de","name":"Deutsch","messages":{"k":"`+filler+`","ok":"fine"}}`)

	if _, err := newTestCatalog(t, root).Pack("de"); !errors.Is(err, domain.ErrLocaleNotFound) {
		t.Errorf("Pack(de) error = %v, want ErrLocaleNotFound; an over-limit pack is rejected whole, not trimmed", err)
	}
}

// English is the pack every other language falls back to, key by key, so a key a translation
// carries but English does not can never be reached through the fallback and is almost always a
// typo in the translated key. The reverse is fine and expected: an untranslated key falls back.
func TestEveryBuiltinKeyExistsInEnglish(t *testing.T) {
	catalog := newTestCatalog(t, t.TempDir())
	english := catalog.builtin[domain.DefaultLocale]

	for code, pack := range catalog.builtin {
		if code == domain.DefaultLocale {
			continue
		}
		for key := range pack.Messages {
			if _, ok := english.Messages[key]; !ok {
				t.Errorf("%s.json carries %q, which en.json does not; nothing can fall back to it", code, key)
			}
		}
	}
}

// The security namespace is only a protection for keys English actually claims: overlay lets an
// untrusted pack ADD a security key nothing built in has. A warning that exists only in a
// translation is therefore a warning a disk pack could have written, which defeats the rule.
func TestEverySecurityKeyIsClaimedByEnglish(t *testing.T) {
	catalog := newTestCatalog(t, t.TempDir())
	english := catalog.builtin[domain.DefaultLocale]

	found := false
	for key := range english.Messages {
		if strings.HasPrefix(key, domain.SecurityMessagePrefix) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no %s* key in en.json; either the namespace is unused or the prefix moved and the "+
			"protection is now guarding nothing", domain.SecurityMessagePrefix)
	}

	for code, pack := range catalog.builtin {
		for key := range pack.Messages {
			if !strings.HasPrefix(key, domain.SecurityMessagePrefix) {
				continue
			}
			if _, ok := english.Messages[key]; !ok {
				t.Errorf("%s.json declares the security key %q that en.json does not claim, so a pack "+
					"on disk could supply it instead", code, key)
			}
		}
	}
}
