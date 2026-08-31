package unit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
)

// writeVault puts data on disk under passphrase, through the same two calls VaultRepo.Create makes.
// Going through the real API rather than a fixture is what keeps these tests honest about the
// format actually shipped.
func writeVault(t *testing.T, dir, passphrase string, data *domain.VaultData) {
	t.Helper()
	session, err := vault.CreateSession(dir, passphrase)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := session.Save(data); err != nil {
		t.Fatalf("save vault: %v", err)
	}
}

func openVault(t *testing.T, dir, passphrase string) *domain.VaultData {
	t.Helper()
	_, data, method, err := vault.Open(dir, passphrase)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	if method != domain.UnlockByPassword {
		t.Fatalf("method = %v, want UnlockByPassword; a password must never be taken for a recovery key", method)
	}
	return data
}

func TestVaultEncryptDecryptRoundtrip(t *testing.T) {
	dir := t.TempDir()
	data := domain.NewVaultData()
	data.Folders = []domain.ConnectionFolder{
		{ID: "f1", Name: "Test Folder", ParentID: "", Order: 0},
	}
	data.Connections = []domain.Connection{
		{
			ID: "c1", FolderID: "f1", Name: "Test", Host: "example.com", Port: 22,
			Users:         []domain.ConnectionUser{{ID: "u1", Username: "root", Auth: domain.AuthMethodKey}},
			DefaultUserID: "u1",
		},
	}
	data.KnownHosts = []string{"example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest"}

	writeVault(t, dir, "test-master-password", data)
	decrypted := openVault(t, dir, "test-master-password")

	if decrypted.Version != domain.CurrentVaultVersion {
		t.Errorf("version: got %d, want %d", decrypted.Version, domain.CurrentVaultVersion)
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
	dir := t.TempDir()
	writeVault(t, dir, "correct-password", domain.NewVaultData())

	if _, _, _, err := vault.Open(dir, "wrong-password"); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("got %v, want ErrVaultDecryptFailed", err)
	}
}

func TestVaultFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	passphrase := "file-test-password"

	data := domain.NewVaultData()
	data.Connections = []domain.Connection{
		{
			ID: "c1", Name: "Server", Host: "10.0.0.1", Port: 22,
			Users:         []domain.ConnectionUser{{ID: "u1", Username: "admin", Auth: domain.AuthMethodKey}},
			DefaultUserID: "u1",
		},
	}

	writeVault(t, dir, passphrase, data)

	vaultPath := filepath.Join(dir, "vault.age")
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		t.Fatal("vault.age file not created")
	}

	tmpPath := filepath.Join(dir, "vault.age.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatal("temporary file should be cleaned up after atomic write")
	}

	read := openVault(t, dir, passphrase)
	if len(read.Connections) != 1 || read.Connections[0].Host != "10.0.0.1" {
		t.Errorf("read data mismatch: %+v", read.Connections)
	}
}

func TestVaultReadNonExistentReturnsErrVaultNotFound(t *testing.T) {
	dir := t.TempDir()

	_, data, _, err := vault.Open(dir, "any-password")
	if !errors.Is(err, domain.ErrVaultNotFound) {
		t.Fatalf("expected ErrVaultNotFound, got: %v", err)
	}
	if data != nil {
		t.Error("expected nil data when no vault file exists")
	}

	// Reading must never bring a vault into existence; only Create may.
	if _, statErr := os.Stat(filepath.Join(dir, "vault.age")); !os.IsNotExist(statErr) {
		t.Error("read must not create vault.age")
	}
}

func TestVaultExists(t *testing.T) {
	dir := t.TempDir()

	if vault.Exists(dir) {
		t.Error("expected Exists false on an empty directory")
	}

	writeVault(t, dir, "correct-horse-battery", domain.NewVaultData())

	if !vault.Exists(dir) {
		t.Error("expected Exists true after a vault is written")
	}
}

func TestVaultIdentitiesStorage(t *testing.T) {
	dir := t.TempDir()
	data := domain.NewVaultData()
	data.Identities["key1"] = domain.SSHIdentity{
		ID: "key1", Comment: "test key", KeyType: "ed25519", Encrypted: false,
	}
	data.KeyBlobs["key1"] = domain.IdentityBlob{
		PEMData: []byte("fake-pem-data"),
	}

	writeVault(t, dir, "identity-test", data)
	decrypted := openVault(t, dir, "identity-test")

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
