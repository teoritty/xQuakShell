package sftp

import (
	"fmt"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

// maxConcurrentRequestsPerFile bounds how many SFTP read/write packets are in
// flight for a single file transfer. Pipelining requests this way hides the
// per-packet round-trip latency that otherwise caps throughput at
// (packet size / RTT) — the reason a synchronous transfer stalls far below
// link speed on anything but a zero-latency link.
const maxConcurrentRequestsPerFile = 64

// NewSFTPClient creates a new SFTP client from an active SSH client connection.
//
// Concurrent reads and writes are enabled so large transfers pipeline many
// packets instead of waiting for each ACK. The packet size is left at the
// protocol-standard 32 KiB (sftp's default) that every server supports; the
// speed-up comes from concurrency, not larger packets.
func NewSFTPClient(sshClient *gossh.Client) (*sftp.Client, error) {
	client, err := sftp.NewClient(
		sshClient,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
		sftp.MaxConcurrentRequestsPerFile(maxConcurrentRequestsPerFile),
	)
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
