package usecase

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// A member's device log is fetched while a commit is verified. Failing to fetch it is the server
// being unreachable or busy, which is retried; failing to verify it is tampering, which stops the
// vault. The UI test found a 429 in the first case stopping a team vault for good as "tampering".
func TestOnlyAWrongAnswerIsAnIntegrityFailure(t *testing.T) {
	timeout := &net.OpError{Op: "dial", Err: context.DeadlineExceeded}
	for name, cause := range map[string]error{
		"rate limited":     &synctrust.RateLimitedError{},
		"network":          fmt.Errorf("member log: %w", timeout),
		"server error":     synctrust.ErrRemoteInternal,
		"cancelled":        context.Canceled,
		"wrapped deep too": fmt.Errorf("resolve: %w", fmt.Errorf("fetch: %w", &synctrust.RateLimitedError{})),
	} {
		if err := integrity("commit", cause); errors.Is(err, synctrust.ErrIntegrity) {
			t.Errorf("%s: %v reads as an integrity failure; it must be retried, not stop the vault", name, err)
		}
	}
	for name, cause := range map[string]error{
		"bad signature":   vaultsync.ErrBadSignature,
		"refused member":  synctrust.ErrRemoteNotFound,
		"broken protocol": synctrust.ErrRemoteProtocol,
		"forbidden":       synctrust.ErrRemoteForbidden,
	} {
		if err := integrity("commit", cause); !errors.Is(err, synctrust.ErrIntegrity) || !errors.Is(err, cause) {
			t.Errorf("%s: %v; a wrong answer stops the vault and keeps its cause", name, err)
		}
	}
}
