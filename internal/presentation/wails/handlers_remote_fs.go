package wails

import (
	"context"
	"fmt"
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
func (a *AppAPI) RemovePath(sessionID, remotePath string) error {
	if a.remoteFS == nil {
		return fmt.Errorf("remote file service unavailable")
	}
	return a.remoteFS.RemovePath(sessionID, remotePath)
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

// --- Known Hosts ---

// GetKnownHosts returns all known host entries.
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
	return a.hostKeys.Add(context.Background(), host, authorizedKey)
}

// RemoveKnownHost removes a known host entry by host pattern.
func (a *AppAPI) RemoveKnownHost(host string) error {
	if a.hostKeys == nil {
		return fmt.Errorf("host key service unavailable")
	}
	return a.hostKeys.Remove(context.Background(), host)
}
