package wails

import (
	"fmt"
	"log/slog"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// RecoveryKeyDTO carries a one-time recovery key to the dialog that shows it.
type RecoveryKeyDTO struct {
	// Key is the grouped display form, dashes included. The canonical form stays on this side of
	// the bridge; normalising what the user types back is the backend's job.
	Key string `json:"key"`
}

// CreateVault creates a new vault protected by masterPassword, issues its one-time recovery key and
// leaves the vault unlocked. It fails rather than overwriting an existing vault.
//
// The key comes back from the same call that creates the vault, so there is no state in which a
// vault exists and the screen that hands over its only backup credential has not been reached.
func (a *AppAPI) CreateVault(masterPassword string) (RecoveryKeyDTO, error) {
	if err := a.vaultRepo.Create(a.reqCtx(), masterPassword); err != nil {
		return RecoveryKeyDTO{}, err
	}
	a.afterVaultOpened()

	if a.recovery == nil {
		return RecoveryKeyDTO{}, nil
	}
	key, err := a.recovery.IssueRecoveryKey(a.reqCtx())
	if err != nil {
		// The vault exists and is open, so this cannot fail the creation. The user simply has no
		// recovery key yet and can mint one from settings; refusing to let them into a vault they
		// just made would be the worse outcome by far.
		slog.Warn("recovery key issue at vault creation failed", "component", "vault", "err", err)
		return RecoveryKeyDTO{}, nil
	}
	return a.holdIssuedKey(key, false), nil
}

// IssueRecoveryKey mints a new recovery key and revokes the previous one, after checking the master
// password.
//
// Re-authentication is required even though the vault is already open, because this is reachable
// from settings on an unattended unlocked machine and its whole effect is to print a credential
// that opens everything.
func (a *AppAPI) IssueRecoveryKey(masterPassword string) (RecoveryKeyDTO, error) {
	if a.recovery == nil {
		return RecoveryKeyDTO{}, fmt.Errorf("recovery keys unavailable: %w", domain.ErrVaultNotFound)
	}
	if err := a.verifyMasterPassword(masterPassword); err != nil {
		return RecoveryKeyDTO{}, err
	}

	// Asked before the new key exists: afterwards the answer is always yes, and the audit log would
	// claim every first key revoked one.
	replaced := a.recovery.HasRecoveryKey()

	key, err := a.recovery.IssueRecoveryKey(a.reqCtx())
	if err != nil {
		return RecoveryKeyDTO{}, err
	}
	return a.holdIssuedKey(key, replaced), nil
}

// ChangeMasterPassword replaces the master password and issues a new recovery key, revoking both
// old credentials in a single write.
func (a *AppAPI) ChangeMasterPassword(current, next string) (RecoveryKeyDTO, error) {
	if a.recovery == nil {
		return RecoveryKeyDTO{}, fmt.Errorf("recovery keys unavailable: %w", domain.ErrVaultNotFound)
	}

	key, err := a.recovery.ChangeMasterPassword(a.reqCtx(), current, next)
	if err != nil {
		return RecoveryKeyDTO{}, err
	}
	return a.holdIssuedKey(key, true), nil
}

// CompleteRecoveryReset sets a master password after an unlock that used the recovery key, issues a
// fresh key, and finally starts the rest of the application.
//
// afterVaultOpened runs here rather than at unlock. Until this call returns, the only credential
// that opened the vault is one the user was told to write down and store away from the machine, so
// the session is not one that should be carrying live connections.
func (a *AppAPI) CompleteRecoveryReset(newPassword string) (RecoveryKeyDTO, error) {
	if a.recovery == nil {
		return RecoveryKeyDTO{}, fmt.Errorf("recovery keys unavailable: %w", domain.ErrVaultNotFound)
	}

	key, err := a.recovery.CompleteRecoveryReset(a.reqCtx(), newPassword)
	if err != nil {
		return RecoveryKeyDTO{}, err
	}
	dto := a.holdIssuedKey(key, true)
	a.afterVaultOpened()
	return dto, nil
}

// AcknowledgeRecoveryKey is the Done button: it forgets the key the dialog was showing.
//
// After this returns, no handler can produce that key again. That is what makes the dialog's
// promise true rather than a claim the UI makes on its own - the countdown on the button is there
// to make sure the user has read the screen before the only copy is dropped.
func (a *AppAPI) AcknowledgeRecoveryKey() {
	a.pendingRecovery.clear()
}

// HasRecoveryKey reports whether the open vault has a recovery key, so settings can say so.
func (a *AppAPI) HasRecoveryKey() bool {
	return a.recovery != nil && a.recovery.HasRecoveryKey()
}

// holdIssuedKey keeps the key for the Save As handler and returns its display form.
//
// revoked says whether this replaced an existing key, which is the difference between one audit
// entry and two. The distinction is worth recording: "a key was revoked" is what tells a user
// reading the log later that a credential they still have on paper stopped working.
func (a *AppAPI) holdIssuedKey(key string, revoked bool) RecoveryKeyDTO {
	a.pendingRecovery.hold(key)
	if revoked {
		a.recoveryAudit.RecordRecoveryEvent(a.reqCtx(), usecase.RecoveryEventRevoked)
	}
	a.recoveryAudit.RecordRecoveryEvent(a.reqCtx(), usecase.RecoveryEventGenerated)
	return RecoveryKeyDTO{Key: domain.FormatRecoveryKey(key)}
}

// wireVaultRecovery installs the recovery credential path.
//
// The vault repository is also the recovery implementation; the type assertion fails only for a
// test double that does not implement it, and every handler here refuses cleanly when it is absent
// rather than panicking the moment a user opens the settings screen.
func (a *AppAPI) wireVaultRecovery(vaultRepo domain.VaultRepository, auditLogRepo domain.AuditLogRepository) {
	if recovery, ok := vaultRepo.(domain.VaultRecovery); ok {
		a.recovery = recovery
	}
	a.recoveryThrottle = domain.NewRecoveryThrottle(nil)
	a.recoveryAudit = usecase.NewRecoveryAuditRecorder(auditLogRepo)
}
