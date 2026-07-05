package usecase

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ssh-client/internal/domain"
)

type memoryConnRepo struct {
	folders      []domain.ConnectionFolder
	conns        map[string]domain.Connection
	saveCalls    int
	getByIDCalls int
	saveErr      error
	getByIDErr   error
}

func newMemoryConnRepo() *memoryConnRepo {
	return &memoryConnRepo{conns: make(map[string]domain.Connection)}
}

func (m *memoryConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return m.folders, nil
}

func (m *memoryConnRepo) SaveFolder(_ context.Context, f *domain.ConnectionFolder) error {
	if f.ID == "" {
		f.ID = "folder-1"
	}
	m.folders = append(m.folders, *f)
	return nil
}

func (m *memoryConnRepo) DeleteFolder(_ context.Context, id string) error {
	for i, f := range m.folders {
		if f.ID == id {
			m.folders = append(m.folders[:i], m.folders[i+1:]...)
			return nil
		}
	}
	return errors.New("folder not found")
}

func (m *memoryConnRepo) GetAllConnections(_ context.Context) ([]domain.Connection, error) {
	out := make([]domain.Connection, 0, len(m.conns))
	for _, c := range m.conns {
		out = append(out, c)
	}
	return out, nil
}

func (m *memoryConnRepo) GetByFolder(context.Context, string) ([]domain.Connection, error) {
	return nil, nil
}

func (m *memoryConnRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	m.getByIDCalls++
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	c, ok := m.conns[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &c, nil
}

func (m *memoryConnRepo) Save(_ context.Context, c *domain.Connection) error {
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	if c.ID == "" {
		c.ID = "conn-1"
	}
	m.conns[c.ID] = *c
	return nil
}

func (m *memoryConnRepo) Delete(_ context.Context, id string) error {
	delete(m.conns, id)
	return nil
}

func (m *memoryConnRepo) MoveToFolder(context.Context, []string, string) error { return nil }
func (m *memoryConnRepo) MoveFolder(context.Context, string, string) error     { return nil }
func (m *memoryConnRepo) ReorderConnections(context.Context, []string, string) error {
	return nil
}
func (m *memoryConnRepo) ReorderFolders(context.Context, []string, string) error { return nil }

type memoryPasswordRepo struct {
	ids     []string
	importN int
}

func (m *memoryPasswordRepo) Import(_ context.Context, _ []byte, _ string) (string, error) {
	m.importN++
	id := "pw-1"
	m.ids = append(m.ids, id)
	return id, nil
}

func (m *memoryPasswordRepo) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (m *memoryPasswordRepo) Delete(_ context.Context, id string) error {
	for i, existing := range m.ids {
		if existing == id {
			m.ids = append(m.ids[:i], m.ids[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *memoryPasswordRepo) List(context.Context) ([]domain.PasswordBlob, error) { return nil, nil }

type memoryIdentityRepo struct {
	idents  []domain.SSHIdentity
	importN int
}

func (m *memoryIdentityRepo) GetAll(_ context.Context) ([]domain.SSHIdentity, error) {
	return m.idents, nil
}

func (m *memoryIdentityRepo) GetKeyBlob(context.Context, string) ([]byte, error) { return nil, nil }

func (m *memoryIdentityRepo) Import(_ context.Context, _ []byte, comment string) (*domain.SSHIdentity, error) {
	m.importN++
	id := domain.SSHIdentity{ID: "id-1", Comment: comment}
	m.idents = append(m.idents, id)
	return &id, nil
}

func (m *memoryIdentityRepo) Delete(context.Context, string) error { return nil }

func newTestVaultService(conn domain.ConnectionRepository, pw domain.PasswordRepository, ident domain.IdentityRepository, ping *PingManager) *VaultService {
	return NewVaultService(VaultServiceConfig{
		ConnRepo:     conn,
		PasswordRepo: pw,
		IdentRepo:    ident,
		PingMgr:      ping,
	})
}

func TestNewVaultServicePanicsOnNilRepos(t *testing.T) {
	conn := newMemoryConnRepo()
	pw := &memoryPasswordRepo{}
	ident := &memoryIdentityRepo{}

	t.Run("nil ConnRepo", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for nil ConnRepo")
			}
		}()
		NewVaultService(VaultServiceConfig{PasswordRepo: pw, IdentRepo: ident})
	})

	t.Run("nil PasswordRepo", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for nil PasswordRepo")
			}
		}()
		NewVaultService(VaultServiceConfig{ConnRepo: conn, IdentRepo: ident})
	})

	t.Run("nil IdentRepo", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for nil IdentRepo")
			}
		}()
		NewVaultService(VaultServiceConfig{ConnRepo: conn, PasswordRepo: pw})
	})
}

