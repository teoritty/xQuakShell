package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"xquakshell/internal/infra/opaque"
)

const keyFileName = "server.key"

// keyFile holds the server's secrets. It lives beside the database, never inside it: a stolen
// database or backup without this file cannot be used to guess passwords offline, because the OPAQUE
// key material and OPRF seed are here (spec section 9.4).
type keyFile struct {
	IdentitySeed      []byte `json:"identitySeed"`
	OPAQUEKeyMaterial []byte `json:"opaqueKeyMaterial"`
	FakeAccountSecret []byte `json:"fakeAccountSecret"`
}

var errKeyFileExists = errors.New("xqsyncd: a key file already exists; refusing to overwrite it")

func newKeyFile(rnd io.Reader) (*keyFile, error) {
	k := &keyFile{IdentitySeed: make([]byte, 32), FakeAccountSecret: make([]byte, 32)}
	if _, err := io.ReadFull(rnd, k.IdentitySeed); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rnd, k.FakeAccountSecret); err != nil {
		return nil, err
	}
	skm, err := opaque.GenerateServerKeyMaterial()
	if err != nil {
		return nil, err
	}
	k.OPAQUEKeyMaterial = skm
	return k, nil
}

// writeKeyFile creates the key file with owner-only permissions and refuses to replace an existing
// one: overwriting it would lock every account out, since their OPAQUE records depend on its keys.
func writeKeyFile(dir string, k *keyFile) error {
	raw, err := json.Marshal(k)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, keyFileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return errKeyFileExists
	}
	if err != nil {
		return fmt.Errorf("xqsyncd: create key file: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return fmt.Errorf("xqsyncd: write key file: %w", err)
	}
	return f.Close()
}

func readKeyFile(dir string) (*keyFile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, keyFileName)) // #nosec G304 -- the directory is the operator's own --dir
	if err != nil {
		return nil, fmt.Errorf("xqsyncd: read key file: %w", err)
	}
	var k keyFile
	if err := json.Unmarshal(raw, &k); err != nil {
		return nil, fmt.Errorf("xqsyncd: parse key file: %w", err)
	}
	if len(k.IdentitySeed) != 32 || len(k.FakeAccountSecret) != 32 || len(k.OPAQUEKeyMaterial) == 0 {
		return nil, errors.New("xqsyncd: key file is incomplete")
	}
	return &k, nil
}

var defaultRand io.Reader = rand.Reader
