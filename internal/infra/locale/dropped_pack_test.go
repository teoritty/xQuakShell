package locale

import (
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain"
)

// The manual scenario, as a test: a user drops a pack next to the executable and it appears.
func TestDroppedPackAppearsAndOverridesByKey(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	de := `{"code":"de","name":"Deutsch","messages":{"settings.tab.files":"Dateien"}}`
	if err := os.WriteFile(filepath.Join(dir, "de.json"), []byte(de), 0o600); err != nil {
		t.Fatal(err)
	}
	ru := `{"code":"ru","name":"X","messages":{"settings.tab.files":"ФАЙЛЫ","security.hostkey.mismatch.body":"ok"}}`
	if err := os.WriteFile(filepath.Join(dir, "ru.json"), []byte(ru), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog, err := NewCatalog(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	var sawDE bool
	for _, info := range catalog.Available() {
		if info.Code == "de" && info.Name == "Deutsch" {
			sawDE = true
		}
	}
	if !sawDE {
		t.Error("a dropped de.json did not appear in the language list")
	}

	dePack, _ := catalog.Pack("de")
	if dePack.Messages["settings.tab.files"] != "Dateien" {
		t.Error("the dropped translation did not win for a key it carries")
	}
	if dePack.Messages["settings.tab.network"] != "Network" {
		t.Error("a key the pack omits did not fall back to English")
	}

	ruPack, _ := catalog.Pack("ru")
	if ruPack.Messages["settings.tab.files"] != "ФАЙЛЫ" {
		t.Error("a disk pack must override a built-in ordinary key")
	}
	want := "The host key for {host} has changed!"
	if got := ruPack.Messages[domain.SecurityMessagePrefix+"hostkey.mismatch.body"]; got == "ok" {
		t.Errorf("a disk pack rewrote a built-in security warning: %q", got)
	} else if len(got) < len(want) {
		t.Errorf("the built-in Russian warning is missing: %q", got)
	}
}
