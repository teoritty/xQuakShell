package sftp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"xquakshell/internal/domain"
)

// walkApply visits rootPath and everything beneath it, calling fn for each node.
//
// Symlinks below the root are neither acted on nor descended into: Lstat reports the link itself,
// so a recursive chmod cannot be steered onto the server's own files by a link the server planted.
// The root is the exception, because a user who names a symlink means that symlink.
func (fs *RemoteFS) walkApply(ctx context.Context, rootPath string, isRoot bool, fn func(p string, isDir bool) error) error {
	return fs.walkApplyDepth(ctx, rootPath, isRoot, 0, fn)
}

// walkApplyDepth is walkApply carrying the recursion depth it has reached.
//
// The depth is the whole reason it exists. The tree being walked is the server's to invent, and one
// that is infinitely deep - /a/a/a/a/... - costs a hostile or compromised server nothing to serve;
// any custom SFTP implementation or FUSE mount can generate one on demand. An unbounded walk
// recurses on it until the stack gives out.
//
// Exceeding the bound is reported rather than quietly truncated. A recursive chmod that stopped
// early and said nothing looks exactly like one that finished, and the user would have no way to
// learn that most of the tree still carries its old mode.
func (fs *RemoteFS) walkApplyDepth(ctx context.Context, rootPath string, isRoot bool, depth int, fn func(p string, isDir bool) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if depth > domain.MaxRemoteWalkDepth {
		return fmt.Errorf("%w: %s at depth %d", domain.ErrRemoteWalkTooDeep, rootPath, depth)
	}

	rootPath = sanitizeRemotePath(rootPath)
	stat, err := fs.client.Lstat(rootPath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", rootPath, err)
	}

	isDir := stat.IsDir()
	visit := isRoot || stat.Mode()&os.ModeSymlink == 0

	var errs []error
	if visit {
		if err := fn(rootPath, isDir); err != nil {
			errs = append(errs, err)
		}
	}
	if isDir && visit {
		errs = append(errs, fs.walkChildren(ctx, rootPath, depth, fn)...)
	}
	return errors.Join(errs...)
}

// walkChildren descends one level, collecting every error rather than stopping at the first.
//
// It is a separate function so walkApplyDepth stays inside the nesting budget, and the split is
// where it belongs anyway: one function decides whether a node is walked, the other walks what is
// under it.
func (fs *RemoteFS) walkChildren(ctx context.Context, dir string, depth int, fn func(p string, isDir bool) error) []error {
	entries, err := fs.client.ReadDir(dir)
	if err != nil {
		return []error{fmt.Errorf("sftp readdir %s: %w", dir, err)}
	}
	var errs []error
	for _, entry := range entries {
		childPath := path.Join(dir, entry.Name())
		if err := fs.walkApplyDepth(ctx, childPath, false, depth+1, fn); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
