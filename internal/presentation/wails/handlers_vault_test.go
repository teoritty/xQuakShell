package wails

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

func TestVaultUnlockUserErrorNamesTheDirection(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"newer", fmt.Errorf("vault version 4: %w", domain.ErrVaultVersionTooNew), "newer version"},
		{"older", fmt.Errorf("vault version 2: %w", domain.ErrVaultVersionTooOld), "older format"},
	}

	for _, tc := range cases {
		got := vaultUnlockUserError(tc.err)
		if got == nil {
			t.Fatalf("%s: mapping swallowed the error", tc.name)
		}
		if !strings.Contains(got.Error(), tc.want) {
			t.Errorf("%s: message %q does not say %q; the form shows this text verbatim and the user cannot act on a direction they are not told", tc.name, got, tc.want)
		}
	}
}

// The unlock form keys on the literal "vault not found" to send the user to the create screen.
// Rewriting unrecognised errors would strand them retyping a password against a missing file.
func TestVaultUnlockUserErrorPassesOtherFailuresThrough(t *testing.T) {
	if got := vaultUnlockUserError(nil); got != nil {
		t.Errorf("nil must stay nil, got %v", got)
	}
	for _, err := range []error{domain.ErrVaultNotFound, domain.ErrVaultDecryptFailed} {
		got := vaultUnlockUserError(fmt.Errorf("read vault: %w", err))
		if !errors.Is(got, err) {
			t.Errorf("%v was rewritten into %q", err, got)
		}
	}
}
