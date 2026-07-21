package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
)

func TestInstallMeta_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	meta := domainplugin.PluginInstallMeta{
		Source:        domainplugin.InstallMetaSourceGitHub,
		RepositoryURL: "https://github.com/user/repo",
		ReleaseTag:    "v1.0.0",
	}
	if err := infraplugin.WriteInstallMeta(dir, meta); err != nil {
		t.Fatal(err)
	}
	got, ok, err := infraplugin.LoadInstallMeta(dir)
	if err != nil || !ok {
		t.Fatalf("load install meta: ok=%v err=%v", ok, err)
	}
	if got.ReleaseTag != "v1.0.0" || got.RepositoryURL != meta.RepositoryURL {
		t.Fatalf("unexpected meta: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, infraplugin.InstallMetaFile)); err != nil {
		t.Fatal(err)
	}
}
