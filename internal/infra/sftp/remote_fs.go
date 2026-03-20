package sftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"

	"ssh-client/internal/domain"
)

const transferChunkSize = 32 * 1024

// RemoteFS implements domain.RemoteFS using an SFTP client.
type RemoteFS struct {
	client       *sftp.Client
	rateLimitKbps int // 0 = unlimited
}

// NewRemoteFS wraps an SFTP client to implement domain.RemoteFS.
func NewRemoteFS(client *sftp.Client) *RemoteFS {
	return &RemoteFS{client: client}
}

// NewRemoteFSWithRateLimit creates RemoteFS with optional transfer speed limit (Kbps, 0 = unlimited).
func NewRemoteFSWithRateLimit(client *sftp.Client, rateLimitKbps int) *RemoteFS {
	return &RemoteFS{client: client, rateLimitKbps: rateLimitKbps}
}

// GetWorkingDirectory returns the remote server's current working directory (typically user's home).
func (fs *RemoteFS) GetWorkingDirectory(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	wd, err := fs.client.Getwd()
	if err != nil {
		return "", fmt.Errorf("sftp getwd: %w", err)
	}
	return sanitizeRemotePath(wd), nil
}

// List returns the direct children of the given remote directory.
func (fs *RemoteFS) List(ctx context.Context, dirPath string) ([]domain.RemoteNode, error) {
	dirPath = sanitizeRemotePath(dirPath)

	entries, err := fs.client.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("sftp list %s: %w", dirPath, err)
	}

	nodes := make([]domain.RemoteNode, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		node := domain.RemoteNode{
			Path:    path.Join(dirPath, entry.Name()),
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    entry.Size(),
			ModTime: entry.ModTime(),
		}
		if sys := entry.Sys(); sys != nil {
			if fs, ok := sys.(*sftp.FileStat); ok {
				node.Mode = fs.FileMode().String()
				node.Owner = strconv.FormatUint(uint64(fs.UID), 10)
				node.Group = strconv.FormatUint(uint64(fs.GID), 10)
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// Upload copies a local file to the remote path, reporting progress.
func (fs *RemoteFS) Upload(ctx context.Context, localPath, remotePath string, progress domain.ProgressFunc) error {
	remotePath = sanitizeRemotePath(remotePath)

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("sftp upload open local %s: %w", localPath, err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("sftp upload stat %s: %w", localPath, err)
	}
	totalSize := stat.Size()

	remoteFile, err := fs.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp upload create remote %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	buf := make([]byte, transferChunkSize)
	var done int64
	src := io.Reader(localFile)
	if fs.rateLimitKbps > 0 {
		src = newThrottledReader(ctx, localFile, fs.rateLimitKbps)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := remoteFile.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("sftp upload write: %w", writeErr)
			}
			done += int64(n)
			if progress != nil {
				progress(done, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("sftp upload read: %w", readErr)
		}
	}

	return nil
}

// Download copies a remote file to the local path, reporting progress.
func (fs *RemoteFS) Download(ctx context.Context, remotePath, localPath string, progress domain.ProgressFunc) error {
	remotePath = sanitizeRemotePath(remotePath)

	remoteFile, err := fs.client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("sftp download open remote %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	stat, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("sftp download stat %s: %w", remotePath, err)
	}
	totalSize := stat.Size()

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("sftp download create local %s: %w", localPath, err)
	}
	defer localFile.Close()

	buf := make([]byte, transferChunkSize)
	var done int64
	src := io.Reader(remoteFile)
	if fs.rateLimitKbps > 0 {
		src = newThrottledReader(ctx, remoteFile, fs.rateLimitKbps)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := localFile.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("sftp download write: %w", writeErr)
			}
			done += int64(n)
			if progress != nil {
				progress(done, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("sftp download read: %w", readErr)
		}
	}

	return nil
}

// computeLocalDirSize returns the total size of all files in the directory (recursive).
func computeLocalDirSize(localDir string) (int64, error) {
	var total int64
	err := filepath.Walk(localDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// UploadRecursive recursively uploads a local directory to the remote path.
func (fs *RemoteFS) UploadRecursive(ctx context.Context, localDir, remoteDir string, progress domain.ProgressFunc) error {
	remoteDir = sanitizeRemotePath(remoteDir)
	if err := fs.client.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("sftp upload recursive mkdir %s: %w", remoteDir, err)
	}
	totalSize, _ := computeLocalDirSize(localDir)
	if totalSize <= 0 {
		totalSize = -1
	}
	var totalDone int64
	return filepath.Walk(localDir, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		remotePath := path.Join(remoteDir, filepath.ToSlash(rel))
		if info.IsDir() {
			return fs.client.MkdirAll(remotePath)
		}
		if err := fs.Upload(ctx, localPath, remotePath, func(done, total int64) {
			if progress != nil {
				progress(totalDone+done, totalSize)
			}
		}); err != nil {
			return err
		}
		totalDone += info.Size()
		return nil
	})
}

// computeRemoteDirSize returns the total size of all files in the remote directory (recursive).
func (fs *RemoteFS) computeRemoteDirSize(ctx context.Context, remoteDir string) (int64, error) {
	remoteDir = sanitizeRemotePath(remoteDir)
	entries, err := fs.client.ReadDir(remoteDir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		remotePath := path.Join(remoteDir, entry.Name())
		if entry.IsDir() {
			subTotal, err := fs.computeRemoteDirSize(ctx, remotePath)
			if err != nil {
				return 0, err
			}
			total += subTotal
		} else {
			total += entry.Size()
		}
	}
	return total, nil
}

// DownloadRecursive recursively downloads a remote directory to the local path.
func (fs *RemoteFS) DownloadRecursive(ctx context.Context, remoteDir, localDir string, progress domain.ProgressFunc) error {
	totalSize, _ := fs.computeRemoteDirSize(ctx, remoteDir)
	if totalSize <= 0 {
		totalSize = -1
	}
	var totalDone int64
	return fs.downloadRecursive(ctx, remoteDir, localDir, &totalDone, totalSize, progress)
}

func (fs *RemoteFS) downloadRecursive(ctx context.Context, remoteDir, localDir string, totalDone *int64, totalSize int64, progress domain.ProgressFunc) error {
	remoteDir = sanitizeRemotePath(remoteDir)
	entries, err := fs.client.ReadDir(remoteDir)
	if err != nil {
		return fmt.Errorf("sftp download recursive readdir %s: %w", remoteDir, err)
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		remotePath := path.Join(remoteDir, entry.Name())
		localPath := filepath.Join(localDir, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", localPath, err)
			}
			if err := fs.downloadRecursive(ctx, remotePath, localPath, totalDone, totalSize, progress); err != nil {
				return err
			}
		} else {
			size := entry.Size()
			if err := fs.Download(ctx, remotePath, localPath, func(done, total int64) {
				if progress != nil {
					progress(*totalDone+done, totalSize)
				}
			}); err != nil {
				return err
			}
			*totalDone += size
		}
	}
	return nil
}

// Mkdir creates a remote directory (and parents if needed).
func (fs *RemoteFS) Mkdir(ctx context.Context, dirPath string) error {
	dirPath = sanitizeRemotePath(dirPath)
	return fs.client.MkdirAll(dirPath)
}

// CreateFile creates an empty remote file.
func (fs *RemoteFS) CreateFile(ctx context.Context, remotePath string) error {
	remotePath = sanitizeRemotePath(remotePath)
	f, err := fs.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp create file %s: %w", remotePath, err)
	}
	return f.Close()
}

