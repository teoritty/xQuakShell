package wails

import "testing"

// Opening the vault is the first moment a grant can be written: plugins are discovered during
// composition, while the vault is still locked. If this call ever went missing, consent recorded
// before ADR-022 would never be carried forward - and the failure would be silent, because an
// unrecorded grant reads as "not granted" rather than as an error.
func TestOpeningTheVaultRunsTheConsentMigration(t *testing.T) {
	ran := 0
	api := &AppAPI{
		vaultRepo:              &refusingVault{},
		pluginConsentMigration: func() { ran++ },
	}

	api.afterVaultOpened()

	if ran != 1 {
		t.Fatalf("the consent migration ran %d times, want exactly 1", ran)
	}
}

// Composition may leave the hook unset - a build with no plugin runtime, or a test double. Opening
// the vault must not depend on it being there.
func TestOpeningTheVaultWithoutAConsentMigrationIsFine(t *testing.T) {
	api := &AppAPI{vaultRepo: &refusingVault{}}

	api.afterVaultOpened()
}
