// Package sshconfig_test exercises the ssh_config import use case against
// in-memory repositories, covering the orchestration that the parser tests in
// internal/infra/sshconfig deliberately leave out.
package sshconfig_test

import (
	"context"
	"errors"
	"sync"

	"xquakshell/internal/domain"
)

// errVaultLocked stands in for the repository refusing to write.
var errVaultLocked = domain.ErrVaultLocked

// fakeConnRepo implements the parts of domain.ConnectionRepository the import
// service uses; the rest panic so an accidental new dependency is loud.
type fakeConnRepo struct {
	mu          sync.Mutex
	existing    []domain.Connection
	saved       []domain.Connection
	saveErr     error
	getAllErr   error
	saveCalls   int
	nextIDIndex int
}

func (f *fakeConnRepo) GetAllConnections(context.Context) ([]domain.Connection, error) {
	if f.getAllErr != nil {
		return nil, f.getAllErr
	}
	return f.existing, nil
}

func (f *fakeConnRepo) Save(_ context.Context, c *domain.Connection) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.nextIDIndex++
	c.ID = "conn-" + string(rune('a'+f.nextIDIndex-1))
	f.saved = append(f.saved, *c)
	return nil
}

func (f *fakeConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return nil, nil
}
func (f *fakeConnRepo) SaveFolder(context.Context, *domain.ConnectionFolder) error { return nil }
func (f *fakeConnRepo) DeleteFolder(context.Context, string) error                 { return nil }
func (f *fakeConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}
func (f *fakeConnRepo) GetByID(context.Context, string) (*domain.Connection, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeConnRepo) Delete(context.Context, string) error                       { return nil }
func (f *fakeConnRepo) MoveToFolder(context.Context, []string, string) error       { return nil }
func (f *fakeConnRepo) MoveFolder(context.Context, string, string) error           { return nil }
func (f *fakeConnRepo) ReorderConnections(context.Context, []string, string) error { return nil }
func (f *fakeConnRepo) ReorderFolders(context.Context, []string, string) error     { return nil }

// fakeIdentRepo records imported key material so tests can assert both how
// many identities were created and which bytes reached the vault.
type fakeIdentRepo struct {
	mu        sync.Mutex
	imported  []importedIdentity
	importErr error
	counter   int
}

type importedIdentity struct {
	data    string
	comment string
}

func (f *fakeIdentRepo) Import(_ context.Context, pemData []byte, comment string) (*domain.SSHIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.importErr != nil {
		return nil, f.importErr
	}
	f.counter++
	f.imported = append(f.imported, importedIdentity{data: string(pemData), comment: comment})
	return &domain.SSHIdentity{ID: "ident-" + string(rune('a'+f.counter-1)), Comment: comment}, nil
}

func (f *fakeIdentRepo) GetAll(context.Context) ([]domain.SSHIdentity, error) { return nil, nil }
func (f *fakeIdentRepo) GetKeyBlob(context.Context, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeIdentRepo) Delete(context.Context, string) error { return nil }

// fakeImporter is a scripted domain.SSHConfigImporter. It records every path
// ReadKeyFile is asked for, which is what lets a test prove the service never
// reads a file the parse result did not name.
type fakeImporter struct {
	mu sync.Mutex

	defaultPath string
	result      domain.SSHConfigParseResult
	parseErr    error
	keyData     map[string]string
	keyErr      error

	parsedPaths  []string
	readKeyPaths []string
}

func (f *fakeImporter) DefaultPath() (string, error) { return f.defaultPath, nil }

func (f *fakeImporter) Parse(path string) (domain.SSHConfigParseResult, error) {
	f.mu.Lock()
	f.parsedPaths = append(f.parsedPaths, path)
	f.mu.Unlock()
	if f.parseErr != nil {
		return domain.SSHConfigParseResult{}, f.parseErr
	}
	return f.result, nil
}

func (f *fakeImporter) ReadKeyFile(path string) ([]byte, error) {
	f.mu.Lock()
	f.readKeyPaths = append(f.readKeyPaths, path)
	f.mu.Unlock()
	if f.keyErr != nil {
		return nil, f.keyErr
	}
	data, ok := f.keyData[path]
	if !ok {
		return nil, errors.New("no such key")
	}
	return []byte(data), nil
}

// knownKeyPaths lists every key path the scripted parse result exposes.
func (f *fakeImporter) knownKeyPaths() map[string]bool {
	known := map[string]bool{}
	for _, host := range f.result.Hosts {
		for _, p := range host.IdentityFiles {
			known[p] = true
		}
		for _, hop := range host.JumpHops {
			for _, p := range hop.IdentityFiles {
				known[p] = true
			}
		}
	}
	return known
}
