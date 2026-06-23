package pluginsdk_test

import (
	"errors"
	"testing"

	"github.com/xquakshell/pluginsdk"
)

func TestCoreErrorHelpers(t *testing.T) {
	err := &pluginsdk.CoreError{Code: pluginsdk.ErrCodeCapabilityDenied, Message: "capability denied"}
	if !pluginsdk.IsCapabilityDenied(err) {
		t.Fatal("expected capability denied")
	}
	if pluginsdk.IsRateLimited(err) {
		t.Fatal("did not expect rate limited")
	}
	code, ok := pluginsdk.CoreErrorCode(err)
	if !ok || code != pluginsdk.ErrCodeCapabilityDenied {
		t.Fatalf("unexpected code %d ok=%v", code, ok)
	}
}

func TestCoreErrorCodeMissing(t *testing.T) {
	if _, ok := pluginsdk.CoreErrorCode(errors.New("other")); ok {
		t.Fatal("expected false for generic error")
	}
}
