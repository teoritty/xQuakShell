package putty

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
)

// buildTestPPK writes a PPK v2 Ed25519 key, optionally AES-256-CBC encrypted.
//
// The fixture is generated rather than checked in so the test carries no key
// material of its own, and so it exercises the real format rather than one
// frozen sample.
func buildTestPPK(t *testing.T, comment, passphrase string) ([]byte, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	const algo = "ssh-ed25519"
	pubBlob := append(sshString([]byte(algo)), sshString(pub)...)
	privBlob := sshString(priv.Seed())

	encryption := "none"
	if passphrase != "" {
		encryption = "aes256-cbc"
		if pad := len(privBlob) % aes.BlockSize; pad != 0 {
			privBlob = append(privBlob, make([]byte, aes.BlockSize-pad)...)
		}
	}

	cipherKey, macKey := deriveTestKeys([]byte(passphrase))

	// PuTTY decrypts before verifying, so the MAC covers the plaintext blob.
	mac := hmac.New(sha1.New, macKey)
	for _, field := range []string{algo, encryption, comment} {
		_ = binary.Write(mac, binary.BigEndian, uint32(len(field)))
		mac.Write([]byte(field))
	}
	for _, blob := range [][]byte{pubBlob, privBlob} {
		_ = binary.Write(mac, binary.BigEndian, uint32(len(blob)))
		mac.Write(blob)
	}

	stored := privBlob
	if passphrase != "" {
		block, err := aes.NewCipher(cipherKey)
		if err != nil {
			t.Fatalf("aes: %v", err)
		}
		stored = make([]byte, len(privBlob))
		cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(stored, privBlob)
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "PuTTY-User-Key-File-2: %s\n", algo)
	fmt.Fprintf(&out, "Encryption: %s\n", encryption)
	fmt.Fprintf(&out, "Comment: %s\n", comment)
	writeBlob(&out, "Public", pubBlob)
	writeBlob(&out, "Private", stored)
	fmt.Fprintf(&out, "Private-MAC: %s\n", hex.EncodeToString(mac.Sum(nil)))
	return out.Bytes(), priv
}

func writeBlob(out *bytes.Buffer, label string, blob []byte) {
	encoded := base64.StdEncoding.EncodeToString(blob)
	var lines []string
	for len(encoded) > 64 {
		lines = append(lines, encoded[:64])
		encoded = encoded[64:]
	}
	lines = append(lines, encoded)
	fmt.Fprintf(out, "%s-Lines: %d\n", label, len(lines))
	out.WriteString(strings.Join(lines, "\n") + "\n")
}

func sshString(b []byte) []byte {
	out := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	return out
}

// deriveTestKeys mirrors PuTTY's v2 key schedule.
func deriveTestKeys(password []byte) (cipherKey, macKey []byte) {
	h := sha1.New()
	h.Write([]byte("putty-private-key-file-mac-key"))
	if len(password) > 0 {
		h.Write(password)
	}
	macKey = h.Sum(nil)

	var k []byte
	for seq := 0; len(k) < 32; seq++ {
		t := append([]byte{0, 0, 0, byte(seq)}, password...)
		s := sha1.Sum(t)
		k = append(k, s[:]...)
	}
	return k[:32], macKey
}

// Ed25519 has been PuTTYgen's default since 0.72, and the PPK parser returns
// it as *ed25519.PrivateKey while x509 accepts only the value form. Marshalling
// the pointer fails, so this asserts the whole conversion end to end rather
// than just that a key was parsed.
func TestPPKToPEMEd25519Unencrypted(t *testing.T) {
	ppk, want := buildTestPPK(t, "test-key", "")

	pemData, comment, err := PPKToPEM(ppk, "")
	if err != nil {
		t.Fatalf("PPKToPEM: %v", err)
	}

	if comment != "test-key" {
		t.Errorf("comment = %q, want test-key", comment)
	}
	assertPEMMatches(t, pemData, want)
}

func TestPPKToPEMEd25519Encrypted(t *testing.T) {
	ppk, want := buildTestPPK(t, "encrypted-key", "correct horse")

	pemData, _, err := PPKToPEM(ppk, "correct horse")
	if err != nil {
		t.Fatalf("PPKToPEM: %v", err)
	}

	assertPEMMatches(t, pemData, want)
}

func TestPPKToPEMRejectsWrongPassphrase(t *testing.T) {
	ppk, _ := buildTestPPK(t, "encrypted-key", "correct horse")

	if _, _, err := PPKToPEM(ppk, "wrong horse"); err == nil {
		t.Error("a wrong passphrase must not yield a key")
	}
	if _, _, err := PPKToPEM(ppk, ""); err == nil {
		t.Error("an empty passphrase must not decrypt an encrypted key")
	}
}

func TestPPKToPEMRejectsGarbage(t *testing.T) {
	if _, _, err := PPKToPEM([]byte("not a ppk file at all"), ""); err == nil {
		t.Error("garbage input must be rejected")
	}
}

// assertPEMMatches decodes the emitted PEM and checks it carries the same key
// that went in — a conversion that produced a valid but different key would
// otherwise pass unnoticed.
func assertPEMMatches(t *testing.T, pemData []byte, want ed25519.PrivateKey) {
	t.Helper()
	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("output is not valid PEM")
	}
	if block.Type != "PRIVATE KEY" {
		t.Errorf("PEM type = %q, want PRIVATE KEY", block.Type)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("emitted PEM does not parse as PKCS#8: %v", err)
	}
	got, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		t.Fatalf("parsed key type = %T, want ed25519.PrivateKey", parsed)
	}
	if !got.Equal(want) {
		t.Error("round-tripped key does not match the original")
	}
}
