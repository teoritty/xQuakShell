package usecase_test

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

func TestBindingAndConfirmingACredential(t *testing.T) {
	v := &syncTestVault{data: domain.NewVaultData()}
	v.data.Identities["mine"] = domain.SSHIdentity{ID: "mine"}
	v.data.Identities["team"] = domain.SSHIdentity{ID: "team", VaultID: "t"}
	s := usecase.NewCredentialBindingService(v)
	b := domain.CredentialBinding{VaultID: "t", ConnectionID: "c", UserID: "u", IdentityID: "team"}
	if err := s.Bind(syncCtx, b); !errors.Is(err, usecase.ErrBindingTarget) {
		t.Fatalf("binding a team key: %v, want ErrBindingTarget", err)
	}
	b.IdentityID, b.SuspendedBy = "mine", "carol"
	if err := s.Bind(syncCtx, b); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Bindings(); list[0].SuspendedBy != "" {
		t.Fatal("a binding the member just made starts suspended")
	}
	if err := s.Bind(syncCtx, b); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Bindings()
	if len(list) != 1 {
		t.Fatalf("bindings = %+v; binding the same user again replaces the binding", list)
	}
	_ = v.UpdateData(syncCtx, func(d *domain.VaultData) error { d.Sync.Bindings[0].SuspendedBy = "carol"; return nil })
	if err := s.Confirm(syncCtx, "t", "c", "u"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Bindings(); list[0].SuspendedBy != "" {
		t.Fatal("a confirmed binding stays suspended")
	}
	if err := s.Confirm(syncCtx, "t", "c", "nobody"); !errors.Is(err, domain.ErrBindingMissing) {
		t.Fatalf("confirming nothing: %v", err)
	}
}
