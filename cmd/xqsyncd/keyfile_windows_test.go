//go:build windows

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// On Windows the key file is sealed with DPAPI (spec 9.4): a copy taken off the machine - a backup of
// the data directory - does not carry the keys in the clear.
func TestKeyFileIsSealedWithDPAPI(t *testing.T) {
	dir := t.TempDir()
	k, err := newKeyFile(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeKeyFile(dir, k); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, keyFileName))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := json.Marshal(k)
	if bytes.Contains(raw, plain) || json.Valid(raw) || bytes.Contains(raw, []byte("identitySeed")) {
		t.Fatal("the key file holds the keys in the clear")
	}
	got, err := readKeyFile(dir)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got.IdentitySeed, k.IdentitySeed) || !bytes.Equal(got.OPAQUEKeyMaterial, k.OPAQUEKeyMaterial) {
		t.Fatal("the sealed key file does not read back to the same keys")
	}
}

// A key file written on Linux, where there is no DPAPI, is plain JSON; moving a server to Windows
// must not lock every account out.
func TestPlainKeyFileStillReads(t *testing.T) {
	dir := t.TempDir()
	k, _ := newKeyFile(rand.Reader)
	plain, _ := json.Marshal(k)
	if err := os.WriteFile(filepath.Join(dir, keyFileName), plain, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readKeyFile(dir)
	if err != nil || !bytes.Equal(got.IdentitySeed, k.IdentitySeed) {
		t.Fatalf("plain key file: %v", err)
	}
}
