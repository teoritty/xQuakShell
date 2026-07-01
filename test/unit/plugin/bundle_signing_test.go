package plugin_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

func fakeChecksumsHex(seed byte) string {
	b := make([]byte, sha256.Size)
	for i := range b {
		b[i] = seed
	}
	return hex.EncodeToString(b)
}

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
	digest := fakeChecksumsHex(0xab)

	sig, err := domainplugin.SignManifest(m, digest, priv)
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = sig

	ok, err := domainplugin.VerifyManifestSignature(m, digest, []ed25519.PublicKey{pub})
	if err != nil || !ok {
		t.Fatalf("expected trusted signature, ok=%v err=%v", ok, err)
	}

	wrongPub, _, _ := ed25519.GenerateKey(nil)
	ok, err = domainplugin.VerifyManifestSignature(m, digest, []ed25519.PublicKey{wrongPub})
	if err != nil || ok {
		t.Fatalf("expected untrusted signature, ok=%v err=%v", ok, err)
	}
}

func TestEvaluateInstallTrust(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := domainplugin.Manifest{ID: "a", Name: "A", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "a.exe"}}
	digest := fakeChecksumsHex(0x01)
	sig, _ := domainplugin.SignManifest(m, digest, priv)
	m.Signature = sig

	trust, err := domainplugin.EvaluateInstallTrust(m, digest, domainplugin.InstallTrustPolicy{TrustedKeys: []ed25519.PublicKey{pub}})
	if err != nil || !trust.SignatureVerified {
		t.Fatalf("expected verified trust: %+v err=%v", trust, err)
	}

	trust, err = domainplugin.EvaluateInstallTrust(domainplugin.Manifest{ID: "b", Name: "B", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "b.exe"}},
		"", domainplugin.InstallTrustPolicy{RequireSigned: true})
	if err == nil {
		t.Fatal("expected require signed error")
	}

	_, err = domainplugin.EvaluateInstallTrust(m, "", domainplugin.InstallTrustPolicy{RequireSigned: true})
	if err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("expected SHA256SUMS error, got %v", err)
	}
}

func TestManifestSignatureRejectsChecksumsTamper(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	writeTestPluginTree(t, dir)
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}

	digest, err := bundle.ChecksumsDigest(dir)
	if err != nil || digest == "" {
		t.Fatalf("checksums digest: %q err=%v", digest, err)
	}

	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}

	sig, err := domainplugin.SignManifest(manifest, digest, priv)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = sig

	ok, err := domainplugin.VerifyManifestSignature(manifest, digest, []ed25519.PublicKey{pub})
	if err != nil || !ok {
		t.Fatalf("expected valid signature before tamper, ok=%v err=%v", ok, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "plugin.exe"), []byte("tampered-binary"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}
	if err := bundle.ValidateChecksums(dir); err != nil {
		t.Fatalf("recalculated checksums should validate tree: %v", err)
	}

	newDigest, err := bundle.ChecksumsDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if newDigest == digest {
		t.Fatal("expected digest to change after tamper")
	}

	ok, err = domainplugin.VerifyManifestSignature(manifest, newDigest, []ed25519.PublicKey{pub})
	if err != nil || ok {
		t.Fatalf("expected signature rejected after tamper, ok=%v err=%v", ok, err)
	}

	trust, err := domainplugin.EvaluateInstallTrust(manifest, newDigest, domainplugin.InstallTrustPolicy{
		TrustedKeys:   []ed25519.PublicKey{pub},
		RequireSigned: true,
	})
	if err == nil || trust.SignatureVerified {
		t.Fatalf("expected install trust failure after tamper: trust=%+v err=%v", trust, err)
	}
}

func TestSignManifestRejectsMissingChecksums(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := domainplugin.Manifest{
		ID: "com.test", Name: "T", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "t.exe"},
	}
	if _, err := domainplugin.SignManifest(m, "", priv); err == nil {
		t.Fatal("expected error signing without checksums digest")
	}
}

