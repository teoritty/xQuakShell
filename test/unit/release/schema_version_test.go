package release_test

import (
	"regexp"
	"strconv"
	"testing"

	"xquakshell/internal/domain"
)

var changelogVaultSchema = regexp.MustCompile(`(?m)^\|\s*Vault schema\s*\|\s*(\d+)`)

// Bumping CurrentVaultVersion is the single most destructive one-character change in this
// repository: Decrypt refuses anything that is not the current version, and there is no migration
// mechanism, so raising the constant makes every existing vault unreadable by the build that
// raised it.
//
// This is a tripwire, not proof that a migration exists — nothing here can tell whether the
// migration is correct, and the project deliberately has no migration registry to check against
// (there has never been a migration to write). What it does guarantee is that the constant cannot
// move without the release notes moving with it, which is the point at which someone has to write
// down what happens to a user's data, and therefore the point at which they notice they owe a
// migration and a backup.
func TestVaultSchemaVersionMatchesTheChangelog(t *testing.T) {
	entries := changelogEntries(t)
	if len(entries) == 0 {
		t.Fatal("CHANGELOG.md has no versioned entry; this gate has stopped checking anything")
	}

	match := changelogVaultSchema.FindStringSubmatch(entries[0].body)
	if match == nil {
		t.Fatalf("the compatibility block for %s has no '| Vault schema | N |' row; "+
			"this gate has stopped checking anything", entries[0].version)
	}
	declared, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("vault schema in the changelog is not a number: %v", err)
	}

	if declared != domain.CurrentVaultVersion {
		t.Errorf("CHANGELOG.md declares vault schema %d, the code is at %d.\n"+
			"Raising CurrentVaultVersion makes every existing vault unreadable until a migration "+
			"exists. Before changing it: write the migration from %d, call vault.BackupVaultFile "+
			"before the first write at the new version, and record the bump and its user-visible "+
			"effect in the changelog entry (ADR-017).",
			declared, domain.CurrentVaultVersion, declared)
	}
}
