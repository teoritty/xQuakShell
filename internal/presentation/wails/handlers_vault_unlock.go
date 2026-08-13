package wails

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"xquakshell/internal/domain"
)

// UnlockVault decrypts the vault with the given master password.
// After unlocking, applies persisted settings (e.g. lockout) to the running managers.
//
// Attempts are throttled. This binding is reachable by anything that reaches window.go, so without
// a growing cost per failure a script in the WebView guesses in a loop for as long as it likes;
// scrypt makes each guess expensive but never makes the ten-thousandth harder than the first.
func (a *AppAPI) UnlockVault(masterPassword string) error {
	if retryAfter, allowed := a.unlockThrottle.Check(); !allowed {
		return fmt.Errorf("%w: try again in %s", domain.ErrUnlockThrottled, retryAfter.Round(time.Second))
	}

	if err := a.vaultRepo.Unlock(a.reqCtx(), masterPassword); err != nil {
		// A wrong password is the only failure this counts. Refusing to open a vault that needs
		// migrating, or one written by a newer build, says nothing about the password and
		// throttling it would lock the user out over a condition they cannot type their way past.
		if errors.Is(err, domain.ErrVaultDecryptFailed) {
			a.unlockThrottle.RecordFailure()
			slog.Warn("vault unlock rejected",
				"component", "vault", "consecutiveFailures", a.unlockThrottle.Failures())
		}
		return vaultUnlockUserError(err)
	}

	a.unlockThrottle.RecordSuccess()
	a.afterVaultOpened()
	return nil
}
