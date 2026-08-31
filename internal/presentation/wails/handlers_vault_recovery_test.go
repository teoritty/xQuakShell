package wails

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// fakeRecovery answers unlock attempts from a fixed pair of credentials, so the routing, throttling
// and error-shaping in the handlers can be tested without paying a scrypt pass per attempt.
type fakeRecovery struct {
	password string
	key      string

	converted   bool
	hasKey      bool
	issued      []string
	issueErr    error
	resets      int
	lastNewPass string
}

func (r *fakeRecovery) UnlockWithCredential(_ context.Context, credential string) (domain.UnlockMethod, error) {
	normalized, ok := domain.NormalizeRecoveryKey(credential)
	switch {
	case credential == r.password:
		return domain.UnlockByPassword, nil
	case ok && normalized == r.key:
		return domain.UnlockByRecoveryKey, nil
	default:
		return domain.UnlockByPassword, domain.ErrVaultDecryptFailed
	}
}

func (r *fakeRecovery) IssueRecoveryKey(context.Context) (string, error) {
	if r.issueErr != nil {
		return "", r.issueErr
	}
	r.hasKey = true
	minted := strings.Repeat("A", domain.RecoveryKeyLength-1) + string(rune('0'+len(r.issued)))
	r.issued = append(r.issued, minted)
	return minted, nil
}

func (r *fakeRecovery) ChangeMasterPassword(ctx context.Context, current, next string) (string, error) {
	if current != r.password {
		return "", domain.ErrVaultDecryptFailed
	}
	r.password = next
	return r.IssueRecoveryKey(ctx)
}

func (r *fakeRecovery) CompleteRecoveryReset(ctx context.Context, next string) (string, error) {
	r.resets++
	r.lastNewPass = next
	r.password = next
	return r.IssueRecoveryKey(ctx)
}

func (r *fakeRecovery) HasRecoveryKey() bool    { return r.hasKey }
func (r *fakeRecovery) ConvertedOnUnlock() bool { return r.converted }

// refusingVault stands in for the plain repository, which the recovery-aware path replaces. Only
// the settings applied after a successful open reach it, and nothing here opens successfully.
type refusingVault struct {
	domain.VaultRepository
	password string
}

func (v *refusingVault) Unlock(context.Context, string) error { return domain.ErrVaultDecryptFailed }
func (v *refusingVault) IsUnlocked() bool                     { return false }

// GetData returns a vault with no settings, so afterVaultOpened runs its whole path and applies
// nothing. A password unlock has to reach that call, and a repository that panicked there would
// hide the very branch these tests exist to walk.
func (v *refusingVault) GetData() (*domain.VaultData, error) { return &domain.VaultData{}, nil }

// VerifyMasterPassword answers the re-authentication the settings handlers require, against the
// same password the fake recovery port knows.
func (v *refusingVault) VerifyMasterPassword(_ context.Context, password string) error {
	if password != v.password {
		return domain.ErrVaultDecryptFailed
	}
	return nil
}

// testClock is the injected time source. Real sleeping in a throttle test buys nothing but a slow
// suite, and a test that waited out a five-minute recovery backoff would never be run.
type testClock struct{ now time.Time }

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func (c *testClock) Now() time.Time          { return c.now }
func (c *testClock) Advance(d time.Duration) { c.now = c.now.Add(d) }
func (c *testClock) AdvancePastAnyBackoff()  { c.Advance(2 * domain.RecoveryMaxDelay) }

func recoveryAPI(t *testing.T, recovery *fakeRecovery) (*AppAPI, *testClock) {
	t.Helper()
	clock := newTestClock()
	return &AppAPI{
		vaultRepo:        &refusingVault{password: recovery.password},
		recovery:         recovery,
		unlockThrottle:   *domain.NewUnlockThrottle(clock.Now),
		recoveryThrottle: domain.NewRecoveryThrottle(clock.Now),
	}, clock
}

func validKey() string { return strings.Repeat("A", domain.RecoveryKeyLength) }

// The single field must accept both credentials and say which one was used, because a recovery
// unlock has to land on the reset screen rather than the connection list.
func TestOneFieldAcceptsEitherCredentialAndNamesIt(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}

	for _, tc := range []struct {
		name       string
		credential string
		want       string
	}{
		{"master password", "the-master-password", "password"},
		{"recovery key, grouped as printed", domain.FormatRecoveryKey(validKey()), "recovery"},
		{"recovery key, lowercase and unspaced", strings.ToLower(validKey()), "recovery"},
	} {
		api, _ := recoveryAPI(t, recovery)
		got, err := api.UnlockVault(tc.credential)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got.Method != tc.want {
			t.Errorf("%s: method = %q, want %q", tc.name, got.Method, tc.want)
		}
	}
}

