package sshconfig

import "xquakshell/internal/domain"

// PortAdapter implements domain.SSHConfigImporter using this package.
//
// It is stateless: every call re-reads the configuration from disk, so an
// import always acts on the file as it is now rather than on a snapshot taken
// when the dialog was opened.
type PortAdapter struct{}

// DefaultPath implements domain.SSHConfigImporter.
func (PortAdapter) DefaultPath() (string, error) { return DefaultPath() }

// Parse implements domain.SSHConfigImporter.
func (PortAdapter) Parse(path string) (domain.SSHConfigParseResult, error) { return Parse(path) }

// ReadKeyFile implements domain.SSHConfigImporter.
func (PortAdapter) ReadKeyFile(path string) ([]byte, error) { return ReadKeyFile(path) }