func TestUnsignedPluginWithoutChecksumsLoadsSuccessfully(t *testing.T) {
	dir := t.TempDir()
	writeTestPluginTree(t, dir)

	digest, err := bundle.ChecksumsDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if digest != "" {
		t.Fatalf("expected empty digest without SHA256SUMS, got %q", digest)
	}

	plugin, err := infraplugin.LoadPluginDir(dir)
	if err != nil {
		t.Fatalf("load unsigned plugin without checksums: %v", err)
	}
	if plugin.ChecksumsDigest != "" {
		t.Fatalf("expected empty ChecksumsDigest, got %q", plugin.ChecksumsDigest)
	}

	trust, err := domainplugin.EvaluateInstallTrust(plugin.Manifest, "", domainplugin.InstallTrustPolicy{})
	if err != nil || !trust.UnsignedWarning {
		t.Fatalf("expected unsigned warning: trust=%+v err=%v", trust, err)
	}

	_, err = domainplugin.EvaluateInstallTrust(plugin.Manifest, "", domainplugin.InstallTrustPolicy{RequireSigned: true})
	if err == nil {
		t.Fatal("expected require signed error")
	}
}

func TestSignedPluginWithoutChecksumsFailsWithClearError(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	m := domainplugin.Manifest{
		ID: "com.signed", Name: "S", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "s.exe"},
	}
	digest := fakeChecksumsHex(0x02)
	sig, err := domainplugin.SignManifest(m, digest, priv)
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = sig

	_, err = domainplugin.EvaluateInstallTrust(m, "", domainplugin.InstallTrustPolicy{RequireSigned: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("expected SHA256SUMS error, got %v", err)
	}
	if strings.Contains(err.Error(), "signature not trusted") {
		t.Fatalf("should not report signature not trusted: %v", err)
	}
}

func TestOldSignatureFormatDetected(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := domainplugin.Manifest{
		ID: "com.old", Name: "Old", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "old.exe"},
	}
	copy := m
	payload, err := json.Marshal(copy)
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload))

	digest := fakeChecksumsHex(0x03)
	ok, err := domainplugin.VerifyManifestSignature(m, digest, []ed25519.PublicKey{pub})
	if ok || !errors.Is(err, domainplugin.ErrSignatureFormatOutdated) {
		t.Fatalf("expected outdated format error, ok=%v err=%v", ok, err)
	}

	_, err = domainplugin.EvaluateInstallTrust(m, digest, domainplugin.InstallTrustPolicy{
		TrustedKeys:   []ed25519.PublicKey{pub},
		RequireSigned: true,
	})
	if err == nil || !strings.Contains(err.Error(), "outdated") {
		t.Fatalf("expected outdated format in trust eval, got %v", err)
	}
}

func TestCRLFNormalization(t *testing.T) {
	dir := t.TempDir()
	writeTestPluginTree(t, dir)
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatal(err)
	}

	lfDigest, err := bundle.ChecksumsDigest(dir)
	if err != nil {
		t.Fatal(err)
	}

	sumPath := filepath.Join(dir, bundle.ChecksumsFile)
	data, err := os.ReadFile(sumPath)
	if err != nil {
		t.Fatal(err)
	}
	crlf := bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
	if err := os.WriteFile(sumPath, crlf, 0644); err != nil {
		t.Fatal(err)
	}

	crlfDigest, err := bundle.ChecksumsDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if crlfDigest != lfDigest {
		t.Fatalf("CRLF normalization failed: lf=%s crlf=%s", lfDigest, crlfDigest)
	}
}

func TestSignedPluginWithoutChecksumsFileFailsAtLoad(t *testing.T) {
	dir := t.TempDir()
	writeTestPluginTree(t, dir)

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := fakeChecksumsHex(0x04)
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	sig, err := domainplugin.SignManifest(manifest, digest, priv)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = sig
	out, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		t.Fatal(err)
	}

	_, err = infraplugin.LoadPluginDir(dir)
	if err == nil {
		t.Fatal("expected load failure for signed plugin without SHA256SUMS")
	}
	if !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("expected SHA256SUMS in error, got %v", err)
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
