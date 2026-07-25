package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/pathsafe"
)

// sanitizeLocalPath normalizes a local path to prevent basic traversal attacks.
func sanitizeLocalPath(p string) string {
	return filepath.Clean(p)
}

// RemoteFS implements domain.RemoteFS using an SFTP client.
type RemoteFS struct {
	client       *sftp.Client
	rateLimitKbps int // 0 = unlimited

	// readDirFn is a test seam for downloadRecursive: it defaults to
	// client.ReadDir but lets tests drive downloadRecursive with a fake
	// directory listing (including hostile names) without a live SFTP
	// server. Production code never sets this field directly; it is
	// resolved lazily via readDir() below.
	readDirFn func(dir string) ([]os.FileInfo, error)
}

// readDir returns the configured readDirFn, or fs.client.ReadDir if unset.
func (fs *RemoteFS) readDir(dir string) ([]os.FileInfo, error) {
	if fs.readDirFn != nil {
		return fs.readDirFn(dir)
	}
	return fs.client.ReadDir(dir)
}

// safeEntryName validates a filename taken from a remote directory listing.
// The SFTP server controls these bytes completely, so a name is accepted only
// if it is a single, inert path segment. Rejecting here — at the adapter
// boundary — is what lets every consumer of RemoteFS treat RemoteNode.Name as
// a trusted component. pkg/sftp already applies path.Base, but path.Base only
// understands forward slashes: a name like `..\..\evil.exe` reaches us intact
// and escapes once filepath.Join cleans it on Windows.
func safeEntryName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	// Windows resolves ADS and drive-relative syntax inside a single segment.
	if strings.ContainsRune(name, ':') {
		return false
	}
	if strings.ContainsRune(name, 0) {
		return false
	}
	return true
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

		name := entry.Name()
		if !safeEntryName(name) {
			slog.Warn("sftp: skipping unsafe remote entry name",
				"dir", dirPath, "name", name)
			continue
		}

		node := domain.RemoteNode{
			Path:    path.Join(dirPath, name),
			Name:    name,
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
	localPath = sanitizeLocalPath(localPath)

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

	// Feed the upload through a progress-reporting, cancellable reader. The
	// throttle (if any) sits underneath so the reported progress reflects the
	// throttled delivery rate. progressReader exposes Size(), which is what
	// lets ReadFrom pipeline concurrent writes.
	var src io.Reader = localFile
	if fs.rateLimitKbps > 0 {
		src = newThrottledReader(ctx, localFile, fs.rateLimitKbps)
	}
	metered := &progressReader{r: src, ctx: ctx, total: totalSize, progress: progress}

	if _, err := remoteFile.ReadFrom(metered); err != nil {
		// Concurrent writes can leave a file longer than the last contiguously
		// written byte on error (holes), so the partial upload is never a
		// valid file — remove it rather than leave corruption behind. Use a
		// background context so cleanup still runs when ctx is the cause.
		_ = remoteFile.Close()
		_ = fs.client.Remove(remotePath)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("sftp upload %s: %w", remotePath, err)
	}
	return nil
}

// Download copies a remote file to the local path, reporting progress.
func (fs *RemoteFS) Download(ctx context.Context, remotePath, localPath string, progress domain.ProgressFunc) error {
	remotePath = sanitizeRemotePath(remotePath)
	localPath = sanitizeLocalPath(localPath)

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

	// WriteTo owns the (concurrent) read side and writes to our writer
	// sequentially, in offset order, so the destination is the stream we
	// wrap: progress + cancellation, with the throttle underneath.
	var dst io.Writer = localFile
	if fs.rateLimitKbps > 0 {
		dst = newThrottledWriter(ctx, localFile, fs.rateLimitKbps)
	}
	metered := &progressWriter{w: dst, ctx: ctx, total: totalSize, progress: progress}

	if _, err := remoteFile.WriteTo(metered); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("sftp download %s: %w", remotePath, err)
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
	totalSize, err := computeLocalDirSize(localDir)
	if err != nil {
		slog.Warn("sftp upload recursive: compute dir size failed", "dir", localDir, "err", err)
		totalSize = -1
	}
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
	totalSize, err := fs.computeRemoteDirSize(ctx, remoteDir)
	if err != nil {
		slog.Warn("sftp download recursive: compute dir size failed", "dir", remoteDir, "err", err)
		totalSize = -1
	}
	if totalSize <= 0 {
		totalSize = -1
	}
	var totalDone int64
	return fs.downloadRecursive(ctx, remoteDir, localDir, &totalDone, totalSize, progress)
}

