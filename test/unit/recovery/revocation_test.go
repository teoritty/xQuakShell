package recovery

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

// Rotation has to revoke, not accumulate. A vault that quietly kept every key it ever printed would
// mean the paper a user threw away last year still opens everything.
func TestRotatingTheKeyRevokesTheOldOne(t *testing.T) {
	f := newFixture(t)
	old := f.key

	fresh, err := f.repo.IssueRecoveryKey(context.Background())
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if fresh == old {
		t.Fatal("rotation returned the same key")
	}

	if _, err := f.unlock(t, domain.FormatRecoveryKey(old)); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Errorf("the old key still opens the vault: got %v, want ErrVaultDecryptFailed", err)
	}
	if _, err := f.unlock(t, domain.FormatRecoveryKey(fresh)); err != nil {
		t.Errorf("the new key does not open the vault: %v", err)
	}
}

// A password change is what someone does when they believe their credentials have been seen.
// Leaving the recovery key valid would make that gesture accomplish nothing, so both credentials
// move together and both old ones stop working.
func TestChangingThePasswordRevokesBothOldCredentials(t *testing.T) {
	f := newFixture(t)
	oldPassword, oldKey := f.password, f.key
	const nextPassword = "a-different-long-password"

	freshKey, err := f.repo.ChangeMasterPassword(context.Background(), oldPassword, nextPassword)
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	for _, tc := range []struct {
		name       string
		credential string
		wantOpen   bool
	}{
		{"old password", oldPassword, false},
		{"old recovery key", domain.FormatRecoveryKey(oldKey), false},
		{"new password", nextPassword, true},
		{"new recovery key", domain.FormatRecoveryKey(freshKey), true},
	} {
		_, err := f.unlock(t, tc.credential)
		if tc.wantOpen && err != nil {
			t.Errorf("%s should open the vault: %v", tc.name, err)
		}
		if !tc.wantOpen && !errors.Is(err, domain.ErrVaultDecryptFailed) {
			t.Errorf("%s still opens the vault: got %v, want ErrVaultDecryptFailed", tc.name, err)
		}
	}
}

// The reset after a recovery unlock is the whole point of forcing one: the credential the user just
// spent must stop working, or a key read off a desk stays a permanent way in.
func TestResettingAfterARecoveryUnlockRevokesTheKeyThatWasUsed(t *testing.T) {
	f := newFixture(t)
	usedKey := f.key
	const nextPassword = "chosen-after-the-reset"

	repo := f.reopen(t)
	method, err := repo.UnlockWithCredential(context.Background(), domain.FormatRecoveryKey(usedKey))
	if err != nil {
		t.Fatalf("unlock with recovery key: %v", err)
	}
	if method != domain.UnlockByRecoveryKey {
		t.Fatalf("method = %v, want UnlockByRecoveryKey", method)
	}
	f.repo = repo

	freshKey, err := repo.CompleteRecoveryReset(context.Background(), nextPassword)
	if err != nil {
		t.Fatalf("complete reset: %v", err)
	}

	if _, err := f.unlock(t, domain.FormatRecoveryKey(usedKey)); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Errorf("the spent recovery key still opens the vault: got %v, want ErrVaultDecryptFailed", err)
	}
	if _, err := f.unlock(t, f.password); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Errorf("the forgotten password still opens the vault after a reset: got %v", err)
	}
	if _, err := f.unlock(t, nextPassword); err != nil {
		t.Errorf("the new password does not open the vault: %v", err)
	}
	if _, err := f.unlock(t, domain.FormatRecoveryKey(freshKey)); err != nil {
		t.Errorf("the new recovery key does not open the vault: %v", err)
	}
}

// Changing the password without knowing the current one would make an unattended unlocked machine
// enough to lock the owner out of their own vault permanently.
func TestChangingThePasswordRequiresTheCurrentOne(t *testing.T) {
	f := newFixture(t)

	if _, err := f.repo.ChangeMasterPassword(context.Background(), "not-the-password", "some-new-password"); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("change with a wrong current password: got %v, want ErrVaultDecryptFailed", err)
	}
	if _, err := f.unlock(t, f.password); err != nil {
		t.Errorf("the original password stopped working after a refused change: %v", err)
	}
}

// The recovery key is not a substitute for the password when re-authenticating. Callers use
// VerifyMasterPassword before exporting a private key or trusting a plugin, and the recovery key is
// the credential most likely to be lying on paper next to the machine.
func TestReauthenticationRefusesTheRecoveryKey(t *testing.T) {
	f := newFixture(t)

	if err := f.repo.VerifyMasterPassword(context.Background(), domain.FormatRecoveryKey(f.key)); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Fatalf("re-authentication accepted the recovery key: got %v, want ErrVaultDecryptFailed", err)
	}
	if err := f.repo.VerifyMasterPassword(context.Background(), f.password); err != nil {
		t.Errorf("re-authentication rejected the master password: %v", err)
	}
	if _, err := f.repo.ChangeMasterPassword(context.Background(), domain.FormatRecoveryKey(f.key), "some-new-password"); err == nil {
		t.Error("the recovery key was accepted as the current password when changing it")
	}
}

// A password shorter than the policy must be refused wherever it is set, or the reset screen
// becomes the way around a rule the create screen enforces.
func TestTheResetPathEnforcesThePasswordPolicy(t *testing.T) {
	f := newFixture(t)

	short := "short"
	if _, err := f.repo.CompleteRecoveryReset(context.Background(), short); !errors.Is(err, domain.ErrMasterPasswordTooShort) {
		t.Errorf("reset to a short password: got %v, want ErrMasterPasswordTooShort", err)
	}
	if _, err := f.repo.ChangeMasterPassword(context.Background(), f.password, short); !errors.Is(err, domain.ErrMasterPasswordTooShort) {
		t.Errorf("change to a short password: got %v, want ErrMasterPasswordTooShort", err)
	}
	if _, err := f.unlock(t, f.password); err != nil {
		t.Errorf("the original password stopped working after refused changes: %v", err)
	}
}

// Issuing a key needs an open vault. A locked one has no vault key in memory to wrap, and anything
// that appeared to succeed there would be printing a credential that opens nothing.
func TestIssuingAKeyRequiresAnOpenVault(t *testing.T) {
	f := newFixture(t)
	f.repo.Lock()

	if _, err := f.repo.IssueRecoveryKey(context.Background()); !errors.Is(err, domain.ErrVaultLocked) {
		t.Errorf("issue on a locked vault: got %v, want ErrVaultLocked", err)
	}
	if f.repo.HasRecoveryKey() {
		t.Error("a locked vault claims to have a recovery key; nothing is in memory to answer with")
	}
}
