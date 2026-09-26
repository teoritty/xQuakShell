package sftp

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/pkg/sftp"

	"xquakshell/internal/domain"
)

// newInMemoryRemoteFS serves a real SFTP conversation over a pipe, backed by pkg/sftp's in-memory
// handler. It is the SSH_FX_NO_SUCH_FILE status on the wire that has to become
// ErrDirectoryNotFound, so a fake that returned os.ErrNotExist directly would test nothing.
func newInMemoryRemoteFS(t *testing.T) (*RemoteFS, *sftp.Client) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	server := sftp.NewRequestServer(serverConn, sftp.InMemHandler())
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Close() })

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatalf("sftp client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return NewRemoteFSWithRateLimit(client, 0), client
}

func TestListOfAMissingRemoteDirectoryIsDirectoryNotFound(t *testing.T) {
	fs, _ := newInMemoryRemoteFS(t)

	_, err := fs.List(context.Background(), "/gone")
	if !errors.Is(err, domain.ErrDirectoryNotFound) {
		t.Fatalf("List(/gone) = %v, want ErrDirectoryNotFound so the pane steps up instead of erroring", err)
	}
}

func TestListOfAnExistingRemoteDirectoryStillLists(t *testing.T) {
	fs, client := newInMemoryRemoteFS(t)
	if err := client.Mkdir("/here"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := client.Create("/here/a.txt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _ = io.WriteString(f, "x")
	_ = f.Close()

	nodes, err := fs.List(context.Background(), "/here")
	if err != nil {
		t.Fatalf("List(/here) = %v, want nil", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "a.txt" {
		t.Fatalf("nodes = %+v, want the one file", nodes)
	}
}
