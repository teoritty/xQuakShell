package portable

import (
	"fmt"
	"os"
	"path/filepath"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/pathsafe"
)

// DataStore implements domain.PortableDataStore for jailed application state (ADR-006).
type DataStore struct {
	root     string
	tempDir  string
	portable domain.PortableRuntime
}

// NewDataStore creates a portable data store rooted at dataRoot.
func NewDataStore(dataRoot, tempDir string, portableRuntime domain.PortableRuntime) *DataStore {
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		abs = filepath.Clean(dataRoot)
	} else {
		abs = filepath.Clean(abs)
	}
	tempAbs := tempDir
	if tempDir != "" {
		if resolved, err := filepath.Abs(tempDir); err == nil {
			tempAbs = filepath.Clean(resolved)
		}
	}
	return &DataStore{
		root:     abs,
		tempDir:  tempAbs,
		portable: portableRuntime,
	}
}

// DataRoot returns the absolute portable data root.
func (s *DataStore) DataRoot() string {
	return s.root
}

// TempDir returns the portable temp directory path.
func (s *DataStore) TempDir() string {
	return s.tempDir
}

// EnsureTempDir creates the portable temp directory when writable.
func (s *DataStore) EnsureTempDir() (string, error) {
	if s == nil || s.tempDir == "" {
		return "", fmt.Errorf("portable temp directory unavailable")
	}
	if err := s.requireWritable(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.tempDir, 0o700); err != nil {
		return "", err
	}
	return s.tempDir, nil
}

// ResolvePath normalizes path and verifies it stays under the data root.
func (s *DataStore) ResolvePath(path string) (string, error) {
	if s == nil || s.root == "" {
		return "", fmt.Errorf("portable data store unavailable")
	}
	if path == "" {
		return s.root, nil
	}
	resolved, err := pathsafe.ResolveUnderRoot(s.root, path)
	if err != nil {
		return "", domain.ErrPortablePathDenied
	}
	return resolved, nil
}

func (s *DataStore) requireWritable() error {
	if s == nil || s.portable == nil {
		return nil
	}
	return s.portable.RequireWritable()
}

var _ domain.PortableDataStore = (*DataStore)(nil)
