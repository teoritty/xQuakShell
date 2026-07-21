package usecase

import (
	"context"

	"ssh-client/internal/domain"
)

// fileMover performs the four filesystem primitives the plan executor needs,
// abstracting whether the target is remote (upload), local (download), or a
// local copy. One executor loop drives any mover (SRP: the loop owns sequencing
// and conflict outcomes; each mover owns only how bytes and directories move on
// its filesystem).
type fileMover interface {
	// ensureDir creates dir and any missing parents (a no-op if it exists).
	ensureDir(ctx context.Context, dir string) error
	// existingNames returns the set of entry names already in dir, used to pick a
	// free name for a Rename outcome. A dir that cannot be listed yields an empty
	// set.
	existingNames(ctx context.Context, dir string) map[string]bool
	// removeTarget deletes targetPath (a file or a whole directory tree), used to
	// overwrite a type-mismatched target (writing a file where a directory sits).
	removeTarget(ctx context.Context, targetPath string) error
	// moveFile transfers one file from source to target, reporting byte progress.
	moveFile(ctx context.Context, source, target string, progress domain.ProgressFunc) error
}

// uploadMover writes to the remote filesystem over SFTP.
type uploadMover struct{ fs domain.RemoteFS }

func (m *uploadMover) ensureDir(ctx context.Context, dir string) error { return m.fs.Mkdir(ctx, dir) }
func (m *uploadMover) existingNames(ctx context.Context, dir string) map[string]bool {
	return namesOf(listRemoteTargetDir(ctx, m.fs, dir))
}
func (m *uploadMover) removeTarget(ctx context.Context, targetPath string) error {
	return m.fs.RemoveAll(ctx, targetPath, nil)
}
func (m *uploadMover) moveFile(ctx context.Context, source, target string, progress domain.ProgressFunc) error {
	return m.fs.Upload(ctx, source, target, progress)
}

// downloadMover writes to the local filesystem, reading over SFTP.
type downloadMover struct {
	fs     domain.RemoteFS
	hostFS domain.HostFileSystem
}

func (m *downloadMover) ensureDir(_ context.Context, dir string) error { return m.hostFS.Mkdir(dir) }
func (m *downloadMover) existingNames(_ context.Context, dir string) map[string]bool {
	return namesOf(listLocalTargetDir(m.hostFS, dir))
}
func (m *downloadMover) removeTarget(_ context.Context, targetPath string) error {
	return m.hostFS.Remove(targetPath)
}
func (m *downloadMover) moveFile(ctx context.Context, source, target string, progress domain.ProgressFunc) error {
	return m.fs.Download(ctx, source, target, progress)
}

// localCopyMover copies within the local filesystem (OS Explorer drop). Local
// copy has no streaming progress, so byte progress is reported once on
// completion by the executor rather than by the mover.
type localCopyMover struct{ hostFS domain.HostFileSystem }

func (m *localCopyMover) ensureDir(_ context.Context, dir string) error { return m.hostFS.Mkdir(dir) }
func (m *localCopyMover) existingNames(_ context.Context, dir string) map[string]bool {
	return namesOf(listLocalTargetDir(m.hostFS, dir))
}
func (m *localCopyMover) removeTarget(_ context.Context, targetPath string) error {
	return m.hostFS.Remove(targetPath)
}
func (m *localCopyMover) moveFile(_ context.Context, source, target string, _ domain.ProgressFunc) error {
	return m.hostFS.CopyTo(source, target)
}

func namesOf(index map[string]domain.FileStat) map[string]bool {
	out := make(map[string]bool, len(index))
	for name := range index {
		out[name] = true
	}
	return out
}
