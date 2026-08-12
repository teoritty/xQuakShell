package wails

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"xquakshell/internal/domain"
)

// verifyingVault records whether the master password was checked and with what, and can refuse it.
type verifyingVault struct {
	domain.VaultRepository
	seen     string
	verified bool
	err      error
}

func (v *verifyingVault) VerifyMasterPassword(_ context.Context, masterPassword string) error {
	v.verified = true
	v.seen = masterPassword
	return v.err
}

// The export gate is the only thing between a stray click and a private key on disk, and the use
// case will not second-guess the boolean it is handed - so if this check is skipped here, it does
// not happen anywhere.
func TestExportRefusesAnEmptyMasterPasswordWithoutAskingTheVault(t *testing.T) {
	vault := &verifyingVault{}
	api := &AppAPI{vaultRepo: vault, ctx: context.Background()}

	out, err := api.ExportKey("k1", "", "", "")
	if !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("export with no master password = %v, want ErrVaultDecryptFailed", err)
	}
	if out != "" {
		t.Error("a refused export returned data")
	}
	if vault.verified {
		t.Error("an empty password reached the vault; it is never a real answer and checking it costs a full scrypt pass")
	}
}

func TestExportRefusesAWrongMasterPassword(t *testing.T) {
	vault := &verifyingVault{err: domain.ErrVaultDecryptFailed}
	api := &AppAPI{vaultRepo: vault, ctx: context.Background()}

	if _, err := api.ExportKey("k1", "wrong", "", ""); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("export with a wrong master password = %v, want ErrVaultDecryptFailed", err)
	}
	if vault.seen != "wrong" {
		t.Errorf("the vault was asked about %q, want the password the caller supplied", vault.seen)
	}
}

// Verification must not go through Unlock: that replaces the in-memory snapshot and clears the
// dirty flag, so re-authenticating would discard any vault change not yet flushed to disk.
func TestExportVerifiesWithoutUnlocking(t *testing.T) {
	vault := &verifyingVault{err: domain.ErrVaultDecryptFailed}
	api := &AppAPI{vaultRepo: vault, ctx: context.Background()}

	if _, err := api.ExportKey("k1", "master", "", ""); err == nil {
		t.Fatal("expected the refusal to propagate")
	}
	if !vault.verified {
		t.Error("the master password was never verified")
	}
}

// A migrator that is absent must refuse cleanly rather than panic at the moment a user with an
// old vault tries to open it.
func TestMigrationHandlersRefuseWithoutAMigrator(t *testing.T) {
	api := &AppAPI{ctx: context.Background()}

	if _, err := api.PlanKeyMigration("master"); !errors.Is(err, domain.ErrVaultNotFound) {
		t.Errorf("plan without a migrator = %v, want ErrVaultNotFound", err)
	}
	if _, err := api.CompleteKeyMigration("master", nil); !errors.Is(err, domain.ErrVaultNotFound) {
		t.Errorf("complete without a migrator = %v, want ErrVaultNotFound", err)
	}
}

// An unset cache policy must mean what the application did before the policy existed. Mapping it
// to "never" instead would silently start asking for a passphrase on every connection.
func TestKeyOptionsDefaultToTheBehaviourThatPredatesThem(t *testing.T) {
	got := DTOToKeyOptions(KeyOptionsDTO{})
	if got.Cache != domain.CacheUntilLock {
		t.Errorf("cache policy = %q, want %q", got.Cache, domain.CacheUntilLock)
	}
	if got := DTOToKeyOptions(KeyOptionsDTO{CachePolicy: "weekly"}); got.Cache != domain.CacheUntilLock {
		t.Errorf("an unrecognised cache policy = %q, want %q", got.Cache, domain.CacheUntilLock)
	}
	if got := DTOToKeyOptions(KeyOptionsDTO{CachePolicy: "never"}); got.Cache != domain.CacheNever {
		t.Errorf("a recognised cache policy was not kept: %q", got.Cache)
	}
}

// The listing DTO must not carry key material. This is a compile-time-adjacent guard: it fails the
// moment somebody adds a convenient field to the struct.
func TestIdentityDTOCarriesNoPrivateMaterial(t *testing.T) {
	dto := IdentityToDTO(domain.SSHIdentity{ID: "k1", Comment: "prod", Fingerprint: "SHA256:x"})
	if dto.Fingerprint != "SHA256:x" || dto.Comment != "prod" {
		t.Fatalf("dto = %+v, want the metadata carried across", dto)
	}
	for _, field := range structFieldNames(dto) {
		switch field {
		case "PEM", "PEMData", "PrivateKey", "DataKey", "Passphrase", "Blob":
			t.Errorf("IdentityDTO carries %q; a listing must never be able to leak key material", field)
		}
	}
}

// structFieldNames lists a struct's field names via reflection, so the guard above checks the type
// as it is rather than a copy of it someone forgot to update.
func structFieldNames(v any) []string {
	t := reflect.TypeOf(v)
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		names = append(names, t.Field(i).Name)
	}
	return names
}
