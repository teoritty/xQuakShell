package wails

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

func consentInstallManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID: "com.example.sync", Name: "Sync", Version: "1",
		BundleFormat: domainplugin.CurrentBundleFormat,
		Engine:       domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Auth: &domainplugin.AuthCaps{Provider: true},
		},
	}
}

func consentInstallManager(t *testing.T) *usecase.PluginManager {
	t.Helper()
	manifest := consentInstallManifest()
	return usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry: usecase.NewPluginRegistry(),
		LoadBundle: func(sourcePath string) (domainplugin.InstalledPlugin, error) {
			return domainplugin.InstalledPlugin{Manifest: manifest, RootDir: sourcePath}, nil
		},
		InstallBundle: func(_, dataRoot string) (domainplugin.InstalledPlugin, error) {
			return domainplugin.InstalledPlugin{Manifest: manifest, RootDir: dataRoot}, nil
		},
		InstallRoot: t.TempDir(),
	})
}

// Installing is the only moment consent is collected. If this call went missing the plugin would be
// installed with no recorded permissions and refused everything it asked for - silently, because a
// missing grant reads as "not granted" rather than as an error, so nothing in the install would
// report a problem and the plugin would simply not work.
func TestInstallingAPluginRecordsTheConsentTheUserGave(t *testing.T) {
	var seen *domainplugin.ConsentFlags
	var seenID string
	api := &AppAPI{
		plugins: consentInstallManager(t),
		pluginConsentRecorder: func(manifest *domainplugin.Manifest, consent domainplugin.ConsentFlags) error {
			seenID = manifest.ID
			seen = &consent
			return nil
		},
	}

	if _, err := api.InstallPlugin(t.TempDir(), false, true, false, false, false, false); err != nil {
		t.Fatalf("InstallPlugin err = %v, want nil", err)
	}

	if seen == nil {
		t.Fatal("the install recorded no consent")
	}
	if seenID != "com.example.sync" {
		t.Errorf("consent was recorded for %q", seenID)
	}
	if !seen.AuthProvider {
		t.Error("the capability the user agreed to did not reach the recorder")
	}
	if seen.SecretAccess || seen.TunnelProvider || seen.MultiSession || seen.ArbitraryNetwork || seen.ExecChannel {
		t.Errorf("a capability the user declined reached the recorder: %+v", *seen)
	}
}

// A recorder that fails must fail the install. Reporting success would leave a plugin installed and
// running with no record of what it was allowed.
func TestInstallingAPluginFailsWhenConsentCannotBeRecorded(t *testing.T) {
	api := &AppAPI{
		plugins: consentInstallManager(t),
		pluginConsentRecorder: func(*domainplugin.Manifest, domainplugin.ConsentFlags) error {
			return errPluginManagerUnavailable
		},
	}

	if _, err := api.InstallPlugin(t.TempDir(), false, true, false, false, false, false); err == nil {
		t.Fatal("the install reported success although consent was never recorded")
	}
}