func TestVaultServiceSaveConnectionReloadsAfterSave(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	svc := newTestVaultService(connRepo, &memoryPasswordRepo{}, &memoryIdentityRepo{}, nil)

	conn := &domain.Connection{Name: "Server", Host: "10.0.0.1", Port: 22, Protocol: domain.ProtocolSSH}
	saved, err := svc.SaveConnection(ctx, conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if connRepo.saveCalls != 1 {
		t.Fatalf("save calls: got %d, want 1", connRepo.saveCalls)
	}
	if connRepo.getByIDCalls != 1 {
		t.Fatalf("getByID calls: got %d, want 1", connRepo.getByIDCalls)
	}
	if saved.ID != "conn-1" {
		t.Fatalf("saved id: got %q, want conn-1", saved.ID)
	}
}

func TestVaultServiceSaveConnectionPropagatesSaveError(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	connRepo.saveErr = errors.New("save failed")
	svc := newTestVaultService(connRepo, &memoryPasswordRepo{}, &memoryIdentityRepo{}, nil)

	_, err := svc.SaveConnection(ctx, &domain.Connection{Host: "h"}, nil)
	if err == nil || err.Error() != "save failed" {
		t.Fatalf("expected save error, got %v", err)
	}
	if connRepo.getByIDCalls != 0 {
		t.Fatalf("getByID should not run after save error")
	}
}

func TestVaultServiceSaveConnectionTriggersPing(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	pinger := &recordingPinger{}
	pingMgr := NewPingManager(connRepo, domain.DefaultPingSettings(), newStubConcurrencyLimiter(1), pinger)
	svc := newTestVaultService(connRepo, &memoryPasswordRepo{}, &memoryIdentityRepo{}, pingMgr)

	conn := domain.Connection{ID: "c1", Host: "127.0.0.1", Port: 22, Protocol: domain.ProtocolSSH}
	if err := connRepo.Save(ctx, &conn); err != nil {
		t.Fatal(err)
	}

	_, err := svc.SaveConnection(ctx, &conn, nil)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&pinger.pings) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&pinger.pings) == 0 {
		t.Fatal("expected ping after SaveConnection")
	}
}

func TestVaultServiceFolderPasswordIdentityCRUD(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	pwRepo := &memoryPasswordRepo{}
	identRepo := &memoryIdentityRepo{}
	svc := newTestVaultService(connRepo, pwRepo, identRepo, nil)

	folder := domain.ConnectionFolder{Name: "Work"}
	if err := svc.SaveFolder(ctx, &folder); err != nil {
		t.Fatal(err)
	}
	folders, err := svc.GetAllFolders(ctx)
	if err != nil || len(folders) != 1 || folders[0].Name != "Work" {
		t.Fatalf("folders: %+v err=%v", folders, err)
	}
	if err := svc.DeleteFolder(ctx, folder.ID); err != nil {
		t.Fatal(err)
	}

	pwID, err := svc.ImportPassword(ctx, []byte("secret"), "label")
	if err != nil || pwID != "pw-1" || pwRepo.importN != 1 {
		t.Fatalf("import password: id=%q err=%v n=%d", pwID, err, pwRepo.importN)
	}
	if err := svc.DeletePassword(ctx, pwID); err != nil {
		t.Fatal(err)
	}

	identity, err := svc.ImportIdentity(ctx, []byte("pem"), "comment")
	if err != nil || identity.ID != "id-1" || identRepo.importN != 1 {
		t.Fatalf("import identity: %+v err=%v n=%d", identity, err, identRepo.importN)
	}
	idents, err := svc.GetAllIdentities(ctx)
	if err != nil || len(idents) != 1 {
		t.Fatalf("identities: %+v err=%v", idents, err)
	}
}

func TestVaultServiceConnectionMutations(t *testing.T) {
	ctx := context.Background()
	connRepo := newMemoryConnRepo()
	svc := newTestVaultService(connRepo, &memoryPasswordRepo{}, &memoryIdentityRepo{}, nil)

	if err := svc.DeleteConnection(ctx, "missing"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveConnections(ctx, []string{"c1"}, "f1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveFolder(ctx, "f1", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReorderConnections(ctx, []string{"c1"}, "f1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReorderFolders(ctx, []string{"f1"}, ""); err != nil {
		t.Fatal(err)
	}

	connRepo.conns["c1"] = domain.Connection{ID: "c1", Name: "A"}
	all, err := svc.GetAllConnections(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("connections: %+v err=%v", all, err)
	}
}
