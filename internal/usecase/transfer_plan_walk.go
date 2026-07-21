package usecase

import (
	"context"
	"path"
	"time"

	"ssh-client/internal/domain"
)

// localModTimeLayout matches domain.LocalFileEntry.ModTime formatting in HostFS.
const localModTimeLayout = "2006-01-02 15:04:05"

// walkLocalSource enumerates a dropped local root (file or directory) into
// sourceEntry nodes rooted at the root's base name. Symlinked directories are
// not descended into (HostFileSystem.List reports them as non-dir entries), so
// they are transferred as single entries rather than followed — avoiding loops.
func walkLocalSource(hostFS domain.HostFileSystem, root string) ([]sourceEntry, error) {
	info, err := hostFS.Stat(root)
	if err != nil {
		return nil, err
	}
	base := path.Base(toSlash(root))
	if !info.IsDir {
		return []sourceEntry{{AbsPath: root, Rel: base, Size: info.Size, ModTime: info.ModTime}}, nil
	}
	out := []sourceEntry{{AbsPath: root, Rel: base, IsDir: true}}
	var recur func(dir, rel string) error
	recur = func(dir, rel string) error {
		entries, err := hostFS.List(dir, true, nil)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childRel := rel + "/" + e.Name
			if e.IsDir {
				out = append(out, sourceEntry{AbsPath: e.Path, Rel: childRel, IsDir: true})
				if err := recur(e.Path, childRel); err != nil {
					return err
				}
				continue
			}
			out = append(out, sourceEntry{
				AbsPath: e.Path, Rel: childRel,
				Size: e.Size, ModTime: parseLocalModTime(e.ModTime),
			})
		}
		return nil
	}
	if err := recur(root, base); err != nil {
		return nil, err
	}
	return out, nil
}

// walkRemoteSource enumerates a dropped remote root into sourceEntry nodes. It
// discovers the root's own type by listing its parent (RemoteFS has no single
// Stat), then recurses via List for directories.
func walkRemoteSource(ctx context.Context, fs domain.RemoteFS, root string) ([]sourceEntry, error) {
	parent := path.Dir(root)
	base := path.Base(root)
	siblings, err := fs.List(ctx, parent)
	if err != nil {
		return nil, err
	}
	var node *domain.RemoteNode
	for i := range siblings {
		if siblings[i].Name == base {
			node = &siblings[i]
			break
		}
	}
	if node == nil {
		return nil, &notFoundError{path: root}
	}
	if !node.IsDir {
		return []sourceEntry{{AbsPath: root, Rel: base, Size: node.Size, ModTime: node.ModTime}}, nil
	}
	out := []sourceEntry{{AbsPath: root, Rel: base, IsDir: true}}
	var recur func(dir, rel string) error
	recur = func(dir, rel string) error {
		entries, err := fs.List(ctx, dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childRel := rel + "/" + e.Name
			if e.IsDir {
				out = append(out, sourceEntry{AbsPath: e.Path, Rel: childRel, IsDir: true})
				if err := recur(e.Path, childRel); err != nil {
					return err
				}
				continue
			}
			out = append(out, sourceEntry{AbsPath: e.Path, Rel: childRel, Size: e.Size, ModTime: e.ModTime})
		}
		return nil
	}
	if err := recur(root, base); err != nil {
		return nil, err
	}
	return out, nil
}

// listRemoteTargetDir indexes a remote directory by entry name. A directory that
// cannot be listed (e.g. it does not exist yet) yields an empty index, meaning
// "no conflicts here".
func listRemoteTargetDir(ctx context.Context, fs domain.RemoteFS, dir string) map[string]domain.FileStat {
	entries, err := fs.List(ctx, dir)
	if err != nil {
		return map[string]domain.FileStat{}
	}
	index := make(map[string]domain.FileStat, len(entries))
	for _, e := range entries {
		index[e.Name] = domain.FileStat{Exists: true, IsDir: e.IsDir, Size: e.Size, ModTime: e.ModTime}
	}
	return index
}

// listLocalTargetDir indexes a local directory by entry name.
func listLocalTargetDir(hostFS domain.HostFileSystem, dir string) map[string]domain.FileStat {
	entries, err := hostFS.List(dir, true, nil)
	if err != nil {
		return map[string]domain.FileStat{}
	}
	index := make(map[string]domain.FileStat, len(entries))
	for _, e := range entries {
		index[e.Name] = domain.FileStat{Exists: true, IsDir: e.IsDir, Size: e.Size, ModTime: parseLocalModTime(e.ModTime)}
	}
	return index
}

func parseLocalModTime(s string) time.Time {
	if t, err := time.ParseInLocation(localModTimeLayout, s, time.Local); err == nil {
		return t
	}
	return time.Time{}
}

func toSlash(p string) string {
	out := make([]rune, 0, len(p))
	for _, r := range p {
		if r == '\\' {
			r = '/'
		}
		out = append(out, r)
	}
	return string(out)
}

type notFoundError struct{ path string }

func (e *notFoundError) Error() string { return "remote path not found: " + e.path }
