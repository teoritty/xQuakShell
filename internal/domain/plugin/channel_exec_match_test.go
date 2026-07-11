package plugin

import (
	"encoding/json"
	"errors"
	"testing"
)

func containerTemplates() []ExecCommandTemplate {
	return []ExecCommandTemplate{
		{Argv: []string{"docker", "system", "dial-stdio"}},
		{
			Argv:   []string{"docker", "exec", "-it", "{containerId}", "sh"},
			Params: map[string]string{"containerId": "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$"},
		},
	}
}

func mustHint(t *testing.T, req ExecChannelRequest) string {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal hint: %v", err)
	}
	return string(b)
}

func TestMatchExecCommand_ValidContainerID(t *testing.T) {
	hint := mustHint(t, ExecChannelRequest{Template: 1, Params: map[string]string{"containerId": "abc123"}})
	argv, err := MatchExecCommand(containerTemplates(), hint)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"docker", "exec", "-it", "abc123", "sh"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestMatchExecCommand_LiteralTemplateNoParams(t *testing.T) {
	hint := mustHint(t, ExecChannelRequest{Template: 0})
	argv, err := MatchExecCommand(containerTemplates(), hint)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"docker", "system", "dial-stdio"}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestMatchExecCommand_ParamFailsRegex(t *testing.T) {
	hint := mustHint(t, ExecChannelRequest{Template: 1, Params: map[string]string{"containerId": "x; rm -rf /"}})
	_, err := MatchExecCommand(containerTemplates(), hint)
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}

func TestMatchExecCommand_PartialRegexMatchRejected(t *testing.T) {
	// "abc123extra!" contains a valid prefix but must fail because the match isn't full-string.
	hint := mustHint(t, ExecChannelRequest{Template: 1, Params: map[string]string{"containerId": "abc123 extra"}})
	_, err := MatchExecCommand(containerTemplates(), hint)
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}

func TestMatchExecCommand_NoMatchingTemplate(t *testing.T) {
	hint := mustHint(t, ExecChannelRequest{Template: 5, Params: map[string]string{"containerId": "abc"}})
	_, err := MatchExecCommand(containerTemplates(), hint)
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}

func TestMatchExecCommand_MissingParam(t *testing.T) {
	hint := mustHint(t, ExecChannelRequest{Template: 1, Params: map[string]string{}})
	_, err := MatchExecCommand(containerTemplates(), hint)
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}

func TestMatchExecCommand_InvalidHintJSON(t *testing.T) {
	_, err := MatchExecCommand(containerTemplates(), "not json")
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied", err)
	}
}
