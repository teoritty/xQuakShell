package domain

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestHostKeyInfoFromPublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	info := HostKeyInfoFromPublicKey("example.com:22", pk)
	if info.Host != "example.com:22" {
		t.Fatalf("Host: %q", info.Host)
	}
	if info.KeyType != pk.Type() {
		t.Fatalf("KeyType: %q", info.KeyType)
	}
	if info.Fingerprint == "" || info.KeyBase64 == "" {
		t.Fatal("expected fingerprint and key base64")
	}
}
