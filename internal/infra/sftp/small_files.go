package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"xquakshell/internal/domain"
)

// ReadSmallFile returns a whole remote file, refusing anything past maxBytes.
//
// It reads maxBytes+1 and treats the overflow byte as the refusal signal, rather than trusting
// the size from a Stat: an SFTP server is free to report one size and serve another, and the
// point of the cap is to survive a server that does exactly that.
func (fs *RemoteFS) ReadSmallFile(_ context.Context, remotePath string, maxBytes int64) ([]byte, error) {
	remotePath = sanitizeRemotePath(remotePath)
	f, err := fs.client.Open(remotePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("sftp read %s: %w", remotePath, domain.ErrRemoteFileNotFound)
		}
		return nil, fmt.Errorf("sftp open %s: %w", remotePath, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close failed", "resource", remotePath, "err", err)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("sftp read %s: %w", remotePath, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("sftp read %s: %w", remotePath, domain.ErrRemoteFileTooLarge)
	}
	return data, nil
}

// WriteSmallFile replaces a remote file's contents.
//
// The file is opened truncating and its mode is set afterwards rather than relying on the open
// flags, because an SFTP server applies the remote user's umask to a newly created file and would
// otherwise hand back something more permissive than asked for - which for authorized_keys means
// sshd refuses it.
func (fs *RemoteFS) WriteSmallFile(_ context.Context, remotePath string, data []byte, mode os.FileMode) error {
	remotePath = sanitizeRemotePath(remotePath)
	f, err := fs.client.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("sftp open %s for write: %w", remotePath, err)
	}
	if _, err := f.Write(data); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			slog.Warn("close failed", "resource", remotePath, "err", closeErr)
		}
		return fmt.Errorf("sftp write %s: %w", remotePath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sftp close %s: %w", remotePath, err)
	}
	if err := fs.client.Chmod(remotePath, mode); err != nil {
		return fmt.Errorf("sftp chmod %s after write: %w", remotePath, err)
	}
	return nil
}
