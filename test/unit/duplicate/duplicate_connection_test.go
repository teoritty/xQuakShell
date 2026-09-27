package duplicate_test

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/keys"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/usecase"
)

// Duplication runs through the real vault, repository and service: the property that matters -
// that a copy is a connection the rest of the application accepts as its own - only shows once the
// copy goes back through the same save path every edited connection takes.

type fixture struct {
	vault *persistence.VaultRepo
	conns *persistence.ConnectionRepo
	svc   *usecase.VaultService
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	vault := persistence.NewVaultRepo(t.TempDir())
	if err := vault.Create(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	conns := persistence.NewConnectionRepo(vault)
	svc := usecase.NewVaultService(usecase.VaultServiceConfig{
		ConnRepo:     conns,
		PasswordRepo: persistence.NewPasswordRepo(vault),
		IdentRepo:    persistence.NewIdentityRepo(vault, keys.NewCodec(), keys.NewDataKey),
	})
	svc.SetForwardRuleValidator(usecase.NewForwardRuleValidator(conns, nil, nil))
	return fixture{vault: vault, conns: conns, svc: svc}
}

func (f fixture) saveSource(t *testing.T) *domain.Connection {
	t.Helper()
	src := &domain.Connection{
		Name: "prod-web", Host: "10.0.0.7", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u1", Username: "deploy", Auth: domain.AuthMethodKey,
			KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"key-1"}}}},
		DefaultUserID: "u1",
		ForwardRules: []domain.ForwardRule{
			{Kind: domain.ForwardRuleLocal, BindPort: 18080, TargetHost: "localhost", TargetPort: 8080, Enabled: true},
		},
	}
	saved, err := f.svc.SaveConnection(context.Background(), src, nil)
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	return saved
}

func TestDuplicateIsAConnectionTheSavePathAccepts(t *testing.T) {
	f := newFixture(t)
	src := f.saveSource(t)

	dup, err := f.svc.DuplicateConnection(context.Background(), src.ID, "prod-web - copy")
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if dup.ID == src.ID || dup.Name != "prod-web - copy" || dup.Host != src.Host || dup.DefaultUserID != "u1" {
		t.Fatalf("duplicate = %+v", dup)
	}
	all, err := f.svc.GetAllConnections(context.Background())
	if err != nil || len(all) != 2 {
		t.Fatalf("connections after duplicate = %d, %v; want the source and its copy", len(all), err)
	}

	// Forward rule ids are checked for uniqueness across every connection on save. A copy that
	// kept the source's would fail here, the first time anyone edited either of them.
	if _, err := f.svc.SaveConnection(context.Background(), dup, nil); err != nil {
		t.Errorf("saving the copy: %v", err)
	}
	if _, err := f.svc.SaveConnection(context.Background(), src, nil); err != nil {
		t.Errorf("saving the source after duplicating it: %v", err)
	}
}

func TestDeletingTheOriginalLeavesTheCopy(t *testing.T) {
	f := newFixture(t)
	src := f.saveSource(t)
	dup, err := f.svc.DuplicateConnection(context.Background(), src.ID, "copy")
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if err := f.svc.DeleteConnection(context.Background(), src.ID); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	if _, err := f.svc.GetConnection(context.Background(), dup.ID); err != nil {
		t.Errorf("the copy went with its original: %v", err)
	}
}

func TestDuplicateRefusesAnEmptyName(t *testing.T) {
	f := newFixture(t)
	src := f.saveSource(t)
	if _, err := f.svc.DuplicateConnection(context.Background(), src.ID, "  "); !errors.Is(err, domain.ErrInvalidConnectionConfig) {
		t.Errorf("err = %v, want ErrInvalidConnectionConfig", err)
	}
}

// The id is the only handle on the source, so nothing outside this vault can be copied into it.
func TestDuplicateOfAnUnknownIDIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.saveSource(t)
	if _, err := f.svc.DuplicateConnection(context.Background(), "not-in-this-vault", "copy"); !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Errorf("err = %v, want ErrConnectionNotFound", err)
	}
}

func TestDuplicateNeedsAnUnlockedVault(t *testing.T) {
	f := newFixture(t)
	src := f.saveSource(t)
	f.vault.Lock()
	if _, err := f.svc.DuplicateConnection(context.Background(), src.ID, "copy"); !errors.Is(err, domain.ErrVaultLocked) {
		t.Errorf("err = %v, want ErrVaultLocked", err)
	}
}
