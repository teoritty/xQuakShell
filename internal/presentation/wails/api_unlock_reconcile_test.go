package wails

import "testing"

// Opening the vault is the first moment anything the vault records about a plugin can be written:
// plugins are discovered during composition, while it is still locked. If this call went missing,
// consent would never be carried forward and no plugin would ever get its scope folder - and both
// failures would be silent, because a missing grant reads as "not granted" rather than as an error
// and an absent scope simply exposes nothing.
func TestOpeningTheVaultRunsTheUnlockReconciler(t *testing.T) {
	ran := 0
	api := &AppAPI{
		vaultRepo:              &refusingVault{},
		pluginUnlockReconciler: func() { ran++ },
	}

	api.afterVaultOpened()

	if ran != 1 {
		t.Fatalf("the unlock reconciler ran %d times, want exactly 1", ran)
	}
}

// Composition may leave the hook unset - a build with no plugin runtime, or a test double. Opening
// the vault must not depend on it being there.
func TestOpeningTheVaultWithoutAReconcilerIsFine(t *testing.T) {
	api := &AppAPI{vaultRepo: &refusingVault{}}

	api.afterVaultOpened()
}
