package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestValidateBundleRelativePathAllowsNestedEntry(t *testing.T) {
	if err := domainplugin.ValidateBundleRelativePath(`bin/plugin.exe`); err != nil {
		t.Fatalf("expected nested entry ok, got %v", err)
	}
}

func TestValidateBundleRelativePathRejectsParentTraversal(t *testing.T) {
	cases := []string{
		`../evil.exe`,
		`..\\evil.exe`,
		`bin/../../outside.exe`,
		`/etc/passwd`,
		`C:\Windows\System32\cmd.exe`,
	}
	for _, entry := range cases {
		if err := domainplugin.ValidateBundleRelativePath(entry); err == nil {
			t.Fatalf("expected rejection for %q", entry)
		}
	}
}

func TestManifestValidateRejectsEngineEntryTraversal(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.bad",
		Name:    "Bad",
		Version: "1.0.0",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "../evil.exe",
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected manifest validation to reject engine.entry traversal")
	}
}
