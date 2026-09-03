package wails

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// UnlockResultDTO tells the frontend which credential opened the vault.
type UnlockResultDTO struct {
	// Method is "password" or "recovery". A recovery unlock leaves the application on the
	// reset screen instead of the connection list.
	Method string `json:"method"`

	// RecoveryKey carries a one-time key when the unlock also issued one, which happens only when
	// this unlock upgraded a vault written before recovery keys existed. It is empty otherwise, and
	// it is the only response in which a key ever appears.
	RecoveryKey string `json:"recoveryKey,omitempty"`
}

// UnlockVault opens the vault with the master password or the recovery key, whichever the user
// typed into the single field, and reports which one worked.
//
// Attempts are throttled. This binding is reachable by anything that reaches window.go, so without
// a growing cost per failure a script in the WebView guesses in a loop for as long as it likes;
// scrypt makes each guess expensive but never makes the ten-thousandth harder than the first.
//
// The two credentials count against separate throttles, chosen by the shape of the input. That
// choice leaks nothing - it is a fact about characters the caller just typed - and it keeps a
// household that mistyped its password all morning from arriving at the recovery prompt already
// waiting out someone else's backoff.
func (a *AppAPI) UnlockVault(credential string) (UnlockResultDTO, error) {
	throttle := a.throttleFor(credential)
	if retryAfter, allowed := throttle.Check(); !allowed {
		return UnlockResultDTO{}, fmt.Errorf("%w: try again in %s", domain.ErrUnlockThrottled, retryAfter.Round(time.Second))
	}

	method, err := a.openVault(credential)
	if err != nil {
		// A wrong credential is the only failure this counts. Refusing to open a vault that needs
		// migrating, or one written by a newer build, says nothing about what was typed and
		// throttling it would lock the user out over a condition they cannot type their way past.
		if errors.Is(err, domain.ErrVaultDecryptFailed) {
			throttle.RecordFailure()
			slog.Warn("vault unlock rejected",
				"component", "vault", "consecutiveFailures", throttle.Failures())
		}
		return UnlockResultDTO{}, vaultUnlockUserError(err)
	}
	throttle.RecordSuccess()

	if method == domain.UnlockByRecoveryKey {
		// afterVaultOpened is deliberately not called yet. The vault is readable, but the password
		// it is protected by is one the user has just spent and, if they followed instructions, is
		// written on paper. Nothing else starts until CompleteRecoveryReset replaces it.
		a.recoveryAudit.RecordRecoveryEvent(a.reqCtx(), usecase.RecoveryEventUsed)
		return UnlockResultDTO{Method: "recovery"}, nil
	}

	a.afterVaultOpened()
	return UnlockResultDTO{Method: "password", RecoveryKey: a.issueKeyForConvertedVault()}, nil
}

// throttleFor picks the counter this attempt belongs to.
func (a *AppAPI) throttleFor(credential string) *domain.UnlockThrottle {
	if domain.LooksLikeRecoveryKey(credential) && a.recoveryThrottle != nil {
		return a.recoveryThrottle
	}
	return &a.unlockThrottle
}

// openVault routes to the recovery-aware unlock when one is wired, and to the plain one otherwise.
//
// The fallback matters for test doubles: a vault repository that does not implement
// domain.VaultRecovery must still unlock with a password rather than refuse everything.
func (a *AppAPI) openVault(credential string) (domain.UnlockMethod, error) {
	if a.recovery != nil {
		return a.recovery.UnlockWithCredential(a.reqCtx(), credential)
	}
	return domain.UnlockByPassword, a.vaultRepo.Unlock(a.reqCtx(), credential)
}

// issueKeyForConvertedVault mints a first recovery key for a vault that this unlock just upgraded
// from the pre-envelope format, and returns it for the one-time dialog.
//
// This is the only place a key is issued without the user asking. It fires once, on the unlock that
// performed the conversion, and never again - a user who dismissed the dialog by killing the
// application has to ask for a new key in settings. Handing one out on every launch would train
// people to click through the one screen that must not be clicked through.
//
// A failure here is logged and swallowed. The vault is open and the data is safe; refusing to let
// someone in because their optional backup credential could not be minted would turn a convenience
// into an outage.
func (a *AppAPI) issueKeyForConvertedVault() string {
	if a.recovery == nil || !a.recovery.ConvertedOnUnlock() {
		return ""
	}

	key, err := a.recovery.IssueRecoveryKey(a.reqCtx())
	if err != nil {
		slog.Warn("recovery key issue after vault upgrade failed", "component", "vault", "err", err)
		return ""
	}
	a.pendingRecovery.hold(key)
	a.recoveryAudit.RecordRecoveryEvent(a.reqCtx(), usecase.RecoveryEventGenerated)
	return domain.FormatRecoveryKey(key)
}
