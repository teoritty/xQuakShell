package usecase

import (
	"context"
	"os"
	"path"
	"sync"
	"unicode"

	"xquakshell/internal/domain"
)

type remoteFSSessionPort interface {
	Exec(sessionID, cmd string) (string, error)
	GetRemoteFS(sessionID string) (domain.RemoteFS, error)
	GetSessionContext(sessionID string) (context.Context, error)
}

type RemoteFSService struct {
	sessions   remoteFSSessionPort
	ownerCache map[string]map[string]string
	groupCache map[string]map[string]string
	mu         sync.Mutex
}

func NewRemoteFSService(sessions remoteFSSessionPort) *RemoteFSService {
	return &RemoteFSService{
		sessions:   sessions,
		ownerCache: make(map[string]map[string]string),
		groupCache: make(map[string]map[string]string),
	}
}

func (s *RemoteFSService) ClearSessionCache(sessionID string) {
	s.mu.Lock()
	delete(s.ownerCache, sessionID)
	delete(s.groupCache, sessionID)
	s.mu.Unlock()
}

func (s *RemoteFSService) ListPath(sessionID, dirPath string) ([]domain.RemoteNode, error) {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return nil, err
	}
	ctx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return nil, err
	}
	nodes, err := fs.List(ctx, dirPath)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if nodes[i].Owner != "" {
			nodes[i].Owner = s.resolveOwner(sessionID, nodes[i].Owner)
		}
		if nodes[i].Group != "" {
			nodes[i].Group = s.resolveGroup(sessionID, nodes[i].Group)
		}
	}
	return nodes, nil
}

func (s *RemoteFSService) MkdirPath(sessionID, parentPath, name string) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.Mkdir(ctx, path.Join(parentPath, name))
}

func (s *RemoteFSService) CreateFilePath(sessionID, parentPath, name string) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.CreateFile(ctx, path.Join(parentPath, name))
}

func (s *RemoteFSService) RenamePath(sessionID, oldPath, newPath string) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.Rename(ctx, oldPath, newPath)
}

func (s *RemoteFSService) ChmodPath(sessionID, remotePath string, mode os.FileMode) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.Chmod(ctx, remotePath, mode)
}

func (s *RemoteFSService) ChownPath(sessionID, remotePath string, uid, gid int) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return fs.Chown(ctx, remotePath, uid, gid)
}

func isNumericID(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func (s *RemoteFSService) resolveOwner(sessionID, uid string) string {
	if !isNumericID(uid) {
		return uid
	}
	s.mu.Lock()
	if s.ownerCache[sessionID] == nil {
		s.ownerCache[sessionID] = make(map[string]string)
	}
	if name, ok := s.ownerCache[sessionID][uid]; ok {
		s.mu.Unlock()
		return name
	}
	s.mu.Unlock()

	out, err := s.sessions.Exec(sessionID, "getent passwd "+uid)
	if err != nil {
		return uid
	}
	name := parseGetentName(out, uid)
	s.mu.Lock()
	s.ownerCache[sessionID][uid] = name
	s.mu.Unlock()
	return name
}

func (s *RemoteFSService) resolveGroup(sessionID, gid string) string {
	if !isNumericID(gid) {
		return gid
	}
	s.mu.Lock()
	if s.groupCache[sessionID] == nil {
		s.groupCache[sessionID] = make(map[string]string)
	}
	if name, ok := s.groupCache[sessionID][gid]; ok {
		s.mu.Unlock()
		return name
	}
	s.mu.Unlock()

	out, err := s.sessions.Exec(sessionID, "getent group "+gid)
	if err != nil {
		return gid
	}
	name := parseGetentName(out, gid)
	s.mu.Lock()
	s.groupCache[sessionID][gid] = name
	s.mu.Unlock()
	return name
}

func parseGetentName(out, fallback string) string {
	for i, r := range out {
		if r == ':' {
			if i > 0 {
				return out[:i]
			}
			break
		}
	}
	return fallback
}
