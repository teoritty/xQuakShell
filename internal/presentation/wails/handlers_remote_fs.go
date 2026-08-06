package wails

import (
	"fmt"
	"os"

	"xquakshell/internal/domain"
)

// --- SFTP remote operations ---

// ListPath lists the contents of a remote directory.
func (a *AppAPI) ListPath(sessionID, dirPath string) ([]RemoteNodeDTO, error) {
	if a.remoteFS == nil {
		return nil, fmt.Errorf("remote file service unavailable")
	}
	nodes, err := a.remoteFS.ListPath(sessionID, dirPath)
	if err != nil {
		return nil, err
	}
	return RemoteNodesToDTO(nodes), nil
}

// RemovePath deletes a remote file or directory (recursively for directories).
// It runs in the background and streams progress through the transfer/operation
// event; the call returns as soon as the job is scheduled.
func (a *AppAPI) RemovePath(sessionID, remotePath string) error {
	if a.remoteOpSvc == nil {
		return fmt.Errorf("remote operation service unavailable")
	}
	return a.remoteOpSvc.Delete(sessionID, remotePath, a.emitTransferProgress)
}

// MkdirPath creates a remote directory (and parents if needed).
func (a *AppAPI) MkdirPath(sessionID, parentPath, name string) error {
	if a.remoteFS == nil {
		return fmt.Errorf("remote file service unavailable")
	}
	return a.remoteFS.MkdirPath(sessionID, parentPath, name)
}

// CreateFilePath creates an empty remote file.
func (a *AppAPI) CreateFilePath(sessionID, parentPath, name string) error {
	if a.remoteFS == nil {
		return fmt.Errorf("remote file service unavailable")
	}
	return a.remoteFS.CreateFilePath(sessionID, parentPath, name)
}

// RenamePath renames a remote file or directory.
func (a *AppAPI) RenamePath(sessionID, oldPath, newPath string) error {
	if a.remoteFS == nil {
		return fmt.Errorf("remote file service unavailable")
	}
	return a.remoteFS.RenamePath(sessionID, oldPath, newPath)
}

// applyTargetFromString maps the "files"|"dirs"|"both" wire value to domain.ApplyTarget.
func applyTargetFromString(applyTo string) domain.ApplyTarget {
	switch applyTo {
	case "files":
		return domain.ApplyFilesOnly
	case "dirs":
		return domain.ApplyDirsOnly
	default:
		return domain.ApplyBoth
	}
}

// Chmod sets permission bits on a remote path.
func (a *AppAPI) Chmod(sessionID, remotePath string, mode uint32) error {
	if a.remoteFS == nil {
		return fmt.Errorf("remote file service unavailable")
	}
	return a.remoteFS.ChmodPath(sessionID, remotePath, os.FileMode(mode))
}

// Chown sets the owner uid/gid on a remote path.
func (a *AppAPI) Chown(sessionID, remotePath string, uid, gid int) error {
	if a.remoteFS == nil {
		return fmt.Errorf("remote file service unavailable")
	}
	return a.remoteFS.ChownPath(sessionID, remotePath, uid, gid)
}

// ChmodRecursive applies mode recursively under remotePath, filtered by
// applyTo ("files", "dirs", or "both"). Runs in the background with progress.
func (a *AppAPI) ChmodRecursive(sessionID, remotePath string, mode uint32, applyTo string) error {
	if a.remoteOpSvc == nil {
		return fmt.Errorf("remote operation service unavailable")
	}
	return a.remoteOpSvc.ChmodRecursive(sessionID, remotePath, os.FileMode(mode), applyTargetFromString(applyTo), a.emitTransferProgress)
}

// ChownRecursive applies uid/gid recursively under remotePath, filtered by
// applyTo ("files", "dirs", or "both"). Runs in the background with progress.
func (a *AppAPI) ChownRecursive(sessionID, remotePath string, uid, gid int, applyTo string) error {
	if a.remoteOpSvc == nil {
		return fmt.Errorf("remote operation service unavailable")
	}
	return a.remoteOpSvc.ChownRecursive(sessionID, remotePath, uid, gid, applyTargetFromString(applyTo), a.emitTransferProgress)
}

// --- Known Hosts ---

// GetKnownHosts feeds the known-hosts manager. A missing host key service is
// an error rather than an empty list: a wiring mistake must not reach the user
// looking like "you have trusted nothing", which invites re-trusting a host
// that was already pinned.
func (a *AppAPI) GetKnownHosts() ([]KnownHostDTO, error) {
	if a.hostKeys == nil {
		return nil, fmt.Errorf("host key service unavailable")
	}
	entries, err := a.hostKeys.List()
	if err != nil {
		return nil, err
	}
	return KnownHostsToDTO(entries), nil
}

// AddKnownHost adds a known host entry from an authorized_key formatted string.
func (a *AppAPI) AddKnownHost(host, authorizedKey string) error {
	if a.hostKeys == nil {
		return fmt.Errorf("host key service unavailable")
	}
	return a.hostKeys.Add(a.reqCtx(), host, authorizedKey)
}

// RemoveKnownHost removes a known host entry by host pattern.
func (a *AppAPI) RemoveKnownHost(host string) error {
	if a.hostKeys == nil {
		return fmt.Errorf("host key service unavailable")
	}
	return a.hostKeys.Remove(a.reqCtx(), host)
}
