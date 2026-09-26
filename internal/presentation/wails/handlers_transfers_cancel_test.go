package wails

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/usecase"
)

// Every rejected RPC becomes an error dialog on the frontend, so the transfer handlers must resolve
// a cancellation as success and nothing else.
func TestQuietCancelAbsorbsCancellationsOnly(t *testing.T) {
	absorbed := []error{
		usecase.ErrOperationCancelled,
		fmt.Errorf("%w: %w", usecase.ErrTransferCancelled, errors.New("write: file already closed")),
		fmt.Errorf("plan upload: %w", usecase.ErrTransferCancelled),
	}
	for _, err := range absorbed {
		if got := quietCancel(err); got != nil {
			t.Errorf("quietCancel(%v) = %v, want nil: the user cancelled, there is nothing to report", err, got)
		}
	}

	// A bare context.Canceled is not absorbed: it carries no evidence that the user asked for it,
	// and the use case marks every cancellation it owns.
	surfaced := []error{errors.New("permission denied"), context.Canceled}
	for _, err := range surfaced {
		if got := quietCancel(err); !errors.Is(got, err) {
			t.Errorf("quietCancel(%v) = %v, want the error unchanged", err, got)
		}
	}
	if quietCancel(nil) != nil {
		t.Error("quietCancel(nil) != nil")
	}
}
