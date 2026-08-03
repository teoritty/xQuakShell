package putty

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/kayrus/putty"
)

// PPKToPEM converts a PuTTY PPK file content to PEM format.
// passphrase is required if the PPK is encrypted.
func PPKToPEM(ppkContent []byte, passphrase string) ([]byte, string, error) {
	puttyKey, err := putty.New(ppkContent)
	if err != nil {
		return nil, "", fmt.Errorf("parse ppk: %w", err)
	}

	var pass []byte
	if puttyKey.Encryption != "none" && puttyKey.Encryption != "" {
		pass = []byte(passphrase)
	}

	privateKey, err := puttyKey.ParseRawPrivateKey(pass)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt ppk: %w", err)
	}

	der, err := x509.MarshalPKCS8PrivateKey(normalizeKey(privateKey))
	if err != nil {
		return nil, "", fmt.Errorf("marshal key: %w", err)
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(block), puttyKey.Comment, nil
}

// normalizeKey adapts the parser's return type to what x509 marshalling
// accepts.
//
// The PPK parser hands back a pointer for every algorithm. x509 takes
// *rsa.PrivateKey and *ecdsa.PrivateKey as pointers, but wants
// ed25519.PrivateKey by value — the type is already a slice — and rejects the
// pointer form outright. Without this, every Ed25519 .ppk failed to import
// with "unknown key type while marshaling PKCS#8", which is the default key
// type PuTTYgen has produced since 0.72.
func normalizeKey(key any) any {
	if ed, ok := key.(*ed25519.PrivateKey); ok && ed != nil {
		return *ed
	}
	return key
}
