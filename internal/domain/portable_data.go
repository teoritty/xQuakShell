package domain

import "errors"

// ErrPortablePathDenied is returned when a path escapes the portable data root.
var ErrPortablePathDenied = errors.New("portable path access denied")

// PortableDataStore manages internal application state under the portable data root (ADR-006).
//
// All paths resolved through this port must stay under <exe>/data. Do not use this port
// for Local Files browser, SFTP transfers, or file dialogs — use HostFileSystem (ADR-007).
type PortableDataStore interface {
	DataRoot() string
	TempDir() string
	EnsureTempDir() (string, error)
	ResolvePath(path string) (string, error)
}