// Remove deletes a remote file or empty directory.
func (fs *RemoteFS) Remove(ctx context.Context, remotePath string) error {
	remotePath = sanitizeRemotePath(remotePath)
	return fs.client.Remove(remotePath)
}

// RemoveAll recursively deletes a remote path (file or directory with contents).
func (fs *RemoteFS) RemoveAll(ctx context.Context, remotePath string) error {
	remotePath = sanitizeRemotePath(remotePath)
	stat, err := fs.client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("sftp removeall stat %s: %w", remotePath, err)
	}
	if !stat.IsDir() {
		return fs.client.Remove(remotePath)
	}
	entries, err := fs.client.ReadDir(remotePath)
	if err != nil {
		return fmt.Errorf("sftp removeall readdir %s: %w", remotePath, err)
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		childPath := path.Join(remotePath, entry.Name())
		if entry.IsDir() {
			if err := fs.RemoveAll(ctx, childPath); err != nil {
				return err
			}
		} else {
			if err := fs.client.Remove(childPath); err != nil {
				return fmt.Errorf("sftp removeall file %s: %w", childPath, err)
			}
		}
	}
	return fs.client.Remove(remotePath)
}

// Rename moves/renames a remote path.
func (fs *RemoteFS) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = sanitizeRemotePath(oldPath)
	newPath = sanitizeRemotePath(newPath)
	return fs.client.Rename(oldPath, newPath)
}

// Close releases the underlying SFTP connection.
func (fs *RemoteFS) Close() error {
	return fs.client.Close()
}

// sanitizeRemotePath normalizes a remote path to prevent basic traversal attacks.
func sanitizeRemotePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "" || p == "." {
		p = "/"
	}
	return p
}