// A wrong password and a wrong recovery key must be indistinguishable from outside. Telling them
// apart would hand someone holding a stolen vault file the one bit that decides which half of it is
// worth spending compute on.
func TestAWrongPasswordAndAWrongKeyAreTheSameFailure(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}

	passwordAPI, _ := recoveryAPI(t, recovery)
	keyAPI, _ := recoveryAPI(t, recovery)

	_, passwordErr := passwordAPI.UnlockVault("not-the-password")
	_, keyErr := keyAPI.UnlockVault(strings.Repeat("B", domain.RecoveryKeyLength))

	if passwordErr == nil || keyErr == nil {
		t.Fatal("a wrong credential was accepted")
	}
	if passwordErr.Error() != keyErr.Error() {
		t.Errorf("the two failures read differently:\n  password: %q\n  key:      %q\nthat difference is an oracle", passwordErr, keyErr)
	}
	if !errors.Is(passwordErr, domain.ErrVaultDecryptFailed) || !errors.Is(keyErr, domain.ErrVaultDecryptFailed) {
		t.Error("the failures do not carry ErrVaultDecryptFailed, so nothing downstream counts them as guesses")
	}
}

// Which counter an attempt lands on is decided by the shape of the input. Sharing one counter would
// let a morning of password typos lock the recovery prompt for someone who has just discovered they
// need it.
func TestFailedAttemptsChargeTheThrottleForTheirOwnCredential(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	// The recovery throttle allows one free attempt, so two failures put it into backoff.
	for range 2 {
		if _, err := api.UnlockVault(strings.Repeat("B", domain.RecoveryKeyLength)); err == nil {
			t.Fatal("a wrong recovery key was accepted")
		}
	}
	if _, allowed := api.recoveryThrottle.Check(); allowed {
		t.Error("two wrong recovery keys left the recovery throttle open")
	}
	if _, allowed := api.unlockThrottle.Check(); !allowed {
		t.Error("wrong recovery keys closed the password throttle; someone who lost their key could no longer type their password")
	}

	// The password is still accepted while the recovery side is in backoff.
	if _, err := api.UnlockVault("the-master-password"); err != nil {
		t.Errorf("the password was refused while the recovery throttle was waiting: %v", err)
	}
}

// The throttle has to refuse before the credential is checked, or the wait costs the attacker
// nothing and the backoff is decoration.
func TestAThrottledAttemptIsRefusedWithoutCheckingTheCredential(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	for range 2 {
		if _, err := api.UnlockVault(strings.Repeat("B", domain.RecoveryKeyLength)); err == nil {
			t.Fatal("a wrong recovery key was accepted")
		}
	}

	// The correct key, offered while the throttle is waiting, must still be refused.
	_, err := api.UnlockVault(domain.FormatRecoveryKey(validKey()))
	if !errors.Is(err, domain.ErrUnlockThrottled) {
		t.Fatalf("got %v, want ErrUnlockThrottled; a throttle that lets the right answer through cannot slow a search down", err)
	}
}

// A successful unlock clears its own counter and only its own. Clearing both would let an attacker
// who knows one credential reset the budget for guessing the other.
func TestASuccessClearsOnlyItsOwnCounter(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, clock := recoveryAPI(t, recovery)

	if _, err := api.UnlockVault("wrong"); err == nil {
		t.Fatal("a wrong password was accepted")
	}
	if api.unlockThrottle.Failures() != 1 {
		t.Fatalf("password failures = %d, want 1", api.unlockThrottle.Failures())
	}
	if _, err := api.UnlockVault(strings.Repeat("B", domain.RecoveryKeyLength)); err == nil {
		t.Fatal("a wrong recovery key was accepted")
	}

	// One wrong key spends the single free recovery attempt, so the backoff has to elapse before
	// the right one is even looked at.
	clock.AdvancePastAnyBackoff()

	if _, err := api.UnlockVault(domain.FormatRecoveryKey(validKey())); err != nil {
		t.Fatalf("the correct recovery key was refused: %v", err)
	}
	if api.recoveryThrottle.Failures() != 0 {
		t.Errorf("recovery failures = %d after a successful recovery unlock, want 0", api.recoveryThrottle.Failures())
	}
	if api.unlockThrottle.Failures() != 1 {
		t.Errorf("password failures = %d after a recovery unlock, want 1; one credential must not clear the other's history", api.unlockThrottle.Failures())
	}
}

// The held key is what SaveRecoveryKeyFile writes. Done must drop it, and nothing may write it
// afterwards - that is what makes the dialog's promise true rather than a claim the UI makes alone.
func TestDoneDropsTheOnlyCopyOfTheKey(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	dto, err := api.CompleteRecoveryReset("a-brand-new-password")
	if err != nil {
		t.Fatalf("complete reset: %v", err)
	}
	if dto.Key == "" {
		t.Fatal("the reset returned no key to show")
	}
	if _, held := api.pendingRecovery.peek(); !held {
		t.Fatal("nothing is held for the save handler while the dialog is open")
	}

	api.AcknowledgeRecoveryKey()

	if _, held := api.pendingRecovery.peek(); held {
		t.Error("a key is still held after Done; it can be saved again")
	}
	if _, err := api.SaveRecoveryKeyFile(); !errors.Is(err, domain.ErrRecoveryKeyUnavailable) {
		t.Errorf("save after Done: got %v, want ErrRecoveryKeyUnavailable", err)
	}
}

