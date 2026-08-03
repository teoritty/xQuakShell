package wails

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// ctxT keeps the repository stub's long method set readable; the interface it
// satisfies takes a context on every call.
type ctxT = context.Context

// stubSSHConfigImporter is a scripted domain.SSHConfigImporter. The handler
// tests live in-package so they can wire the unexported service field without
// widening the production API with a test-only setter.
type stubSSHConfigImporter struct {
	defaultPath string
	result      domain.SSHConfigParseResult
	parseErr    error
}

func (s *stubSSHConfigImporter) DefaultPath() (string, error) { return s.defaultPath, nil }

func (s *stubSSHConfigImporter) Parse(string) (domain.SSHConfigParseResult, error) {
	if s.parseErr != nil {
		return domain.SSHConfigParseResult{}, s.parseErr
	}
	return s.result, nil
}

func (s *stubSSHConfigImporter) ReadKeyFile(string) ([]byte, error) {
	return nil, errors.New("no keys in this stub")
}

// stubConnRepo is a connection repository that can be told to behave as a
// locked vault.
type stubConnRepo struct {
	saved  []domain.Connection
	err    error
	nextID int
}

func (s *stubConnRepo) GetAllConnections(_ ctxT) ([]domain.Connection, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.saved, nil
}

func (s *stubConnRepo) Save(_ ctxT, c *domain.Connection) error {
	if s.err != nil {
		return s.err
	}
	s.nextID++
	c.ID = "c" + string(rune('0'+s.nextID))
	s.saved = append(s.saved, *c)
	return nil
}

func (s *stubConnRepo) GetAllFolders(ctxT) ([]domain.ConnectionFolder, error) { return nil, nil }
func (s *stubConnRepo) SaveFolder(ctxT, *domain.ConnectionFolder) error       { return nil }
func (s *stubConnRepo) DeleteFolder(ctxT, string) error                       { return nil }
func (s *stubConnRepo) GetByFolder(ctxT, string) ([]domain.Connection, error) { return nil, nil }
func (s *stubConnRepo) GetByID(ctxT, string) (*domain.Connection, error) {
	return nil, errors.New("not implemented")
}
func (s *stubConnRepo) Delete(ctxT, string) error                       { return nil }
func (s *stubConnRepo) MoveToFolder(ctxT, []string, string) error       { return nil }
func (s *stubConnRepo) MoveFolder(ctxT, string, string) error           { return nil }
func (s *stubConnRepo) ReorderConnections(ctxT, []string, string) error { return nil }
func (s *stubConnRepo) ReorderFolders(ctxT, []string, string) error     { return nil }

func newSSHConfigTestAPI(importer *stubSSHConfigImporter, conns *stubConnRepo) *AppAPI {
	return &AppAPI{sshConfigImport: usecase.NewSSHConfigImportService(conns, nil, importer)}
}

func twoHostResult() domain.SSHConfigParseResult {
	return domain.SSHConfigParseResult{
		Hosts: []domain.SSHConfigHost{
			{Alias: "web", HostName: "web.example.com", Port: 2222, User: "deploy"},
			{Alias: "db", HostName: "db.example.com"},
		},
		Notices: []domain.SSHConfigNotice{{Kind: domain.SSHConfigNoticeMatchBlockSkipped}},
	}
}

func TestPreviewSSHConfigHappyPath(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{result: twoHostResult()}, &stubConnRepo{})

	dto, err := api.PreviewSSHConfig("/home/u/.ssh/config")
	if err != nil {
		t.Fatalf("PreviewSSHConfig: %v", err)
	}

	if len(dto.Hosts) != 2 {
		t.Fatalf("got %d hosts, want 2", len(dto.Hosts))
	}
	if dto.Hosts[0].Alias != "web" || dto.Hosts[0].Port != 2222 || dto.Hosts[0].User != "deploy" {
		t.Errorf("host = %+v, want the parsed values mapped through", dto.Hosts[0])
	}
	if dto.Hosts[1].Port != 22 {
		t.Errorf("port = %d, want the SSH default surfaced", dto.Hosts[1].Port)
	}
	if len(dto.Notices) != 1 || dto.Notices[0].Kind != string(domain.SSHConfigNoticeMatchBlockSkipped) {
		t.Errorf("notices = %+v, want the parser finding carried across", dto.Notices)
	}
	if dto.Hosts[0].JumpAliases == nil {
		t.Error("optional lists must serialise as [] rather than null")
	}
}

