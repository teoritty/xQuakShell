package plugin_test

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

func TestManifestSignAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	m := domainplugin.Manifest{
		ID:      "com.test.sign",
		Name:    "Sign Test",
		Version: "1.0.0",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "test.exe",
		},
	}

	sig, err := domainplugin.SignManifest(m, priv)
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = sig

	ok, err := domainplugin.VerifyManifestSignature(m, []ed25519.PublicKey{pub})
	if err != nil || !ok {
		t.Fatalf("expected trusted signature, ok=%v err=%v", ok, err)
	}

	wrongPub, _, _ := ed25519.GenerateKey(nil)
	ok, err = domainplugin.VerifyManifestSignature(m, []ed25519.PublicKey{wrongPub})
	if err != nil || ok {
		t.Fatalf("expected untrusted signature, ok=%v err=%v", ok, err)
	}
}

func TestEvaluateInstallTrust(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := domainplugin.Manifest{ID: "a", Name: "A", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"}}
	sig, _ := domainplugin.SignManifest(m, priv)
	m.Signature = sig

	trust, err := domainplugin.EvaluateInstallTrust(m, domainplugin.InstallTrustPolicy{TrustedKeys: []ed25519.PublicKey{pub}})
	if err != nil || !trust.SignatureVerified {
		t.Fatalf("expected verified trust: %+v err=%v", trust, err)
	}

	trust, err = domainplugin.EvaluateInstallTrust(domainplugin.Manifest{ID: "b", Name: "B", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "b.exe"}},
		domainplugin.InstallTrustPolicy{RequireSigned: true})
	if err == nil {
		t.Fatal("expected require signed error")
	}
}

func TestBundlePackValidateExtract(t *testing.T) {
	dir := t.TempDir()
	writeTestPluginTree(t, dir)

	out := filepath.Join(t.TempDir(), "test.xqs-plugin")
	if err := bundle.Pack(dir, out); err != nil {
		t.Fatal(err)
	}
	if err := infraplugin.ValidatePluginSource(out); err != nil {
		t.Fatal(err)
	}

	extractDir := t.TempDir()
	if err := bundle.Extract(out, extractDir); err != nil {
		t.Fatal(err)
	}
	if err := bundle.ValidateChecksums(extractDir); err != nil {
		t.Fatal(err)
	}
}

func writeTestPluginTree(t *testing.T, dir string) {
	t.Helper()
	manifest := domainplugin.Manifest{
		ID:      "com.test.bundle",
		Name:    "Bundle",
		Version: "1.0.0",
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "plugin.exe",
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.exe"), []byte("fake-binary"), 0644); err != nil {
		t.Fatal(err)
	}
}
