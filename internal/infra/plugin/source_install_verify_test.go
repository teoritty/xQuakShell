package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/infra/plugin/bundle"
)

// writePluginTree lays out the smallest tree an install accepts: a manifest, an entry binary, and
// author checksums covering both.
func writePluginTree(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"com.test.verify","name":"V","version":"1.0.0","engine":{"type":"go-binary","entry":"p.exe"}}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("authentic"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}
}

func TestValidateInstalledTreeAcceptsHostWrittenFiles(t *testing.T) {
	dir := t.TempDir()
	writePluginTree(t, dir)

	// Both files are written by the host after the author's SHA256SUMS was produced, so both are
	// legitimately unlisted. Dropping either from the reserved set fails every single install.
	if err := os.WriteFile(filepath.Join(dir, UserInstalledMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, InstallMetaFile), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := validateInstalledTree(dir); err != nil {
		t.Fatalf("validateInstalledTree = %v, want nil; the host's own marker and install metadata "+
			"are unlisted by design and must not count as tampering", err)
	}
}

func TestValidateInstalledTreeRejectsSwappedEntryBinary(t *testing.T) {
	dir := t.TempDir()
	writePluginTree(t, dir)

	// The attack this whole check exists for: same name, same manifest, different code.
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("swapped"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := validateInstalledTree(dir); err == nil {
		t.Fatal("validateInstalledTree = nil, want error; an entry binary whose content no longer " +
			"matches SHA256SUMS must never be accepted as installed")
	}
}

func TestValidateInstalledTreeRejectsUnlistedFile(t *testing.T) {
	dir := t.TempDir()
	writePluginTree(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "extra.dll"), []byte("smuggled"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := validateInstalledTree(dir); err == nil {
		t.Fatal("validateInstalledTree = nil, want error; a file the author never listed must not " +
			"ride along into an installed tree")
	}
}

func TestValidateInstalledTreeRejectsMissingChecksums(t *testing.T) {
	dir := t.TempDir()
	writePluginTree(t, dir)

	if err := os.Remove(filepath.Join(dir, bundle.ChecksumsFile)); err != nil {
		t.Fatal(err)
	}

	if err := validateInstalledTree(dir); err == nil {
		t.Fatal("validateInstalledTree = nil, want error; a tree with no SHA256SUMS carries no " +
			"statement about its own contents")
	}
}

// TestInstallFromSourceValidatesInstalledTree drives the real install path end to end. It is the
// guard against the reserved-name regression: the host writes UserInstalledMarker into destDir
// between the copy and the check, so a validation that did not exempt it would reject every
// install, and no unit test of validateInstalledTree alone would notice.
func TestInstallFromSourceValidatesInstalledTree(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	writePluginTree(t, src)
	dataRoot := t.TempDir()

	installed, err := InstallFromSource(src, dataRoot)
	if err != nil {
		t.Fatalf("InstallFromSource = %v, want nil for a tree whose checksums match", err)
	}

	if err := validateInstalledTree(installed.RootDir); err != nil {
		t.Fatalf("installed tree fails its own validation: %v", err)
	}
}
