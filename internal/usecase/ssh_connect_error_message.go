package usecase

import (
	"errors"
	"strings"

	"xquakshell/internal/domain"
)

// sshConnectErrorMessage is the one line a session tab shows when it could not connect. It names
// what the person can act on - their passphrase, their team credential, a missing key - before
// the generic prefixes, which only say where in the connect path it failed.
func sshConnectErrorMessage(err error) string {
	if err == nil {
		return "Connection failed"
	}
	msg := err.Error()
	switch {
	// The two key passphrase outcomes are named before the generic prefixes that would also match
	// them. "Authentication failed" after the user pressed Cancel, or after a typo, sends them
	// looking at the server's authorized_keys for a problem that is on their side of the dialog.
	case errors.Is(err, domain.ErrPassphrasePromptCancelled):
		return "Key passphrase was not entered"
	case errors.Is(err, domain.ErrKeyPassphraseWrong):
		return "Wrong key passphrase"
	// A team connection with no credential of this member's linked never reached the server, so
	// "Authentication failed" pointed at the server for something only this member can fix.
	case errors.Is(err, domain.ErrBindingMissing):
		return "No key or password of yours is linked to this team connection: set one in its editor"
	case errors.Is(err, domain.ErrBindingSuspended):
		return "Another member changed this connection's address: confirm it in Settings → Sync before connecting"
	case strings.HasPrefix(msg, "authentication failed:"):
		return "Authentication failed"
	case strings.HasPrefix(msg, "jump chain connection failed:"):
		return "Jump chain connection failed"
	default:
		return "Connection failed"
	}
}