// Saving with nothing held must refuse rather than reach the file dialog. Otherwise anything in the
// WebView could open a native save picker on the user's screen whenever it liked.
func TestSavingWithNoPendingKeyRefusesBeforeTheDialog(t *testing.T) {
	api, _ := recoveryAPI(t, &fakeRecovery{password: "p", key: validKey()})

	saved, err := api.SaveRecoveryKeyFile()
	if !errors.Is(err, domain.ErrRecoveryKeyUnavailable) {
		t.Errorf("got %v, want ErrRecoveryKeyUnavailable", err)
	}
	if saved {
		t.Error("the handler reported a file was written with no key to write")
	}
}

// Rotation must replace, never accumulate. Two issued keys after two requests, and the second one
// different from the first, is what tells the user the paper they threw away stopped working.
func TestRotationMintsAFreshKeyEachTime(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	first, err := api.IssueRecoveryKey("the-master-password")
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	second, err := api.IssueRecoveryKey("the-master-password")
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if first.Key == second.Key {
		t.Error("rotating returned the same key; the previous one was never revoked")
	}
	if held, _ := api.pendingRecovery.peek(); held != recovery.issued[len(recovery.issued)-1] {
		t.Error("the handler is holding a key other than the one it just minted")
	}
}

// Minting a key from settings needs the master password. This is reachable on an unattended
// unlocked machine, and its entire effect is to print a credential that opens everything.
func TestMintingFromSettingsRequiresThePassword(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	if _, err := api.IssueRecoveryKey("not-the-password"); err == nil {
		t.Fatal("a key was printed without the master password")
	}
	if len(recovery.issued) != 0 {
		t.Errorf("issued %d keys despite refusing, want 0", len(recovery.issued))
	}
	if _, held := api.pendingRecovery.peek(); held {
		t.Error("a key is held for saving after a refused request")
	}
}

// A recovery unlock must not start the rest of the application. Until the password is replaced, the
// only credential that opened this vault is one the user was told to keep away from the machine.
func TestARecoveryUnlockDoesNotStartTheApplication(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	// afterVaultOpened reaches settingsSvc and the audit service, both nil here. Reaching them
	// would panic, so surviving this call is the assertion.
	got, err := api.UnlockVault(domain.FormatRecoveryKey(validKey()))
	if err != nil {
		t.Fatalf("unlock with the recovery key: %v", err)
	}
	if got.Method != "recovery" {
		t.Fatalf("method = %q, want %q", got.Method, "recovery")
	}
	if got.RecoveryKey != "" {
		t.Error("a recovery unlock handed back a key; the user has not proved anything yet")
	}
	if recovery.resets != 0 {
		t.Error("the unlock reset the password on its own")
	}
}

// Every handler has to refuse cleanly when no recovery implementation is wired, which is what a
// test double that predates this feature looks like. Panicking there would take out the settings
// screen for anyone running such a build.
func TestTheHandlersRefuseCleanlyWithoutARecoveryPort(t *testing.T) {
	api := &AppAPI{vaultRepo: &refusingVault{}}

	if _, err := api.IssueRecoveryKey("password"); err == nil {
		t.Error("IssueRecoveryKey succeeded with no recovery port")
	}
	if _, err := api.ChangeMasterPassword("a", "b"); err == nil {
		t.Error("ChangeMasterPassword succeeded with no recovery port")
	}
	if _, err := api.CompleteRecoveryReset("b"); err == nil {
		t.Error("CompleteRecoveryReset succeeded with no recovery port")
	}
	if api.HasRecoveryKey() {
		t.Error("HasRecoveryKey is true with no recovery port")
	}
	api.AcknowledgeRecoveryKey()
}

// A rejected unlock must leave the throttle counting even when the failure took no time at all,
// which is what a nil clock guards against regressing into.
func TestFailuresAccumulateAcrossCalls(t *testing.T) {
	recovery := &fakeRecovery{password: "the-master-password", key: validKey()}
	api, _ := recoveryAPI(t, recovery)

	start := time.Now()
	for i := range domain.UnlockFreeAttempts {
		if _, err := api.UnlockVault("wrong"); err == nil {
			t.Fatalf("attempt %d was accepted", i)
		}
	}
	if api.unlockThrottle.Failures() != domain.UnlockFreeAttempts {
		t.Errorf("failures = %d, want %d", api.unlockThrottle.Failures(), domain.UnlockFreeAttempts)
	}
	if _, allowed := api.unlockThrottle.Check(); allowed && time.Since(start) < domain.UnlockBaseDelay {
		t.Error("the throttle is still open after the free attempts were spent")
	}
}
