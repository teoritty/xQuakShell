package usecase

import (
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain"
)

// The tab shows this message and nothing else, so a passphrase outcome has to survive every
// wrapping the connect path adds - including a jump hop's, whose prefix would otherwise win.
func TestSSHConnectErrorMessageNamesPassphraseOutcomes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"cancelled on the target", fmt.Errorf("authentication failed: %w",
			fmt.Errorf("passphrase request for k: %w", domain.ErrPassphrasePromptCancelled)), "Key passphrase was not entered"},
		{"wrong on the target", fmt.Errorf("authentication failed: %w",
			fmt.Errorf("parse key k with passphrase: %w", domain.ErrKeyPassphraseWrong)), "Wrong key passphrase"},
		{"wrong on a jump hop", fmt.Errorf("jump chain connection failed: %w",
			fmt.Errorf("hop 0 auth: %w", domain.ErrKeyPassphraseWrong)), "Wrong key passphrase"},
		{"server refused the key", fmt.Errorf("authentication failed: %w", errors.New("no supported methods remain")), "Authentication failed"},
		{"jump hop unreachable", fmt.Errorf("jump chain connection failed: %w", errors.New("dial tcp: refused")), "Jump chain connection failed"},
		{"anything else", errors.New("connection failed: i/o timeout"), "Connection failed"},
	}
	for _, tc := range cases {
		if got := sshConnectErrorMessage(tc.err); got != tc.want {
			t.Errorf("%s: sshConnectErrorMessage = %q, want %q", tc.name, got, tc.want)
		}
	}
}
