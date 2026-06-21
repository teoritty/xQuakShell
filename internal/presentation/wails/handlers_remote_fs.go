package wails

import (
	"context"
	"fmt"
	"path"
	"strings"
	"unicode"

	gossh "golang.org/x/crypto/ssh"
)

// --- SFTP remote operations ---

// runSSHCommand executes a command on the remote host via SSH.
func (a *AppAPI) runSSHCommand(sessionID, cmd string) (string, error) {
	sshClient, err := a.sessions.GetSSHClient(sessionID)
	if err != nil {
		return "", err
	}
	session, err := sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// isNumeric returns true if s contains only digits.
func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// resolveOwner resolves UID to username via getent passwd; uses cache.
func (a *AppAPI) resolveOwner(sessionID, uid string) string {
	if !isNumeric(uid) {
		return uid
	}
	a.ownerCacheMu.Lock()
	if a.ownerCache[sessionID] == nil {
		a.ownerCache[sessionID] = make(map[string]string)
	}
	if name, ok := a.ownerCache[sessionID][uid]; ok {
		a.ownerCacheMu.Unlock()
		return name
	}
	a.ownerCacheMu.Unlock()
	out, err := a.runSSHCommand(sessionID, "getent passwd "+uid)
	if err != nil {
		return uid
	}
	fields := strings.SplitN(out, ":", 2)
	name := uid
	if len(fields) >= 1 && fields[0] != "" {
		name = fields[0]
	}
	a.ownerCacheMu.Lock()
	a.ownerCache[sessionID][uid] = name
	a.ownerCacheMu.Unlock()
	return name
}

// resolveGroup resolves GID to group name via getent group; uses cache.
func (a *AppAPI) resolveGroup(sessionID, gid string) string {
	if !isNumeric(gid) {
		return gid
	}
	a.ownerCacheMu.Lock()
	if a.groupCache[sessionID] == nil {
		a.groupCache[sessionID] = make(map[string]string)
	}
	if name, ok := a.groupCache[sessionID][gid]; ok {
		a.ownerCacheMu.Unlock()
		return name
	}
	a.ownerCacheMu.Unlock()
	out, err := a.runSSHCommand(sessionID, "getent group "+gid)
	if err != nil {
		return gid
	}
	fields := strings.SplitN(out, ":", 2)
	name := gid
	if len(fields) >= 1 && fields[0] != "" {
		name = fields[0]
	}
	a.ownerCacheMu.Lock()
	a.groupCache[sessionID][gid] = name
	a.ownerCacheMu.Unlock()
	return name
}

// ListPath lists the contents of a remote directory.
func (a *AppAPI) ListPath(sessionID, dirPath string) ([]RemoteNodeDTO, error) {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return nil, err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return nil, err
	}
	nodes, err := fs.List(ctx, dirPath)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if nodes[i].Owner != "" {
			nodes[i].Owner = a.resolveOwner(sessionID, nodes[i].Owner)
		}
		if nodes[i].Group != "" {
			nodes[i].Group = a.resolveGroup(sessionID, nodes[i].Group)
		}
	}
	return RemoteNodesToDTO(nodes), nil
}

// RemovePath deletes a remote file or directory (recursively for directories).
func (a *AppAPI) RemovePath(sessionID, remotePath string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.RemoveAll(ctx, remotePath)
}

// MkdirPath creates a remote directory (and parents if needed).
func (a *AppAPI) MkdirPath(sessionID, parentPath, name string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	fullPath := path.Join(parentPath, name)
	return fs.Mkdir(ctx, fullPath)
}

// CreateFilePath creates an empty remote file.
func (a *AppAPI) CreateFilePath(sessionID, parentPath, name string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	fullPath := path.Join(parentPath, name)
	return fs.CreateFile(ctx, fullPath)
}

// RenamePath renames a remote file or directory.
func (a *AppAPI) RenamePath(sessionID, oldPath, newPath string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.Rename(ctx, oldPath, newPath)
}

// --- Known Hosts ---

// GetKnownHosts returns all known host entries.
func (a *AppAPI) GetKnownHosts() ([]KnownHostDTO, error) {
	entries, err := a.knownHosts.List()
	if err != nil {
		return nil, err
	}
	return KnownHostsToDTO(entries), nil
}

// AddKnownHost adds a known host entry from an authorized_key formatted string.
func (a *AppAPI) AddKnownHost(host, authorizedKey string) error {
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return fmt.Errorf("parse authorized key: %w", err)
	}
	return a.knownHosts.Add(context.Background(), host, key)
}

// RemoveKnownHost removes a known host entry by host pattern.
func (a *AppAPI) RemoveKnownHost(host string) error {
	return a.knownHosts.Remove(context.Background(), host)
}
