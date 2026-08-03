package sshconfig_test

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

func newService(importer *fakeImporter, conns *fakeConnRepo, idents *fakeIdentRepo) *usecase.SSHConfigImportService {
	return usecase.NewSSHConfigImportService(conns, idents, importer)
}

func TestPreviewMarksDuplicates(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "web", HostName: "web.example.com", Port: 22, User: "deploy"},
		{Alias: "db", HostName: "db.example.com", User: "deploy"},
	}}}
	conns := &fakeConnRepo{existing: []domain.Connection{{
		Host:          "WEB.EXAMPLE.COM",
		Port:          22,
		Users:         []domain.ConnectionUser{{ID: "u1", Username: "Deploy"}},
		DefaultUserID: "u1",
	}}}

	preview, err := newService(importer, conns, &fakeIdentRepo{}).Preview(context.Background(), "config")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if len(preview.Hosts) != 2 {
		t.Fatalf("got %d hosts, want 2", len(preview.Hosts))
	}
	if !preview.Hosts[0].Duplicate {
		t.Error("web already exists and must be flagged, ignoring host and user case")
	}
	if preview.Hosts[1].Duplicate {
		t.Error("db does not exist and must not be flagged")
	}
}

func TestPreviewNormalisesOmittedPortForDuplicateCheck(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "web", HostName: "web.example.com", User: "deploy"}, // no Port declared
	}}}
	conns := &fakeConnRepo{existing: []domain.Connection{{
		Host:          "web.example.com",
		Port:          22,
		Users:         []domain.ConnectionUser{{ID: "u1", Username: "deploy"}},
		DefaultUserID: "u1",
	}}}

	preview, err := newService(importer, conns, &fakeIdentRepo{}).Preview(context.Background(), "config")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !preview.Hosts[0].Duplicate {
		t.Error("an omitted Port means 22 and must match an existing port-22 connection")
	}
	if preview.Hosts[0].Port != 22 {
		t.Errorf("preview port = %d, want the SSH default surfaced to the UI", preview.Hosts[0].Port)
	}
}

func TestPreviewCountsDistinctKeyFiles(t *testing.T) {
	shared := "/home/u/.ssh/id_shared"
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "a", HostName: "a", IdentityFiles: []string{shared}},
		{Alias: "b", HostName: "b", IdentityFiles: []string{shared, "/home/u/.ssh/id_b"}},
		{Alias: "c", HostName: "c", JumpHops: []domain.SSHConfigHop{
			{Alias: "bastion", HostName: "bastion", IdentityFiles: []string{"/home/u/.ssh/id_bastion"}},
		}},
	}}}

	preview, err := newService(importer, &fakeConnRepo{}, &fakeIdentRepo{}).Preview(context.Background(), "config")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if preview.KeyFileCount != 3 {
		t.Errorf("KeyFileCount = %d, want 3 distinct files across hosts and hops", preview.KeyFileCount)
	}
	if preview.Hosts[2].KeyCount != 1 {
		t.Errorf("host c KeyCount = %d, want 1 — a hop's key counts towards its host", preview.Hosts[2].KeyCount)
	}
}

func TestPreviewPropagatesNotices(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{
		Notices: []domain.SSHConfigNotice{{Kind: domain.SSHConfigNoticeMatchBlockSkipped}},
	}}

	preview, err := newService(importer, &fakeConnRepo{}, &fakeIdentRepo{}).Preview(context.Background(), "config")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if len(preview.Notices) != 1 || preview.Notices[0].Kind != domain.SSHConfigNoticeMatchBlockSkipped {
		t.Errorf("notices = %+v, want the parser's findings surfaced unchanged", preview.Notices)
	}
}

func TestPreviewFailsWhenVaultIsLocked(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{{Alias: "web"}}}}
	conns := &fakeConnRepo{getAllErr: errVaultLocked}

	_, err := newService(importer, conns, &fakeIdentRepo{}).Preview(context.Background(), "config")

	if !errors.Is(err, domain.ErrVaultLocked) {
		t.Fatalf("err = %v, want ErrVaultLocked to reach the caller", err)
	}
}

