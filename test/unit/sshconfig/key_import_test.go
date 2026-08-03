package sshconfig_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

const sharedKeyPath = "/home/u/.ssh/id_shared"

// sharedKeyImporter scripts two hosts that reference the same key file plus
// one host with a key of its own.
func sharedKeyImporter() *fakeImporter {
	return &fakeImporter{
		result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{
			{Alias: "a", HostName: "a", User: "deploy", IdentityFiles: []string{sharedKeyPath}},
			{Alias: "b", HostName: "b", User: "deploy", IdentityFiles: []string{sharedKeyPath}},
			{Alias: "c", HostName: "c", User: "deploy", IdentityFiles: []string{"/home/u/.ssh/id_c"}},
		}},
		keyData: map[string]string{
			sharedKeyPath:         "SHARED KEY",
			"/home/u/.ssh/id_c":   "C KEY",
			"/home/u/.ssh/id_hop": "HOP KEY",
		},
	}
}

func TestImportSkipsKeysWhenNotRequested(t *testing.T) {
	importer := sharedKeyImporter()
	idents := &fakeIdentRepo{}

	result, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, idents, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"a", "b", "c"}, ImportKeys: false,
		})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(importer.readKeyPaths) != 0 {
		t.Errorf("no key may be read when import was not requested, got %v", importer.readKeyPaths)
	}
	if len(idents.imported) != 0 || result.ImportedKeys != 0 {
		t.Errorf("no identity may be stored, got %d", len(idents.imported))
	}
	if result.Connections[0].Users[0].KeyAuth != nil {
		t.Error("without imported keys the user must carry no identity reference")
	}
}

func TestImportStoresEachKeyOnce(t *testing.T) {
	importer := sharedKeyImporter()
	idents := &fakeIdentRepo{}

	result, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, idents, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"a", "b", "c"}, ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.ImportedKeys != 2 {
		t.Errorf("ImportedKeys = %d, want 2 distinct files", result.ImportedKeys)
	}
	if len(idents.imported) != 2 {
		t.Fatalf("stored %d identities, want 2 — a shared key must not be duplicated", len(idents.imported))
	}

	// Both hosts that share the key must reference the same identity.
	first := result.Connections[0].Users[0].KeyAuth
	second := result.Connections[1].Users[0].KeyAuth
	if first == nil || second == nil {
		t.Fatalf("both hosts must reference an identity, got %+v and %+v", first, second)
	}
	if first.IdentityIDs[0] != second.IdentityIDs[0] {
		t.Errorf("identity ids %v and %v differ; the shared key must be reused",
			first.IdentityIDs, second.IdentityIDs)
	}
}

func TestImportLabelsIdentityWithBaseNameOnly(t *testing.T) {
	importer := sharedKeyImporter()
	idents := &fakeIdentRepo{}

	_, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, idents, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"a"}, ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	comment := idents.imported[0].comment
	if !strings.Contains(comment, "id_shared") {
		t.Errorf("comment = %q, want the key's file name", comment)
	}
	if strings.Contains(comment, "/home/u") {
		t.Errorf("comment = %q leaks the home directory layout", comment)
	}
}

func TestImportAttachesHopKeys(t *testing.T) {
	importer := &fakeImporter{
		result: domain.SSHConfigParseResult{Hosts: []domain.SSHConfigHost{{
			Alias: "target", HostName: "target", User: "deploy",
			JumpHops: []domain.SSHConfigHop{
				{Alias: "bastion", HostName: "bastion", User: "jump", IdentityFiles: []string{"/home/u/.ssh/id_hop"}},
			},
		}}},
		keyData: map[string]string{"/home/u/.ssh/id_hop": "HOP KEY"},
	}

	result, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, &fakeIdentRepo{}, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"target"}, ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	hop := result.Connections[0].JumpChain.Hops[0]
	if hop.KeyAuth == nil || len(hop.KeyAuth.IdentityIDs) != 1 {
		t.Fatalf("hop KeyAuth = %+v, want the hop's own key attached", hop.KeyAuth)
	}
	if err := hop.Validate(); err != nil {
		t.Errorf("a hop with user and key must be connect-ready: %v", err)
	}
}

