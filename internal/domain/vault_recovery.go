package domain

import "context"

// UnlockMethod names which of the two credentials opened the vault.
type UnlockMethod int

const (
	// UnlockByPassword is the ordinary case: the master password.
	UnlockByPassword UnlockMethod = iota

	// UnlockByRecoveryKey means the password is gone. The caller must force a new one to be set
	// before the application is usable, because the vault is now protected by a credential the user
	// has just spent - and, if they followed the instructions, wrote on paper.
	UnlockByRecoveryKey
)

// VaultRecovery is the second credential path into the vault.
//
// It is a port of its own rather than four more methods on VaultRepository, for the same reason
// VaultMigrator is: everything here is reached by type assertion from the composition root, is
// irrelevant to the dozens of callers that only read and write vault data, and carries a threat
// model those callers should not have to think about.
//
// The implementation is the same VaultRepo.
type VaultRecovery interface {
	// UnlockWithCredential opens the vault with either credential and reports which one worked.
	//
	// A wrong password and a wrong recovery key both come back as ErrVaultDecryptFailed, worded
	// identically. Telling the two apart would hand a guesser the one bit that makes an offline
	// search worth starting: which half of the file to attack.
	UnlockWithCredential(ctx context.Context, credential string) (UnlockMethod, error)

	// IssueRecoveryKey mints a key and replaces any existing one. The vault must already be open.
	//
	// Replace, never add: a vault carries at most one recovery key, so the paper the user threw
	// away stops working the moment a new one is printed. The returned string is the only time the
	// key exists outside the user's hands - it is not stored anywhere it could be read back.
	IssueRecoveryKey(ctx context.Context) (string, error)

	// ChangeMasterPassword re-wraps the vault key under a new password and issues a fresh recovery
	// key, returning it. current is verified first, so knowing the old password is required.
	//
	// Both credentials move together on purpose. A password change is what a user does after they
	// think someone else has seen their credentials, and leaving the old recovery key valid would
	// make that gesture do nothing.
	ChangeMasterPassword(ctx context.Context, current, next string) (string, error)

	// CompleteRecoveryReset sets the master password after an unlock that used the recovery key,
	// and issues a fresh key. It is separate from ChangeMasterPassword because there is no old
	// password to verify - the whole reason this path exists is that nobody knows it.
	CompleteRecoveryReset(ctx context.Context, next string) (string, error)

	// HasRecoveryKey reports whether a recovery wrap is present on disk.
	HasRecoveryKey() bool

	// ConvertedOnUnlock reports whether the unlock that just happened upgraded a vault written
	// before recovery keys existed.
	//
	// That is the moment, and the only automatic one, to mint a first key and show it. It is
	// deliberately not the same question as HasRecoveryKey returning false: a user who closed the
	// one-time dialog by killing the application also has no key, and handing them a new one on
	// every launch would train them to dismiss the one screen that must not be dismissed.
	ConvertedOnUnlock() bool
}
