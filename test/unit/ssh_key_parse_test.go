package unit

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
	infrassh "ssh-client/internal/infra/ssh"
)

func generateRSAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
}

func generateECDSAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal ecdsa key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

func generateEd25519PEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal ed25519 key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func generateEncryptedOpenSSHPEM(t *testing.T, passphrase string) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	_ = signer
	pemBytes, err := gossh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	if err != nil {
		t.Fatalf("marshal encrypted key: %v", err)
	}
	return pem.EncodeToMemory(pemBytes)
}

func TestParseRSAKey(t *testing.T) {
	pemData := generateRSAPEM(t)
	signer, err := infrassh.ParseKeyWithPassphrase(pemData, "")
	if err != nil {
		t.Fatalf("parse RSA key: %v", err)
	}
	if signer == nil {
		t.Fatal("signer is nil")
	}
	if signer.PublicKey().Type() != "ssh-rsa" {
		t.Errorf("expected ssh-rsa, got %s", signer.PublicKey().Type())
	}
}

func TestParseECDSAKey(t *testing.T) {
	pemData := generateECDSAPEM(t)
	signer, err := infrassh.ParseKeyWithPassphrase(pemData, "")
	if err != nil {
		t.Fatalf("parse ECDSA key: %v", err)
	}
	if signer == nil {
		t.Fatal("signer is nil")
	}
	pubType := signer.PublicKey().Type()
	if pubType != "ecdsa-sha2-nistp256" {
		t.Errorf("expected ecdsa-sha2-nistp256, got %s", pubType)
	}
}

func TestParseEd25519Key(t *testing.T) {
	pemData := generateEd25519PEM(t)
	signer, err := infrassh.ParseKeyWithPassphrase(pemData, "")
	if err != nil {
		t.Fatalf("parse ed25519 key: %v", err)
	}
	if signer == nil {
		t.Fatal("signer is nil")
	}
	if signer.PublicKey().Type() != "ssh-ed25519" {
		t.Errorf("expected ssh-ed25519, got %s", signer.PublicKey().Type())
	}
}

func TestParseEncryptedKeyWithoutPassphrase(t *testing.T) {
	pemData := generateEncryptedOpenSSHPEM(t, "test-passphrase")

	_, err := infrassh.ParseKeyWithPassphrase(pemData, "")
	if err == nil {
		t.Fatal("expected ErrPassphraseRequired, got nil")
	}
	if !errors.Is(err, domain.ErrPassphraseRequired) {
		t.Errorf("expected ErrPassphraseRequired, got: %v", err)
	}
}

func TestParseEncryptedKeyWithCorrectPassphrase(t *testing.T) {
	passphrase := "my-secret-passphrase"
	pemData := generateEncryptedOpenSSHPEM(t, passphrase)

	signer, err := infrassh.ParseKeyWithPassphrase(pemData, passphrase)
	if err != nil {
		t.Fatalf("parse encrypted key with passphrase: %v", err)
	}
	if signer == nil {
		t.Fatal("signer is nil")
	}
}

func TestParseEncryptedKeyWithWrongPassphrase(t *testing.T) {
	pemData := generateEncryptedOpenSSHPEM(t, "correct")

	_, err := infrassh.ParseKeyWithPassphrase(pemData, "wrong")
	if err == nil {
		t.Fatal("expected error for wrong passphrase, got nil")
	}
}

func TestPassphraseCache(t *testing.T) {
	cache := infrassh.NewPassphraseCache()

	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected miss for empty cache")
	}

	cache.Set("key1", "secret")
	val, ok := cache.Get("key1")
	if !ok || val != "secret" {
		t.Errorf("expected 'secret', got '%s' (ok=%v)", val, ok)
	}

	cache.Clear()
	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected miss after clear")
	}
}