func TestImportSurvivesUnreadableKey(t *testing.T) {
	importer := sharedKeyImporter()
	importer.keyErr = domain.ErrSSHConfigUnreadable

	result, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, &fakeIdentRepo{}, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"a", "c"}, ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("an unreadable key must not fail the import: %v", err)
	}

	if len(result.Connections) != 2 {
		t.Errorf("got %d connections, want both saved despite the key failures", len(result.Connections))
	}
	if result.FailedKeys != 2 {
		t.Errorf("FailedKeys = %d, want 2 distinct failures reported", result.FailedKeys)
	}
	if result.Connections[0].Users[0].KeyAuth != nil {
		t.Error("a connection whose key failed must carry no dangling identity reference")
	}
}

func TestImportDoesNotRetryAFailedKey(t *testing.T) {
	importer := sharedKeyImporter()
	importer.keyErr = errors.New("permission denied")

	_, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, &fakeIdentRepo{}, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"a", "b"}, ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Both hosts reference the same key; the second must not re-read it.
	if len(importer.readKeyPaths) != 1 {
		t.Errorf("ReadKeyFile called %d times, want 1: a failed key must not be retried per host",
			len(importer.readKeyPaths))
	}
}

func TestImportSurvivesIdentityStoreFailure(t *testing.T) {
	importer := sharedKeyImporter()
	idents := &fakeIdentRepo{importErr: errors.New("vault write failed")}

	result, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, idents, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path: "config", Aliases: []string{"a"}, ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("a failing identity store must not fail the import: %v", err)
	}

	if len(result.Connections) != 1 {
		t.Errorf("the connection must still be created, got %d", len(result.Connections))
	}
	if result.ImportedKeys != 0 || result.FailedKeys != 1 {
		t.Errorf("ImportedKeys=%d FailedKeys=%d; want 0 and 1", result.ImportedKeys, result.FailedKeys)
	}
}

// This is the security-critical property of the whole feature: the import path
// must never read a file the parse result did not name. The frontend supplies
// only a config path and a list of aliases, so if this holds, no caller can
// steer the importer at an arbitrary file.
func TestImportNeverReadsAPathOutsideTheParseResult(t *testing.T) {
	importer := sharedKeyImporter()
	importer.result.Hosts = append(importer.result.Hosts, domain.SSHConfigHost{
		Alias: "hopper", HostName: "hopper", User: "deploy",
		JumpHops: []domain.SSHConfigHop{
			{Alias: "bastion", HostName: "bastion", User: "jump", IdentityFiles: []string{"/home/u/.ssh/id_hop"}},
		},
	})

	_, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, &fakeIdentRepo{}, importer).
		Import(context.Background(), usecase.SSHConfigImportRequest{
			Path:       "config",
			Aliases:    []string{"a", "b", "c", "hopper", "/etc/shadow", "../../secrets.txt"},
			ImportKeys: true,
		})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	known := importer.knownKeyPaths()
	if len(importer.readKeyPaths) == 0 {
		t.Fatal("expected some keys to be read, otherwise this test proves nothing")
	}
	for _, read := range importer.readKeyPaths {
		if !known[read] {
			t.Errorf("ReadKeyFile(%q) is outside the parse result — the importer must not be steerable", read)
		}
	}
}

func TestDefaultPathIsPassedThrough(t *testing.T) {
	importer := &fakeImporter{defaultPath: "/home/u/.ssh/config"}

	got, err := usecase.NewSSHConfigImportService(&fakeConnRepo{}, &fakeIdentRepo{}, importer).DefaultPath()

	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if got != "/home/u/.ssh/config" {
		t.Errorf("got %q, want the importer's answer", got)
	}
}

func TestServiceWithoutImporterFailsCleanly(t *testing.T) {
	svc := usecase.NewSSHConfigImportService(&fakeConnRepo{}, &fakeIdentRepo{}, nil)

	if _, err := svc.DefaultPath(); err == nil {
		t.Error("DefaultPath must fail when no importer is wired")
	}
	if _, err := svc.Preview(context.Background(), "config"); err == nil {
		t.Error("Preview must fail when no importer is wired")
	}
	if _, err := svc.Import(context.Background(), usecase.SSHConfigImportRequest{Path: "config"}); err == nil {
		t.Error("Import must fail when no importer is wired")
	}
}