func (fs *RemoteFS) downloadRecursive(ctx context.Context, remoteDir, localDir string, totalDone *int64, totalSize int64, progress domain.ProgressFunc) error {
	remoteDir = sanitizeRemotePath(remoteDir)
	absLocalRoot, err := filepath.Abs(localDir)
	if err != nil {
		return fmt.Errorf("resolve local dir %s: %w", localDir, err)
	}
	entries, err := fs.readDir(remoteDir)
	if err != nil {
		return fmt.Errorf("sftp download recursive readdir %s: %w", remoteDir, err)
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		name := entry.Name()
		if !safeEntryName(name) {
			slog.Warn("sftp: skipping unsafe remote entry name",
				"dir", remoteDir, "name", name)
			continue
		}
		remotePath := path.Join(remoteDir, name)
		localPath := filepath.Join(localDir, name)
		if !pathsafe.UnderRoot(absLocalRoot, filepath.Join(absLocalRoot, name)) {
			slog.Warn("sftp: remote entry escapes download root",
				"dir", remoteDir, "name", name, "target", localPath)
			continue
		}
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
// onEach, if non-nil, is called once per removed entry for progress reporting.
func (fs *RemoteFS) RemoveAll(ctx context.Context, remotePath string, onEach func()) error {
	remotePath = sanitizeRemotePath(remotePath)
	stat, err := fs.client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("sftp removeall stat %s: %w", remotePath, err)
	}
	if !stat.IsDir() {
		if err := fs.client.Remove(remotePath); err != nil {
			return err
		}
		tick(onEach)
		return nil
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
			if err := fs.RemoveAll(ctx, childPath, onEach); err != nil {
				return err
			}
		} else {
			if err := fs.client.Remove(childPath); err != nil {
				return fmt.Errorf("sftp removeall file %s: %w", childPath, err)
			}
			tick(onEach)
		}
	}
	if err := fs.client.Remove(remotePath); err != nil {
		return err
	}
	tick(onEach)
	return nil
}

// CountTree counts the entries a recursive operation with the given applyTo
// filter would act on: it walks like walkApply (skipping symlinked directories'
// contents to avoid loops) and counts each node the operation's filter matches.
// Read-only; used to pre-compute a progress total.
func (fs *RemoteFS) CountTree(ctx context.Context, rootPath string, applyTo domain.ApplyTarget, onEach func()) (int64, error) {
	var n int64
	err := fs.walkApply(ctx, rootPath, true, func(_ string, isDir bool) error {
		if applyTo.Matches(isDir) {
			n++
			tick(onEach)
		}
		return nil
	})
	return n, err
}

// tick invokes fn if it is non-nil.
func tick(fn func()) {
	if fn != nil {
		fn()
	}
}

// Rename moves/renames a remote path.
func (fs *RemoteFS) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = sanitizeRemotePath(oldPath)
	newPath = sanitizeRemotePath(newPath)
	return fs.client.Rename(oldPath, newPath)
}

// Chmod sets permission bits on a remote path.
//
// Note: SFTP has no lchmod equivalent — if remotePath is a symlink, this
// changes the permissions of its target, not the link itself.
func (fs *RemoteFS) Chmod(ctx context.Context, remotePath string, mode os.FileMode) error {
	remotePath = sanitizeRemotePath(remotePath)
	if err := fs.client.Chmod(remotePath, mode); err != nil {
		return fmt.Errorf("sftp chmod %s: %w", remotePath, err)
	}
	return nil
}

// Chown sets the owner uid/gid on a remote path.
//
// Note: SFTP has no lchown equivalent — if remotePath is a symlink, this
// changes the owner of its target, not the link itself.
func (fs *RemoteFS) Chown(ctx context.Context, remotePath string, uid, gid int) error {
	remotePath = sanitizeRemotePath(remotePath)
	if err := fs.client.Chown(remotePath, uid, gid); err != nil {
		return fmt.Errorf("sftp chown %s: %w", remotePath, err)
	}
	return nil
}

// ChmodRecursive applies mode to rootPath and, if it's a directory, to its
// descendants filtered by applyTo. The root itself is always changed
// regardless of applyTo (the filter only governs descendants). Continues
// past per-item errors (best-effort) and returns an aggregate error
// listing every path that failed, so one locked file doesn't block
// changing permissions on the rest of a large tree.
func (fs *RemoteFS) ChmodRecursive(ctx context.Context, rootPath string, mode os.FileMode, applyTo domain.ApplyTarget, onEach func()) error {
	return fs.walkApply(ctx, rootPath, true, func(p string, isDir bool) error {
		if !applyTo.Matches(isDir) {
			return nil
		}
		if err := fs.Chmod(ctx, p, mode); err != nil {
			return err
		}
		tick(onEach)
		return nil
	})
}

// ChownRecursive applies uid/gid to rootPath and, if it's a directory, to its
// descendants filtered by applyTo. Same root/best-effort semantics as ChmodRecursive.
func (fs *RemoteFS) ChownRecursive(ctx context.Context, rootPath string, uid, gid int, applyTo domain.ApplyTarget, onEach func()) error {
	return fs.walkApply(ctx, rootPath, true, func(p string, isDir bool) error {
		if !applyTo.Matches(isDir) {
			return nil
		}
		if err := fs.Chown(ctx, p, uid, gid); err != nil {
			return err
		}
		tick(onEach)
		return nil
	})
}

// walkApply applies fn to rootPath (always) and, if rootPath is a directory,
// recursively to every descendant, skipping symlinked directories to avoid
// loops. It collects per-item errors into a joined error rather than
// aborting on the first failure.
func (fs *RemoteFS) walkApply(ctx context.Context, rootPath string, isRoot bool, fn func(p string, isDir bool) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	rootPath = sanitizeRemotePath(rootPath)
	stat, err := fs.client.Lstat(rootPath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", rootPath, err)
	}
	var errs []error
	isDir := stat.IsDir()
	isSymlink := stat.Mode()&os.ModeSymlink != 0
	if isRoot || !isSymlink {
		if err := fn(rootPath, isDir); err != nil {
			errs = append(errs, err)
		}
	}
	if isDir && (isRoot || !isSymlink) {
		entries, err := fs.client.ReadDir(rootPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("sftp readdir %s: %w", rootPath, err))
		} else {
			for _, entry := range entries {
				childPath := path.Join(rootPath, entry.Name())
				if err := fs.walkApply(ctx, childPath, false, fn); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
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
