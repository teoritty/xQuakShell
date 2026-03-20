package putty

import (
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

	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal key: %w", err)
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(block), puttyKey.Comment, nil
}
