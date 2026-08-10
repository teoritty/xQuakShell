package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// wails.json's productVersion is stamped into the Windows executable's resource block, where
// nothing ever reads it back, so it drifts silently: it sat at 1.0.0 while the repository was
// tagging release candidates, and would have kept saying 1.0.0 through every patch after. The
// changelog is the one place a version has to be written down deliberately, so it is the anchor.
//
// The release workflow makes the third comparison — tag against both of these — because a tag does
// not exist on an ordinary commit and there would be nothing here to compare against.

var changelogHeading = regexp.MustCompile(`(?m)^## \[([^\]]+)\](?:\s*—\s*(.+))?$`)

type changelogEntry struct {
	version string
	date    string
	body    string
}

func readChangelog(t *testing.T) string {
	t.Helper()
	root := mustRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("CHANGELOG.md is the source of truth for the release version: %v", err)
	}
	return string(raw)
}

// changelogEntries returns every versioned section in file order, newest first, excluding a
// placeholder "Unreleased" heading.
func changelogEntries(t *testing.T) []changelogEntry {
	t.Helper()
	text := readChangelog(t)
	matches := changelogHeading.FindAllStringSubmatchIndex(text, -1)

	var entries []changelogEntry
	for i, m := range matches {
		version := text[m[2]:m[3]]
		if strings.EqualFold(version, "unreleased") {
			continue
		}
		date := ""
		if m[4] >= 0 {
			date = strings.TrimSpace(text[m[4]:m[5]])
		}
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		entries = append(entries, changelogEntry{version: version, date: date, body: text[m[1]:end]})
	}
	return entries
}

func TestProductVersionMatchesTheChangelog(t *testing.T) {
	entries := changelogEntries(t)
	if len(entries) == 0 {
		t.Fatal("CHANGELOG.md has no versioned heading of the form '## [x.y.z]'; this gate has stopped checking anything")
	}

	want := entries[0].version
	got := readWailsConfig(t).Info.ProductVersion
	if got != want {
		t.Errorf("wails.json info.productVersion = %q, newest changelog version is %q; "+
			"the Windows executable would report a version nobody released", got, want)
	}
}

// A release entry that skips these is an entry nobody can act on: BREAKING is what a plugin author
// reads to know whether their plugin still loads, and Compatibility is what says which axis moved.
// Checking it here rather than only at tag time means an incomplete entry fails on the commit that
// wrote it, while the author still remembers what changed.
func TestEveryChangelogEntryDeclaresCompatibilityAndBreaking(t *testing.T) {
	for _, entry := range changelogEntries(t) {
		if !strings.Contains(entry.body, "### Compatibility") {
			t.Errorf("changelog entry %s has no '### Compatibility' block", entry.version)
		}
		if !strings.Contains(entry.body, "### BREAKING") {
			t.Errorf("changelog entry %s has no '### BREAKING' section; it is required even when "+
				"there is nothing to report, so a missing one always means the entry is incomplete", entry.version)
		}
	}
}

// Only the newest entry may still be undated. An older one without a date means a release went out
// while its section was still marked as being prepared.
func TestOnlyTheNewestChangelogEntryMayBeUndated(t *testing.T) {
	entries := changelogEntries(t)
	if len(entries) < 2 {
		t.Skip("fewer than two released versions recorded")
	}
	for _, entry := range entries[1:] {
		if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(entry.date) {
			t.Errorf("changelog entry %s is dated %q, want YYYY-MM-DD", entry.version, entry.date)
		}
	}
}
