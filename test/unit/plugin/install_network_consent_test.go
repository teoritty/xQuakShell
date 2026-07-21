package plugin_test

import (
	"context"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

type memVault struct {
	data domain.VaultData
}

func (m *memVault) Unlock(_ context.Context, _ string) error { return nil }
func (m *memVault) Lock()                                    {}
func (m *memVault) GetData() (*domain.VaultData, error)      { return &m.data, nil }
func (m *memVault) UpdateData(_ context.Context, fn func(*domain.VaultData) error) error {
	return fn(&m.data)
}
func (m *memVault) IsUnlocked() bool { return true }

func TestInstallPreviewArbitraryNetworkWarning(t *testing.T) {
	m := baseManifest()
	m.Capabilities.Network = &domainplugin.NetworkCaps{AllowArbitraryOutbound: true}
	trust, err := domainplugin.EvaluateInstallTrust(m, "", domainplugin.InstallTrustPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if !trust.ArbitraryNetworkWarning {
		t.Fatal("expected trust arbitrary network warning")
	}
	preview := usecase.InstallPreview{
		ArbitraryNetworkWarning: m.RequiresArbitraryNetworkWarning() || trust.ArbitraryNetworkWarning,
	}
	if !preview.ArbitraryNetworkWarning {
		t.Fatal("expected preview arbitrary network warning")
	}
}

func TestInstallRequiresArbitraryNetworkConsent(t *testing.T) {
	registry := usecase.NewPluginRegistry()
	mgr := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    registry,
		InstallRoot: t.TempDir(),
		LoadBundle: func(sourcePath string) (domainplugin.InstalledPlugin, error) {
			return domainplugin.InstalledPlugin{
				Manifest: domainplugin.Manifest{
					ID: "com.test.arb", Name: "Arb", Version: "1",
					Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
					Capabilities: domainplugin.CapabilitySet{
						Network: &domainplugin.NetworkCaps{AllowArbitraryOutbound: true},
					},
				},
				RootDir: sourcePath,
			}, nil
		},
		InstallBundle: func(sourcePath, dataRoot string) (domainplugin.InstalledPlugin, error) {
			return domainplugin.InstalledPlugin{
				Manifest: domainplugin.Manifest{
					ID: "com.test.arb", Name: "Arb", Version: "1",
					Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
					Capabilities: domainplugin.CapabilitySet{
						Network: &domainplugin.NetworkCaps{AllowArbitraryOutbound: true},
					},
				},
				RootDir: dataRoot,
			}, nil
		},
	})
	_, err := mgr.Install(t.TempDir(), domainplugin.InstallTrustPolicy{}, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "arbitrary network access consent required") {
		t.Fatalf("expected consent error, got %v", err)
	}
}

func TestGrantArbitraryNetworkAccessPersists(t *testing.T) {
	v := &memVault{data: *domain.NewVaultData()}
	settings := usecase.NewPluginVaultSettings(v)
	if err := settings.GrantArbitraryNetworkAccess(context.Background(), "com.test.arb"); err != nil {
		t.Fatal(err)
	}
	if !settings.IsArbitraryNetworkGranted("com.test.arb") {
		t.Fatal("expected arbitrary network grant persisted")
	}
}
