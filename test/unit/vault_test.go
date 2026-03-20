package unit

import (
	"os"
	"path/filepath"
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/vault"
)

func TestVaultEncryptDecryptRoundtrip(t *testing.T) {
	data := domain.NewVaultData()
	data.Folders = []domain.ConnectionFolder{
		{ID: "f1", Name: "Test Folder", ParentID: "", Order: 0},
	}
	data.Connections = []domain.Connection{
		{ID: "c1", FolderID: "f1", Name: "Test", Host: "example.com", Port: 22, User: "root"},
	}
	data.KnownHosts = []string{"example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest"}

	passphrase := "test-master-password"

	ciphertext, err := vault.Encrypt(data, passphrase)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("ciphertext is empty")
	}

	decrypted, err := vault.Decrypt(ciphertext, passphrase)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if decrypted.Version != 1 {
		t.Errorf("version: got %d, want 1", decrypted.Version)
	}
	if len(decrypted.Folders) != 1 || decrypted.Folders[0].Name != "Test Folder" {
		t.Errorf("folders mismatch: %+v", decrypted.Folders)
	}
	if len(decrypted.Connections) != 1 || decrypted.Connections[0].Host != "example.com" {
		t.Errorf("connections mismatch: %+v", decrypted.Connections)
	}
	if len(decrypted.KnownHosts) != 1 {
		t.Errorf("known hosts mismatch: %+v", decrypted.KnownHosts)
	}
}

func TestVaultDecryptWrongPassphrase(t *testing.T) {
	data := domain.NewVaultData()
	ciphertext, err := vault.Encrypt(data, "correct-password")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = vault.Decrypt(ciphertext, "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong passphrase, got nil")
	}
}

func TestVaultFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	passphrase := "file-test-password"

	data := domain.NewVaultData()
	data.Connections = []domain.Connection{
		{ID: "c1", Name: "Server", Host: "10.0.0.1", Port: 22, User: "admin"},
	}

	err := vault.WriteVaultFile(dir, passphrase, data)
	if err != nil {
		t.Fatalf("write vault file: %v", err)
	}

	vaultPath := filepath.Join(dir, "vault.age")
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		t.Fatal("vault.age file not created")
	}

	tmpPath := filepath.Join(dir, "vault.age.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatal("temporary file should be cleaned up after atomic write")
	}

	read, err := vault.ReadVaultFile(dir, passphrase)
	if err != nil {
		t.Fatalf("read vault file: %v", err)
	}

	if len(read.Connections) != 1 || read.Connections[0].Host != "10.0.0.1" {
		t.Errorf("read data mismatch: %+v", read.Connections)
	}
}

func TestVaultReadNonExistentReturnsEmpty(t *testing.T) {
	dir := t.TempDir()

	data, err := vault.ReadVaultFile(dir, "any-password")
	if err != nil {
		t.Fatalf("expected nil error for non-existent file, got: %v", err)
	}
	if data.Version != 1 {
		t.Errorf("expected version 1, got %d", data.Version)
	}
	if len(data.Connections) != 0 {
		t.Errorf("expected empty connections, got %d", len(data.Connections))
	}
}

func TestVaultIdentitiesStorage(t *testing.T) {
	data := domain.NewVaultData()
	data.Identities["key1"] = domain.SSHIdentity{
		ID: "key1", Comment: "test key", KeyType: "ed25519", Encrypted: false,
	}
	data.KeyBlobs["key1"] = domain.IdentityBlob{
		PEMData: []byte("fake-pem-data"),
	}

	passphrase := "identity-test"
	ciphertext, err := vault.Encrypt(data, passphrase)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	decrypted, err := vault.Decrypt(ciphertext, passphrase)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	ident, ok := decrypted.Identities["key1"]
	if !ok {
		t.Fatal("identity key1 not found")
	}
	if ident.Comment != "test key" {
		t.Errorf("identity comment: got %s, want 'test key'", ident.Comment)
	}

	blob, ok := decrypted.KeyBlobs["key1"]
	if !ok {
		t.Fatal("key blob key1 not found")
	}
	if string(blob.PEMData) != "fake-pem-data" {
		t.Errorf("key blob data mismatch: %s", blob.PEMData)
	}
}
