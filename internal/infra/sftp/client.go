package sftp

import (
	"fmt"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

// NewSFTPClient creates a new SFTP client from an active SSH client connection.
func NewSFTPClient(sshClient *gossh.Client) (*sftp.Client, error) {
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("sftp new client: %w", err)
	}
	return client, nil
}
