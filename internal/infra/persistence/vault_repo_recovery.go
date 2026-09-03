package persistence

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
	"xquakshell/internal/pkg/safego"
)

// UnlockWithCredential opens the vault with the master password or the recovery key.
func (r *VaultRepo) UnlockWithCredential(_ context.Context, credential string) (domain.UnlockMethod, error) {
	return r.openWith(credential, true)
}

// openWith is the one place the vault is opened, for both credentials.
//
// allowRecovery is false for the plain VaultRepository.Unlock, and a recovery key offered there is
// reported as an ordinary decryption failure rather than as a refusal. Saying "that was the recovery
// key, but not here" would confirm a stolen key is the right one to a caller that cannot act on it.
func (r *VaultRepo) openWith(credential string, allowRecovery bool) (domain.UnlockMethod, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, data, method, err := vault.Open(r.dir, credential)
	if err != nil {
		return domain.UnlockByPassword, err
	}
	if method == domain.UnlockByRecoveryKey && !allowRecovery {
		return domain.UnlockByPassword, fmt.Errorf("vault unlock: %w", domain.ErrVaultDecryptFailed)
	}
	if vault.NeedsMigration(data) {
		// Refuse rather than migrate silently. A migration rewrites the file and needs
		// passphrases the unlock screen never asked for, so it belongs to an explicit flow the
		// user starts and can see the result of.
		return domain.UnlockByPassword, fmt.Errorf("vault version %d: %w", data.Version, domain.ErrVaultMigrationRequired)
	}

	converted := session.IsLegacy()
	if converted {
		if err := session.ConvertLegacy(credential, data); err != nil {
			return domain.UnlockByPassword, err
		}
	}

	// Unwrapping runs the same scrypt KDF as wrapping (see the scryptWorkFactor comment in
	// internal/infra/vault/vault.go) and transiently allocates ~256 MiB while doing so. Force the
	// Go runtime to release those pages back to the OS immediately. Without this, unlocking
	// produces an RSS spike that can visibly linger for several minutes before the runtime's
	// background scavenger reclaims it on its own. Runs in a goroutine so it never blocks the
	// caller waiting on the unlock's return.
	safego.GoNamed("vault.unlockGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})

	r.session = session
	r.data = data
	r.ensureVaultDataLocked()
	r.unlocked = true
	r.dirty = false
	r.generation = 0
	r.converted = converted

	return method, nil
}

// IssueRecoveryKey mints a recovery key, replaces any previous one, and writes the vault.
//
// The write is synchronous rather than debounced. The user is about to be shown a key and told it
// is the only copy; a four-hundred-millisecond window in which that is not yet true on disk is a
// window in which a crash makes the application a liar.
func (r *VaultRepo) IssueRecoveryKey(_ context.Context) (string, error) {
	key, err := vault.NewRecoveryKey()
	if err != nil {
		return "", err
	}
	if err := r.rewrap(func(s *vault.Session) error { return s.SetRecoveryWrap(key) }); err != nil {
		return "", err
	}
	return key, nil
}

// ChangeMasterPassword re-wraps the vault under a new password and issues a new recovery key,
// after checking the current password.
func (r *VaultRepo) ChangeMasterPassword(ctx context.Context, current, next string) (string, error) {
	if err := r.VerifyMasterPassword(ctx, current); err != nil {
		return "", err
	}
	return r.setPasswordAndReissue(next)
}

// CompleteRecoveryReset sets a master password after an unlock that used the recovery key, and
// issues a new key.
//
// There is no old password to check, which is the entire situation this exists for. What guards it
// is that the vault is already open, and the only credential that could have opened it without a
// password is the recovery key the user just spent.
func (r *VaultRepo) CompleteRecoveryReset(_ context.Context, next string) (string, error) {
	return r.setPasswordAndReissue(next)
}

// setPasswordAndReissue replaces both credentials in one write.
//
// Both move together on purpose: a password change is what someone does when they believe their
// credentials have been seen, and leaving the old recovery key valid would make that gesture
// accomplish nothing. One write, so the vault is never briefly openable by neither or by both.
func (r *VaultRepo) setPasswordAndReissue(next string) (string, error) {
	if len([]rune(next)) < domain.MinMasterPasswordLength {
		return "", domain.ErrMasterPasswordTooShort
	}

	key, err := vault.NewRecoveryKey()
	if err != nil {
		return "", err
	}
	err = r.rewrap(func(s *vault.Session) error {
		if err := s.SetPasswordWrap(next); err != nil {
			return err
		}
		return s.SetRecoveryWrap(key)
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

// rewrap applies a change to the session's credential wraps and persists it immediately.
func (r *VaultRepo) rewrap(mutate func(*vault.Session) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.unlocked || r.session == nil {
		return domain.ErrVaultLocked
	}
	if err := mutate(r.session); err != nil {
		return err
	}

	snapshot := domain.CloneVaultData(r.data)
	if err := r.session.Save(snapshot); err != nil {
		return err
	}
	r.dirty = false
	r.converted = false

	// Wrapping is a full scrypt pass, so this path carries the same ~256 MiB transient cost as an
	// unlock does.
	safego.GoNamed("vault.rewrapGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})
	return nil
}

// HasRecoveryKey reports whether the open vault has a recovery key that would unlock it.
func (r *VaultRepo) HasRecoveryKey() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.session.HasRecoveryWrap()
}

// ConvertedOnUnlock reports whether the current unlock upgraded a pre-envelope vault, which is what
// makes it the moment to issue a first recovery key.
func (r *VaultRepo) ConvertedOnUnlock() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.converted
}
