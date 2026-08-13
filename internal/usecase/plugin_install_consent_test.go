package usecase

import (
	"errors"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// removeRecorder stands in for the portable data store so a test can see whether a refused install
// actually deleted what it had already written.
type removeRecorder struct {
	removed []string
	err     error
}

func (r *removeRecorder) Remove(path string) error {
	r.removed = append(r.removed, path)
	return r.err
}
func (r *removeRecorder) DataRoot() string                     { return "" }
func (r *removeRecorder) TempDir() string                      { return "" }
func (r *removeRecorder) EnsureTempDir() (string, error)       { return "", nil }
func (r *removeRecorder) ResolvePath(p string) (string, error) { return p, nil }
func (r *removeRecorder) ReadFile(string) ([]byte, error)      { return nil, nil }

func multiSessionManifest() domainplugin.Manifest {
	m := domainplugin.Manifest{ID: "com.example.tool"}
	m.Capabilities.Session = &domainplugin.SessionCaps{AllowMultiSession: true}
	return m
}

// consentError is the single place the three install-time consents are decided, so each one is
// asserted individually: a consent added to the install flow without a branch here would be a
// consent nobody has to give.
func TestConsentErrorRequiresEachConsent(t *testing.T) {
	multi := multiSessionManifest()
	if err := consentError(multi, false, true, true); err == nil {
		t.Error("multi-session install proceeded without its consent")
	}
	if err := consentError(multi, true, true, true); err != nil {
		t.Errorf("consentError = %v, want nil once multi-session consent is given", err)
	}
}

func TestConsentErrorPassesAPlainManifest(t *testing.T) {
	if err := consentError(domainplugin.Manifest{ID: "com.example.plain"}, false, false, false); err != nil {
		t.Errorf("consentError = %v, want nil; a manifest asking for none of these needs no consent", err)
	}
}

// The hole this closes: installBundle writes the plugin into installRoot before consent is
// checked, and Discovery.Discover() adopts whatever it finds there on the next start. Returning
// the refusal without removing the files turned "the user declined" into "the user declined, and
// it installed anyway after a restart".
func TestDiscardInstalledFilesRemovesTheTree(t *testing.T) {
	store := &removeRecorder{}
	m := &PluginManager{portableData: store}

	m.discardInstalledFiles(domainplugin.InstalledPlugin{
		Manifest: multiSessionManifest(),
		RootDir:  "/plugins/com.example.tool",
	})

	if len(store.removed) != 1 || store.removed[0] != "/plugins/com.example.tool" {
		t.Fatalf("removed = %v, want the installed root dir", store.removed)
	}
}

// A rollback that cannot run must still leave the refusal as the outcome the caller sees, and must
// not panic on a manager wired without a data store.
func TestDiscardInstalledFilesToleratesAMissingStore(t *testing.T) {
	m := &PluginManager{}

	m.discardInstalledFiles(domainplugin.InstalledPlugin{
		Manifest: multiSessionManifest(),
		RootDir:  "/plugins/com.example.tool",
	})
}

func TestDiscardInstalledFilesToleratesARemoveFailure(t *testing.T) {
	store := &removeRecorder{err: errors.New("permission denied")}
	m := &PluginManager{portableData: store}

	m.discardInstalledFiles(domainplugin.InstalledPlugin{
		Manifest: multiSessionManifest(),
		RootDir:  "/plugins/com.example.tool",
	})

	if len(store.removed) != 1 {
		t.Errorf("removed = %v, want the removal to have been attempted", store.removed)
	}
}

// The refusal has to name which consent was missing, or the user is told "no" with no way to know
// what they would be agreeing to.
func TestConsentErrorNamesTheMissingConsent(t *testing.T) {
	err := consentError(multiSessionManifest(), false, true, true)
	if err == nil || !strings.Contains(err.Error(), "multi-session") {
		t.Fatalf("consentError = %v, want it to name multi-session", err)
	}
}

// installedFrom builds a manager whose bundle installer reports the plugin as already written to
// disk, which is what the real one does before consent is ever consulted.
func installedFrom(manifest domainplugin.Manifest, store *removeRecorder) *PluginManager {
	installed := domainplugin.InstalledPlugin{
		Manifest: manifest,
		RootDir:  "/plugins/" + manifest.ID,
		Source:   domainplugin.SourceUser,
	}
	return &PluginManager{
		registry:      NewPluginRegistry(),
		portableData:  store,
		installBundle: func(string, string) (domainplugin.InstalledPlugin, error) { return installed, nil },
	}
}

// The two helpers being correct is not the claim that matters. Install has to call them: the files
// are on disk by the time consent is checked, so a refusal that does not roll back leaves the
// plugin for Discovery to adopt on the next start.
func TestInstallRemovesTheFilesWhenConsentIsMissing(t *testing.T) {
	store := &removeRecorder{}
	m := installedFrom(multiSessionManifest(), store)

	_, err := m.Install("/tmp/bundle.xqsp", domainplugin.InstallTrustPolicy{}, false, true, true)

	if err == nil {
		t.Fatal("Install succeeded without multi-session consent")
	}
	if len(store.removed) != 1 || store.removed[0] != "/plugins/com.example.tool" {
		t.Fatalf("removed = %v, want the refused install's root dir", store.removed)
	}
	if _, getErr := m.registry.Get("com.example.tool"); getErr == nil {
		t.Error("a refused install was registered")
	}
}

// Each consent gets the same treatment, so a branch that forgets the rollback is caught by name.
func TestInstallRemovesTheFilesForEveryMissingConsent(t *testing.T) {
	execManifest := domainplugin.Manifest{ID: "com.example.tool"}
	execManifest.Capabilities.Channel = &domainplugin.ChannelCaps{
		Purposes:     []string{domainplugin.PurposeExec},
		ExecCommands: []domainplugin.ExecCommandTemplate{{Argv: []string{"ls"}}},
	}
	netManifest := domainplugin.Manifest{ID: "com.example.tool"}
	netManifest.Capabilities.Network = &domainplugin.NetworkCaps{AllowArbitraryOutbound: true}

	cases := []struct {
		name     string
		manifest domainplugin.Manifest
		multi    bool
		network  bool
		exec     bool
	}{
		{"exec channel", execManifest, true, true, false},
		{"arbitrary network", netManifest, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &removeRecorder{}
			m := installedFrom(tc.manifest, store)

			if _, err := m.Install("/tmp/bundle.xqsp", domainplugin.InstallTrustPolicy{}, tc.multi, tc.network, tc.exec); err == nil {
				t.Fatalf("Install succeeded without %s consent", tc.name)
			}
			if len(store.removed) != 1 {
				t.Errorf("removed = %v, want the refused install's root dir", store.removed)
			}
		})
	}
}

// A granted install must keep its files, or consent would be the thing that deletes the plugin.
func TestInstallKeepsTheFilesWhenConsentIsGiven(t *testing.T) {
	store := &removeRecorder{}
	m := installedFrom(multiSessionManifest(), store)

	if _, err := m.Install("/tmp/bundle.xqsp", domainplugin.InstallTrustPolicy{}, true, true, true); err != nil {
		t.Fatalf("Install err = %v, want nil", err)
	}
	if len(store.removed) != 0 {
		t.Errorf("removed = %v, want nothing removed on a consented install", store.removed)
	}
}