func TestImportOnlySelectedAliases(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "web", HostName: "web.example.com", User: "deploy"},
		{Alias: "db", HostName: "db.example.com"},
		{Alias: "cache", HostName: "cache.example.com"},
	}}}
	conns := &fakeConnRepo{}

	result, err := newService(importer, conns, &fakeIdentRepo{}).Import(context.Background(), usecase.SSHConfigImportRequest{
		Path:     "config",
		Aliases:  []string{"cache", "WEB", "ghost"},
		FolderID: "folder-1",
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(result.Connections) != 2 {
		t.Fatalf("got %d connections, want 2", len(result.Connections))
	}
	// Config order is preserved regardless of the order aliases were sent in.
	if result.Connections[0].Name != "web" || result.Connections[1].Name != "cache" {
		t.Errorf("names = %q,%q; want web,cache in config order",
			result.Connections[0].Name, result.Connections[1].Name)
	}
	if len(result.SkippedAliases) != 1 || result.SkippedAliases[0] != "ghost" {
		t.Errorf("SkippedAliases = %v, want [ghost]", result.SkippedAliases)
	}
	if result.Connections[0].FolderID != "folder-1" {
		t.Errorf("FolderID = %q, want folder-1", result.Connections[0].FolderID)
	}
}

func TestImportBuildsUserAndDefaultPort(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "web", HostName: "web.example.com", User: "deploy"},
		{Alias: "anon", HostName: "anon.example.com"},
	}}}
	conns := &fakeConnRepo{}

	result, err := newService(importer, conns, &fakeIdentRepo{}).Import(context.Background(), usecase.SSHConfigImportRequest{
		Path:    "config",
		Aliases: []string{"web", "anon"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	web := result.Connections[0]
	if web.Port != 22 {
		t.Errorf("port = %d, want the SSH default applied", web.Port)
	}
	if len(web.Users) != 1 || web.Users[0].Username != "deploy" {
		t.Fatalf("users = %+v, want one user 'deploy'", web.Users)
	}
	if web.DefaultUserID != web.Users[0].ID || web.DefaultUserID == "" {
		t.Errorf("DefaultUserID = %q, want it to point at the created user", web.DefaultUserID)
	}
	if err := web.Validate(); err != nil {
		t.Errorf("imported connection must be structurally valid: %v", err)
	}

	if len(result.Connections[1].Users) != 0 {
		t.Error("a host with no User must not get an invented one")
	}
}

func TestImportMapsProxyJumpToJumpChain(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{{
		Alias: "internal", HostName: "10.0.0.5", User: "deploy",
		JumpHops: []domain.SSHConfigHop{
			{Alias: "outer", HostName: "outer.example.com", User: "jump"},
			{Alias: "inner", HostName: "inner.example.com", User: "jump", Port: 2222},
		},
	}}}}

	result, err := newService(importer, &fakeConnRepo{}, &fakeIdentRepo{}).Import(context.Background(), usecase.SSHConfigImportRequest{
		Path:    "config",
		Aliases: []string{"internal"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	hops := result.Connections[0].JumpChain.Hops
	if len(hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(hops))
	}
	if hops[0].Host != "outer.example.com" || hops[1].Host != "inner.example.com" {
		t.Errorf("hops = %+v, want traversal order preserved", hops)
	}
	if hops[0].Port != 22 || hops[1].Port != 2222 {
		t.Errorf("hop ports = %d,%d; want 22,2222", hops[0].Port, hops[1].Port)
	}
	if hops[0].ID == "" || hops[0].ID == hops[1].ID {
		t.Errorf("hop IDs must be present and unique, got %q and %q", hops[0].ID, hops[1].ID)
	}
	if err := result.Connections[0].Validate(); err != nil {
		t.Errorf("imported connection must be structurally valid: %v", err)
	}
}

func TestImportAbortsOnSaveFailure(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "a", HostName: "a"}, {Alias: "b", HostName: "b"}, {Alias: "c", HostName: "c"},
	}}}
	conns := &fakeConnRepo{saveErr: errVaultLocked}

	result, err := newService(importer, conns, &fakeIdentRepo{}).Import(context.Background(), usecase.SSHConfigImportRequest{
		Path:    "config",
		Aliases: []string{"a", "b", "c"},
	})

	if !errors.Is(err, domain.ErrVaultLocked) {
		t.Fatalf("err = %v, want ErrVaultLocked", err)
	}
	if len(result.Connections) != 0 {
		t.Errorf("no connection was saved, so none may be reported: %+v", result.Connections)
	}
	if conns.saveCalls != 1 {
		t.Errorf("Save called %d times; a locked vault must abort, not retry per host", conns.saveCalls)
	}
}

func TestImportReportsPartialProgress(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
		{Alias: "a", HostName: "a"}, {Alias: "b", HostName: "b"},
	}}}
	conns := &failOnSecondSave{}

	result, err := usecase.NewSSHConfigImportService(conns, &fakeIdentRepo{}, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{Path: "config", Aliases: []string{"a", "b"}})

	if err == nil {
		t.Fatal("want an error when a save fails partway")
	}
	if len(result.Connections) != 1 || result.Connections[0].Name != "a" {
		t.Errorf("connections = %+v, want the one that was actually saved", result.Connections)
	}
}

func TestImportReParsesRatherThanTrustingThePreview(t *testing.T) {
	importer := &fakeImporter{result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{{Alias: "web", HostName: "web"}}}}
	svc := newService(importer, &fakeConnRepo{}, &fakeIdentRepo{})

	if _, err := svc.Preview(context.Background(), "config"); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if _, err := svc.Import(context.Background(), usecase.SSHConfigImportRequest{Path: "config", Aliases: []string{"web"}}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(importer.parsedPaths) != 2 {
		t.Errorf("Parse called %d times, want 2: Import must read the file itself", len(importer.parsedPaths))
	}
}

func TestImportPropagatesParseError(t *testing.T) {
	importer := &fakeImporter{parseErr: domain.ErrSSHConfigNotFound}

	_, err := newService(importer, &fakeConnRepo{}, &fakeIdentRepo{}).
		Import(context.Background(), usecase.SSHConfigImportRequest{Path: "gone", Aliases: []string{"web"}})

	if !errors.Is(err, domain.ErrSSHConfigNotFound) {
		t.Fatalf("err = %v, want ErrSSHConfigNotFound", err)
	}
}

// failOnSecondSave lets the first connection through and then fails, modelling
// a vault that becomes unwritable mid-import.
type failOnSecondSave struct {
	fakeConnRepo
	calls int
}

func (f *failOnSecondSave) Save(ctx context.Context, c *domain.Connection) error {
	f.calls++
	if f.calls > 1 {
		return errors.New("disk full")
	}
	return f.fakeConnRepo.Save(ctx, c)
}
