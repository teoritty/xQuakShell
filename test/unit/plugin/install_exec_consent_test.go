package plugin_test

import (
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

func execConsentManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID: "com.test.exec", Name: "Exec", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Channel: &domainplugin.ChannelCaps{
				Purposes: []string{domainplugin.PurposeExec},
				ExecCommands: []domainplugin.ExecCommandTemplate{
					{Argv: []string{"uname", "-a"}},
				},
			},
		},
	}
}

func execConsentManager(t *testing.T) *usecase.PluginManager {
	t.Helper()
	return usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    usecase.NewPluginRegistry(),
		InstallRoot: t.TempDir(),
		LoadBundle: func(sourcePath string) (domainplugin.InstalledPlugin, error) {
			return domainplugin.InstalledPlugin{Manifest: execConsentManifest(), RootDir: sourcePath}, nil
		},
		InstallBundle: func(sourcePath, dataRoot string) (domainplugin.InstalledPlugin, error) {
			return domainplugin.InstalledPlugin{Manifest: execConsentManifest(), RootDir: dataRoot}, nil
		},
	})
}

// TestInstallRequiresExecConsent pins the install-time gate the exec backend's consentGranted
// traces to. A plugin declaring the exec channel purpose asks to run commands over the user's
// authenticated SSH session; without an explicit grant the install must be refused outright, which
// is what makes "installed and declares exec" a sound source of consent at channel.open time
// (ADR-011 readiness D3) and why no grant needs persisting.
func TestInstallRequiresExecConsent(t *testing.T) {
	_, err := execConsentManager(t).Install(t.TempDir(), domainplugin.InstallTrustPolicy{}, true, true, false)
	if err == nil || !strings.Contains(err.Error(), "exec channel consent required") {
		t.Fatalf("expected exec consent error, got %v", err)
	}
}

// TestInstallWithExecConsentSucceeds is the other half: the gate must not refuse a granted install.
func TestInstallWithExecConsentSucceeds(t *testing.T) {
	installed, err := execConsentManager(t).Install(t.TempDir(), domainplugin.InstallTrustPolicy{}, true, true, true)
	if err != nil {
		t.Fatalf("install with exec consent: %v", err)
	}
	if !installed.Manifest.RequiresChannelExecConsent() {
		t.Fatal("manifest declaring the exec purpose must report requiring exec consent")
	}
}
