package release_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// versionInfo is the subset of build/windows/info.json this package asserts on. Wails resolves the
// file as a text/template against wails.json's info block and hands the result to winres, which
// turns it into the VS_VERSION_INFO resource of the executable.
type versionInfo struct {
	Fixed struct {
		FileVersion    string `json:"file_version"`
		ProductVersion string `json:"product_version"`
	} `json:"fixed"`
	Info map[string]map[string]string `json:"info"`
}

func readWindowsVersionInfo(t *testing.T) versionInfo {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "build", "windows", "info.json"))
	if err != nil {
		t.Fatalf("build/windows/info.json is the only source of the exe's version resource: %v", err)
	}
	var info versionInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("build/windows/info.json is not valid JSON: %v", err)
	}
	return info
}

// The whole of build/ is ignored because it is reproducible, and info.json is the one file in it
// that is not: it is a customisation Wails only reads from disk. CI builds from a fresh checkout,
// so an untracked fix here exists on one developer's machine and nowhere else, and the release
// silently gets the embedded default instead.
func TestWindowsVersionInfoIsNotIgnored(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "!build/windows/info.json") {
		t.Error(".gitignore no longer un-ignores build/windows/info.json; " +
			"CI would build from the embedded default and the exe would lose its version resource")
	}
}

// A string table is looked up by language id, and the one Wails ships by default is keyed "0000" —
// language-neutral, which VerQueryValue does not resolve: on the first v1.1.0 build the translation
// record said 0000 and every field on the Details tab came out blank, which reads as a version that
// was never bumped. Only a real language id (0409 = en-US) is displayable.
func TestVersionStringsUseADisplayableLanguage(t *testing.T) {
	info := readWindowsVersionInfo(t)
	if len(info.Info) == 0 {
		t.Fatal("info.json declares no string table; the exe would carry no product name or copyright")
	}
	for lang, fields := range info.Info {
		if lang == "0000" {
			t.Errorf("string table is keyed %q (language-neutral); "+
				"VerQueryValue cannot look it up and Explorer shows every field blank", lang)
		}
		if fields["ProductVersion"] == "" {
			t.Errorf("string table %q carries no ProductVersion", lang)
		}
		if fields["ProductName"] == "" {
			t.Errorf("string table %q carries no ProductName", lang)
		}
	}
}

// The fixed block is the numeric VS_FIXEDFILEINFO pair, read by installers and by Explorer's
// "Product version" column. file_version alone leaves product_version at 0.0.0.0, so both are
// templated from the same wails.json field.
func TestFixedVersionsAreBothTemplated(t *testing.T) {
	info := readWindowsVersionInfo(t)
	const want = "{{.Info.ProductVersion}}"
	if info.Fixed.FileVersion != want {
		t.Errorf("fixed.file_version = %q, want %q; a hard-coded number here goes stale the "+
			"next time wails.json is bumped", info.Fixed.FileVersion, want)
	}
	if info.Fixed.ProductVersion != want {
		t.Errorf("fixed.product_version = %q, want %q; without it the exe reports product "+
			"version 0.0.0.0", info.Fixed.ProductVersion, want)
	}
}
