package sftp

import (
	"fmt"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

// NewSFTPClient creates a new SFTP client from an active SSH client connection.
func NewSFTPClient(sshClient *gossh.Client) (*sftp.Client, error) {
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("sftp new client: %w", err)
	}
	return client, nil
}

// sftpClientFactory implements domain.SFTPClientFactory.
type sftpClientFactory struct{}

// NewSFTPClientFactory returns a domain.SFTPClientFactory backed by infra RemoteFS.
func NewSFTPClientFactory() domain.SFTPClientFactory {
	return sftpClientFactory{}
}

func (sftpClientFactory) New(client domain.SSHClient, rateLimitKbps int) (domain.RemoteFS, error) {
	raw, err := NewSFTPClient(client.Client())
	if err != nil {
		return nil, err
	}
	return NewRemoteFSWithRateLimit(raw, rateLimitKbps), nil
}