func TestPreviewSSHConfigRejectsEmptyPath(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{result: twoHostResult()}, &stubConnRepo{})

	for _, path := range []string{"", "   "} {
		if _, err := api.PreviewSSHConfig(path); err == nil {
			t.Errorf("PreviewSSHConfig(%q) must be rejected", path)
		}
	}
}

func TestPreviewSSHConfigFileNotFound(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{parseErr: domain.ErrSSHConfigNotFound}, &stubConnRepo{})

	_, err := api.PreviewSSHConfig("/home/u/.ssh/absent")

	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a user-facing not-found message", err)
	}
}

func TestPreviewSSHConfigLockedVault(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{result: twoHostResult()}, &stubConnRepo{err: domain.ErrVaultLocked})

	_, err := api.PreviewSSHConfig("/home/u/.ssh/config")

	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("err = %v, want a vault-locked message", err)
	}
}

func TestSSHConfigHandlersWithoutService(t *testing.T) {
	api := &AppAPI{}

	if _, err := api.GetSSHConfigDefaultPath(); err == nil {
		t.Error("GetSSHConfigDefaultPath must fail when the service is not wired")
	}
	if _, err := api.PreviewSSHConfig("/config"); err == nil {
		t.Error("PreviewSSHConfig must fail when the service is not wired")
	}
	if _, err := api.ImportSSHConfig("/config", []string{"web"}, "", false); err == nil {
		t.Error("ImportSSHConfig must fail when the service is not wired")
	}
}

func TestImportSSHConfigHappyPath(t *testing.T) {
	conns := &stubConnRepo{}
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{result: twoHostResult()}, conns)

	dto, err := api.ImportSSHConfig("/home/u/.ssh/config", []string{"web"}, "folder-1", false)
	if err != nil {
		t.Fatalf("ImportSSHConfig: %v", err)
	}

	if len(dto.Connections) != 1 {
		t.Fatalf("got %d connections, want 1", len(dto.Connections))
	}
	if dto.Connections[0].Name != "web" || dto.Connections[0].FolderID != "folder-1" {
		t.Errorf("connection = %+v, want web in folder-1", dto.Connections[0])
	}
	if dto.SkippedAliases == nil {
		t.Error("optional lists must serialise as [] rather than null")
	}
}

func TestImportSSHConfigRejectsInvalidInput(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{result: twoHostResult()}, &stubConnRepo{})

	tests := []struct {
		name    string
		path    string
		aliases []string
	}{
		{name: "empty path", path: "", aliases: []string{"web"}},
		{name: "blank path", path: "   ", aliases: []string{"web"}},
		{name: "no aliases", path: "/config", aliases: nil},
		{name: "too many aliases", path: "/config", aliases: make([]string, maxImportAliases+1)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := api.ImportSSHConfig(tc.path, tc.aliases, "", false); err == nil {
				t.Error("want the request rejected before any work is done")
			}
		})
	}
}

func TestImportSSHConfigLockedVaultReportsPartialProgress(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{result: twoHostResult()}, &stubConnRepo{err: domain.ErrVaultLocked})

	dto, err := api.ImportSSHConfig("/config", []string{"web", "db"}, "", false)

	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("err = %v, want a vault-locked message", err)
	}
	if len(dto.Connections) != 0 {
		t.Errorf("connections = %+v, want none reported when nothing was saved", dto.Connections)
	}
}

// Error text is the one place internal detail leaks most easily, so the
// mapping is asserted directly: an unrecognised error must never be forwarded.
func TestSSHConfigUserErrorHidesInternalDetail(t *testing.T) {
	internal := errors.New("open /home/alice/.ssh/id_rsa: permission denied")

	got := sshConfigUserError(internal)

	if got == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(got.Error(), "/home/alice") || strings.Contains(got.Error(), "permission denied") {
		t.Errorf("error = %q leaks internal detail", got.Error())
	}
	if sshConfigUserError(nil) != nil {
		t.Error("nil must map to nil")
	}
}

func TestGetSSHConfigDefaultPath(t *testing.T) {
	api := newSSHConfigTestAPI(&stubSSHConfigImporter{defaultPath: "/home/u/.ssh/config"}, &stubConnRepo{})

	got, err := api.GetSSHConfigDefaultPath()

	if err != nil {
		t.Fatalf("GetSSHConfigDefaultPath: %v", err)
	}
	if got != "/home/u/.ssh/config" {
		t.Errorf("got %q, want the detected path", got)
	}
}
