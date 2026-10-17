package usecase

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

type bindingVault struct {
	sshTestVaultRepo
	d *domain.VaultData
}

func (b bindingVault) GetData() (*domain.VaultData, error) { return b.d, nil }

func boundConnection() (*domain.Connection, *domain.VaultData) {
	d := domain.NewVaultData()
	d.Passwords["mine"] = domain.PasswordBlob{Label: "mine"}
	d.Sync = &domain.VaultSync{Bindings: []domain.CredentialBinding{{VaultID: "t", ConnectionID: "c1", UserID: "u1", PasswordID: "mine"}}}
	conn := &domain.Connection{ID: "c1", Host: "db.example", Port: 22, VaultID: "t", DefaultUserID: "u1",
		Users: []domain.ConnectionUser{{ID: "u1", Username: "deploy", Auth: domain.AuthMethodPassword, CredentialSource: domain.CredentialSourcePersonal}}}
	return conn, d
}

func TestConnectorAuthenticatesWithTheBoundCredential(t *testing.T) {
	conn, d := boundConnection()
	c := NewSSHConnector(SSHConnectorConfig{VaultRepo: bindingVault{d: d}, PasswordRepo: sshTestPasswordRepo{password: "own secret"}})
	_, password, _, _, err := c.resolveAuth(context.Background(), conn)
	if err != nil || password != "own secret" {
		t.Fatalf("password = %q, %v; a personal source authenticates with the member's bound password", password, err)
	}
}

func TestConnectorRefusesASuspendedBinding(t *testing.T) {
	conn, d := boundConnection()
	d.Sync.Bindings[0].SuspendedBy = "carol"
	c := NewSSHConnector(SSHConnectorConfig{VaultRepo: bindingVault{d: d}, PasswordRepo: sshTestPasswordRepo{password: "own secret"}})
	if _, password, _, _, err := c.resolveAuth(context.Background(), conn); !errors.Is(err, domain.ErrBindingSuspended) || password != "" {
		t.Fatalf("password %q, err %v; a repointed host must get nothing", password, err)
	}
}
