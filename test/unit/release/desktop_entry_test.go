package release_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readDesktopEntry parses the packaged .desktop template into key/value pairs. Comments and the
// group header are skipped; the format is simple enough that a real INI parser would be overkill.
func readDesktopEntry(t *testing.T) map[string]string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(root, "packaging", "linux", "xquakshell.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	entries := map[string]string{}
	sawHeader := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[Desktop Entry]" {
			sawHeader = true
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			t.Errorf("malformed line in .desktop file: %q", line)
			continue
		}
		entries[key] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !sawHeader {
		t.Fatal("[Desktop Entry] group header missing; desktop environments ignore the file entirely")
	}
	return entries
}

// A .desktop file missing a required key is silently ignored by desktop environments — no error,
// no log, just no launcher and no icon. These assertions are the only feedback available.
func TestDesktopEntryHasRequiredKeys(t *testing.T) {
	entries := readDesktopEntry(t)

	required := map[string]string{
		"Type": "Application",
		"Name": "xQuakShell",
	}
	for key, want := range required {
		if entries[key] != want {
			t.Errorf("%s = %q, want %q", key, entries[key], want)
		}
	}
	for _, key := range []string{"Exec", "Icon", "Categories"} {
		if entries[key] == "" {
			t.Errorf("%s is missing; the entry would not launch or would show no icon", key)
		}
	}
}

// Categories must come from the registered freedesktop list and be semicolon-terminated, or the
// entry lands in a fallback menu section instead of Internet.
func TestDesktopEntryCategoriesAreWellFormed(t *testing.T) {
	entries := readDesktopEntry(t)

	categories := entries["Categories"]
	if !strings.HasSuffix(categories, ";") {
		t.Errorf("Categories = %q, want a trailing semicolon", categories)
	}
	if !strings.Contains(categories, "Network") {
		t.Errorf("Categories = %q, want it to include the Network main category", categories)
	}
}

// The archive is portable, so absolute paths are only known once it is unpacked. Both path keys
// must therefore still carry the placeholder the install step substitutes — a hard-coded path
// here would point at the machine that built the release.
func TestDesktopEntryPathsArePlaceholders(t *testing.T) {
	entries := readDesktopEntry(t)

	const placeholder = "%%INSTALL_DIR%%"
	for _, key := range []string{"Exec", "Icon"} {
		if !strings.Contains(entries[key], placeholder) {
			t.Errorf("%s = %q, want it to contain %s so the install step can substitute the unpack directory",
				key, entries[key], placeholder)
		}
	}
}
