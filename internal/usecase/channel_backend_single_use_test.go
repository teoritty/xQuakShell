package usecase

import (
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func singleUseExecTemplates() []domainplugin.ExecCommandTemplate {
	return []domainplugin.ExecCommandTemplate{{Argv: []string{"uptime"}}}
}

// A channel backend resolves and checks its target during Authorize, and Wire acts on what it
// finds there. The resolver is documented as returning a fresh backend per channel.open, and a
// documented rule is one a future resolver can break silently: share one instance and the second
// channel.open replaces the target the first was cleared for, so bytes flow somewhere nobody
// approved — past an authorization that did happen, for a destination that was never the one
// authorized.
//
// Each backend now refuses the second Authorize itself, which is the same rule enforced where the
// state actually lives.
func TestAnExecBackendRefusesASecondAuthorize(t *testing.T) {
	backend := NewChannelExecBackend("com.test", singleUseExecTemplates(), true, nil, nil, nil)

	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hintJSON(t, 0, nil)); err != nil {
		t.Fatalf("first authorize: %v", err)
	}
	err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hintJSON(t, 0, nil))

	if !errors.Is(err, domainplugin.ErrChannelBackendReused) {
		t.Fatalf("second authorize err = %v, want ErrChannelBackendReused", err)
	}
}

func TestAnEmbedBackendRefusesASecondAuthorize(t *testing.T) {
	sink := newFakeEmbedSink()
	sink.owners["sess-1"] = "com.test"
	backend := NewChannelEmbedBackend("com.test", true, sink, nil, nil)

	if err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "main"); err != nil {
		t.Fatalf("first authorize: %v", err)
	}
	err := backend.Authorize(domainplugin.PurposeEmbedStream, "sess-1", "other")

	if !errors.Is(err, domainplugin.ErrChannelBackendReused) {
		t.Fatalf("second authorize err = %v, want ErrChannelBackendReused", err)
	}
	// The refusal must also leave the first channel's target alone: reporting an error while
	// having already overwritten the field would be the same bug with a louder log line.
	backend.mu.Lock()
	tunnelID := backend.tunnelID
	backend.mu.Unlock()
	if tunnelID != "main" {
		t.Errorf("tunnelID = %q after a refused re-authorize, want the first channel's %q", tunnelID, "main")
	}
}

// A backend whose Authorize failed never became single-use: nothing was resolved into it, so the
// refusal must not lock out the retry a caller is entitled to make.
func TestAFailedAuthorizeDoesNotBurnTheBackend(t *testing.T) {
	backend := NewChannelExecBackend("com.test", singleUseExecTemplates(), true, nil, nil, nil)

	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hintJSON(t, 7, nil)); err == nil {
		t.Fatal("a template index outside the manifest allowlist was authorized")
	}
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hintJSON(t, 0, nil)); err != nil {
		t.Fatalf("a legitimate authorize after a refused one failed: %v", err)
	}
}
